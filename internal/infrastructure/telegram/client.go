// Package telegram provides a gotd/td-backed implementation of port.TelegramGateway.
// All gotd/td types are confined to this package; the port layer sees only clean DTOs.
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync/atomic"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/domain/entity"
)

const (
	historyPageLimit = 100
	dialogsPageLimit = 100
)

// Client wraps gotd/td and implements port.TelegramGateway and port.VoiceTranscriber.
// It is safe to call Dialogs/History/Self concurrently once Run has invoked its handler.
type Client struct {
	cfg            config.TelegramConfig
	syncCfg        config.SyncConfig
	transcriptionCfg config.TranscriptionConfig
	log            *slog.Logger

	// td and api are populated inside Run() before fn is called.
	td  atomic.Pointer[telegram.Client]
	api atomic.Pointer[tg.Client]

	// selfID is cached after Self() is called; used by the message mapper.
	selfID atomic.Int64
}

// New constructs a Client. Call Run to activate the MTProto connection.
func New(cfg config.TelegramConfig, syncCfg config.SyncConfig, transcriptionCfg config.TranscriptionConfig, log *slog.Logger) *Client {
	return &Client{cfg: cfg, syncCfg: syncCfg, transcriptionCfg: transcriptionCfg, log: log}
}

// Run connects to Telegram via MTProto, authenticates (or loads saved session),
// then invokes fn. All other Client methods MUST be called from within fn.
//
// On context cancellation Run performs a clean disconnect and returns ctx.Err.
// gotd/td handles FLOOD_WAIT and connection-level retries internally.
func (c *Client) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	session, err := newFileSession(c.cfg.SessionFile)
	if err != nil {
		return fmt.Errorf("init session storage: %w", err)
	}

	tdClient := telegram.NewClient(c.cfg.AppID, c.cfg.AppHash, telegram.Options{
		SessionStorage: session,
		Logger:         zap.NewNop(), // gotd internal logs; our business logs use slog
	})
	c.td.Store(tdClient)

	return tdClient.Run(ctx, func(ctx context.Context) error {
		c.api.Store(tdClient.API())

		if err := c.authenticate(ctx); err != nil {
			return fmt.Errorf("authenticate: %w", err)
		}
		c.log.Info("telegram authenticated", "phone", c.cfg.Phone)

		return fn(ctx)
	})
}

// Self returns the authenticated user's profile and caches the user ID.
func (c *Client) Self(ctx context.Context) (*port.UserInfo, error) {
	resp, err := c.mustAPI().UsersGetFullUser(ctx, &tg.InputUserSelf{})
	if err != nil {
		return nil, fmt.Errorf("UsersGetFullUser: %w", err)
	}
	if len(resp.Users) == 0 {
		return nil, fmt.Errorf("self user: empty response")
	}
	self, ok := resp.Users[0].(*tg.User)
	if !ok || !self.Self {
		return nil, fmt.Errorf("self user: unexpected response type")
	}
	c.selfID.Store(self.ID)
	return mapUserInfo(self), nil
}

// ListDialogs returns all accessible dialogs, paginating until exhausted.
func (c *Client) ListDialogs(ctx context.Context) ([]port.DialogInfo, error) {
	api := c.mustAPI()

	var (
		results    []port.DialogInfo
		offsetDate int
		offsetID   int
		offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	)

	for {
		resp, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			OffsetPeer: offsetPeer,
			Limit:      dialogsPageLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("MessagesGetDialogs: %w", err)
		}

		var (
			rawDialogs []tg.DialogClass
			rawUsers   []tg.UserClass
			rawChats   []tg.ChatClass
			total      int
		)

		switch v := resp.(type) {
		case *tg.MessagesDialogs:
			rawDialogs, rawUsers, rawChats = v.Dialogs, v.Users, v.Chats
			total = len(v.Dialogs)
		case *tg.MessagesDialogsSlice:
			rawDialogs, rawUsers, rawChats = v.Dialogs, v.Users, v.Chats
			total = v.Count
		case *tg.MessagesDialogsNotModified:
			return results, nil
		}

		if len(rawDialogs) == 0 {
			break
		}

		users, chats, channels := buildDialogLookups(rawUsers, rawChats)
		for _, d := range rawDialogs {
			dialog, ok := d.(*tg.Dialog)
			if !ok {
				continue
			}
			info, ok := mapDialogInfo(dialog, users, chats, channels)
			if !ok {
				continue
			}
			results = append(results, info)
		}

		if len(rawDialogs) < dialogsPageLimit || len(results) >= total {
			break
		}

		// Advance offset using last dialog's top message date.
		if last, ok := rawDialogs[len(rawDialogs)-1].(*tg.Dialog); ok {
			offsetID = last.TopMessage
			// offsetDate requires us to look up the date from the messages slice.
			// For simplicity, use 0 (server handles overlap gracefully with offsetID).
			offsetDate = 0
			offsetPeer = &tg.InputPeerEmpty{}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}

	c.log.Info("dialogs loaded", "count", len(results))
	return results, nil
}

