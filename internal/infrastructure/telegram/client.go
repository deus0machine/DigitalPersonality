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
)

const (
	historyPageLimit = 100
	dialogsPageLimit = 100
)

// Client wraps gotd/td and implements port.TelegramGateway.
// It is safe to call Dialogs/History/Self concurrently once Run has invoked its handler.
type Client struct {
	cfg     config.TelegramConfig
	syncCfg config.SyncConfig
	log     *slog.Logger

	// td and api are populated inside Run() before fn is called.
	td  atomic.Pointer[telegram.Client]
	api atomic.Pointer[tg.Client]

	// selfID is cached after Self() is called; used by the message mapper.
	selfID atomic.Int64
}

// New constructs a Client. Call Run to activate the MTProto connection.
func New(cfg config.TelegramConfig, syncCfg config.SyncConfig, log *slog.Logger) *Client {
	return &Client{cfg: cfg, syncCfg: syncCfg, log: log}
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