// GetHistory returns one page of message history for the given dialog.
// Transparently retries on FLOOD_WAIT with exponential backoff.
// Non-flood errors are returned immediately without retry.
func (c *Client) GetHistory(ctx context.Context, req port.HistoryRequest) (*port.HistoryPage, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = historyPageLimit
	}

	tgReq := &tg.MessagesGetHistoryRequest{
		Peer:     inputPeerFromDialog(req.Dialog),
		OffsetID: int(req.OffsetID),
		Limit:    limit,
	}

	resp, err := c.fetchHistoryWithRetry(ctx, req.Dialog.ID, req.OffsetID, tgReq)
	if err != nil {
		return nil, err
	}
	return c.buildHistoryPage(resp)
}

// fetchHistoryWithRetry calls MessagesGetHistory and retries on FLOOD_WAIT.
// Sleep on each retry = floodWait * multiplier^(attempt-1) + jitter.
// Non-FLOOD_WAIT errors propagate immediately without consuming retries.
func (c *Client) fetchHistoryWithRetry(
	ctx context.Context,
	chatID, offset int64,
	req *tg.MessagesGetHistoryRequest,
) (tg.MessagesMessagesClass, error) {
	maxRetries := c.syncCfg.FloodMaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	multiplier := c.syncCfg.FloodBackoffMultiplier
	if multiplier <= 1.0 {
		multiplier = 1.5
	}
	jitter := c.syncCfg.FloodJitter

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.mustAPI().MessagesGetHistory(ctx, req)
		if err == nil {
			if attempt > 1 {
				c.log.Info("history retry succeeded",
					"chat_id", chatID, "offset", offset, "attempt", attempt)
			}
			return resp, nil
		}

		floodDur, isFlood := tgerr.AsFloodWait(err)
		if !isFlood {
			return nil, fmt.Errorf("MessagesGetHistory chat=%d offset=%d: %w", chatID, offset, err)
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("MessagesGetHistory chat=%d offset=%d flood wait: max retries (%d) exceeded: %w",
				chatID, offset, maxRetries, err)
		}

		sleep := time.Duration(float64(floodDur)*math.Pow(multiplier, float64(attempt-1))) + jitter

		c.log.Warn("history flood wait",
			"chat_id", chatID,
			"offset", offset,
			"wait_seconds", int(floodDur.Seconds()),
			"sleep", sleep.Round(time.Millisecond),
			"attempt", attempt,
		)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(sleep):
		}
	}

	// unreachable
	return nil, fmt.Errorf("MessagesGetHistory chat=%d offset=%d: exhausted %d retries", chatID, offset, maxRetries)
}

// TranscribeVoice requests Telegram Premium STT for the given voice message.
// Poll loop: calls MessagesTranscribeAudio up to PollAttempts times, sleeping
// PollDelay between attempts when Telegram returns Pending=true.
// FLOOD_WAIT is handled transparently inside doTranscribeWithFloodRetry.
//
// Errors are wrapped into port sentinels (ErrTranscriptionPending,
// ErrTranscriptionPermanent, ErrPremiumRequired) — no tgerr types leak out.
func (c *Client) TranscribeVoice(
	ctx           context.Context,
	chatType      entity.ChatType,
	chatID        int64,
	accessHash    int64,
	telegramMsgID int,
) (string, error) {
	peer := buildInputPeer(chatType, chatID, accessHash)
	req := &tg.MessagesTranscribeAudioRequest{Peer: peer, MsgID: telegramMsgID}

	pollAttempts := c.transcriptionCfg.PollAttempts
	if pollAttempts <= 0 {
		pollAttempts = 2
	}

	for poll := 1; poll <= pollAttempts; poll++ {
		resp, err := c.doTranscribeWithFloodRetry(ctx, req, chatID, telegramMsgID)
		if err != nil {
			return "", err
		}
		if !resp.Pending {
			return resp.Text, nil
		}
		if poll == pollAttempts {
			break
		}
		c.log.Info("transcription pending, will poll again",
			"chat_id", chatID, "telegram_id", telegramMsgID,
			"poll_attempt", poll, "poll_max", pollAttempts)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.transcriptionCfg.PollDelay):
		}
	}

	return "", port.ErrTranscriptionPending
}

// doTranscribeWithFloodRetry calls MessagesTranscribeAudio once, retrying only
// on FLOOD_WAIT. All other errors are classified and returned immediately.
func (c *Client) doTranscribeWithFloodRetry(
	ctx           context.Context,
	req           *tg.MessagesTranscribeAudioRequest,
	chatID        int64,
	telegramMsgID int,
) (*tg.MessagesTranscribedAudio, error) {
	maxRetries := c.syncCfg.FloodMaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := c.mustAPI().MessagesTranscribeAudio(ctx, req)
		if err == nil {
			return resp, nil
		}
		if d, ok := tgerr.AsFloodWait(err); ok {
			if attempt == maxRetries {
				return nil, fmt.Errorf("transcribe flood wait exhausted chat=%d msg=%d: %w",
					chatID, telegramMsgID, err)
			}
			sleep := d + c.syncCfg.FloodJitter
			c.log.Warn("transcribe flood wait",
				"chat_id", chatID, "telegram_id", telegramMsgID,
				"wait", sleep, "attempt", attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleep):
			}
			continue
		}
		return nil, c.classifyTranscribeError(err, chatID, telegramMsgID)
	}
	return nil, fmt.Errorf("transcribe exhausted retries chat=%d msg=%d", chatID, telegramMsgID)
}

// classifyTranscribeError wraps known permanent RPC errors into port sentinels.
func (c *Client) classifyTranscribeError(err error, chatID int64, msgID int) error {
	if tgerr.Is(err, "PREMIUM_ACCOUNT_REQUIRED") {
		return fmt.Errorf("%w: %w", port.ErrPremiumRequired, err)
	}
	if tgerr.Is(err, "MSG_VOICE_MISSING", "TRANSCRIPTION_FAILED", "MSG_ID_INVALID", "PEER_ID_INVALID") {
		return fmt.Errorf("%w: chat=%d msg=%d: %w", port.ErrTranscriptionPermanent, chatID, msgID, err)
	}
	return fmt.Errorf("transcribe chat=%d msg=%d: %w", chatID, msgID, err)
}

// buildInputPeer constructs the correct InputPeer from stored chat metadata.
func buildInputPeer(chatType entity.ChatType, chatID, accessHash int64) tg.InputPeerClass {
	switch chatType {
	case entity.ChatTypeSavedMessages:
		return &tg.InputPeerSelf{}
	case entity.ChatTypePrivate:
		return &tg.InputPeerUser{UserID: chatID, AccessHash: accessHash}
	case entity.ChatTypeGroup:
		return &tg.InputPeerChat{ChatID: chatID}
	case entity.ChatTypeChannel, entity.ChatTypeSupergroup:
		return &tg.InputPeerChannel{ChannelID: chatID, AccessHash: accessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func (c *Client) buildHistoryPage(resp tg.MessagesMessagesClass) (*port.HistoryPage, error) {
	selfID := c.selfID.Load()

	var (
		rawMessages []tg.MessageClass
		rawUsers    []tg.UserClass
		total       int
	)
	switch v := resp.(type) {
	case *tg.MessagesMessages:
		rawMessages, rawUsers, total = v.Messages, v.Users, len(v.Messages)
	case *tg.MessagesMessagesSlice:
		rawMessages, rawUsers, total = v.Messages, v.Users, v.Count
	case *tg.MessagesChannelMessages:
		rawMessages, rawUsers, total = v.Messages, v.Users, v.Count
	case *tg.MessagesMessagesNotModified:
		return &port.HistoryPage{}, nil
	}

	page := &port.HistoryPage{
		Messages:     make([]port.IncomingMessage, 0, len(rawMessages)),
		Participants: mapParticipants(rawUsers),
	}
	var minID int64

	for _, m := range rawMessages {
		msg, ok := m.(*tg.Message)
		if !ok {
			continue
		}
		incoming, ok := mapMessage(msg, selfID)
		if !ok {
			continue
		}
		page.Messages = append(page.Messages, incoming)

		if id := int64(msg.ID); minID == 0 || id < minID {
			minID = id
		}
	}

	page.MinID = minID
	page.HasMore = len(rawMessages) > 0 && total > len(rawMessages)
	return page, nil
}

// authenticate runs the interactive auth flow if no valid session exists.
func (c *Client) authenticate(ctx context.Context) error {
	userAuth := &consoleAuthenticator{
		phone:    c.cfg.Phone,
		password: c.cfg.TwoFAPassword,
	}
	flow := auth.NewFlow(userAuth, auth.SendCodeOptions{})
	// td.Auth() returns an *auth.Client backed by the live tg.Client;
	// IfNecessary is a no-op when a valid session is already loaded.
	return c.mustTD().Auth().IfNecessary(ctx, flow)
}

func (c *Client) mustAPI() *tg.Client {
	if api := c.api.Load(); api != nil {
		return api
	}
	panic("telegram: Client methods called outside of Run handler")
}

func (c *Client) mustTD() *telegram.Client {
	if td := c.td.Load(); td != nil {
		return td
	}
	panic("telegram: Client methods called outside of Run handler")
}

