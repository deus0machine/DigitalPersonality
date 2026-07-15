// Package bot is the Telegram Bot API delivery layer for the digital persona.
// It long-polls getUpdates and answers incoming private messages as the
// persona: a burst of short messages with realistic typing pauses.
//
// Uses the Bot API over plain HTTP (no MTProto, no external SDK) —
// consistent with the ollama/openai infrastructure clients.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/digital-personality/internal/application/persona"
)

const (
	// maxTypingPause caps sampled intra-burst pauses so the bot never stalls.
	maxTypingPause = 10 * time.Second

	// typingRefresh re-sends the "typing" action while generating:
	// Telegram shows the indicator for ~5 seconds per sendChatAction call.
	typingRefresh = 4 * time.Second

	// pollErrorBackoff is the wait after a failed getUpdates call.
	pollErrorBackoff = 5 * time.Second
)

// Bot wires the persona service to the Telegram Bot API.
type Bot struct {
	api     *apiClient
	persona *persona.Service
	allowed map[int64]bool // empty = reply to everyone
	log     *slog.Logger
}

// New creates a Bot. allowedUserIDs restricts who the persona replies to;
// an empty list means everyone (privacy risk — logged as a warning in Run).
func New(token string, svc *persona.Service, allowedUserIDs []int64, log *slog.Logger) (*Bot, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	allowed := make(map[int64]bool, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowed[id] = true
	}
	return &Bot{
		api:     newAPIClient(token),
		persona: svc,
		allowed: allowed,
		log:     log,
	}, nil
}

// Run long-polls updates until ctx is cancelled. Messages are handled
// sequentially — generation is the bottleneck and Ollama is single-node.
func (b *Bot) Run(ctx context.Context) error {
	me, err := b.api.getMe(ctx)
	if err != nil {
		return fmt.Errorf("bot getMe: %w", err)
	}
	b.log.Info("bot started", "username", me.Username, "allowlist_size", len(b.allowed))
	if len(b.allowed) == 0 {
		b.log.Warn("bot allowlist is empty — persona will reply to ANYONE who messages it; " +
			"set TELEGRAM_BOT_ALLOWED_USER_IDS to restrict access")
	}

	offset := 0
	for {
		select {
		case <-ctx.Done():
			b.log.Info("bot stopping")
			return nil
		default:
		}

		updates, err := b.api.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.log.Error("get updates failed", "error", err)
			select {
			case <-time.After(pollErrorBackoff):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message == nil || strings.TrimSpace(u.Message.Text) == "" {
				continue
			}
			b.handleMessage(ctx, u.Message)
		}
	}
}

// handleMessage answers one incoming message as the persona.
// Message text is intentionally never logged — private data.
func (b *Bot) handleMessage(ctx context.Context, msg *message) {
	if len(b.allowed) > 0 && !b.allowed[msg.From.ID] {
		b.log.Info("message from non-allowlisted user ignored", "user_id", msg.From.ID)
		return
	}

	query := msg.Text
	if query == "/start" {
		query = "привет"
	}

	start := time.Now()

	// Keep the typing indicator alive while the persona thinks —
	// local generation takes tens of seconds.
	typingCtx, stopTyping := context.WithCancel(ctx)
	go b.keepTyping(typingCtx, msg.Chat.ID)
	reply, err := b.persona.Reply(ctx, query)
	stopTyping()

	if err != nil {
		b.log.Error("persona reply failed",
			"chat_id", msg.Chat.ID, "user_id", msg.From.ID,
			"duration", time.Since(start), "error", err)
		return
	}

	for i, text := range reply.Messages {
		if i > 0 {
			pause := reply.SamplePause(maxTypingPause)
			_ = b.api.sendChatAction(ctx, msg.Chat.ID, "typing")
			select {
			case <-time.After(pause):
			case <-ctx.Done():
				return
			}
		}
		if err := b.api.sendMessage(ctx, msg.Chat.ID, text); err != nil {
			b.log.Error("send message failed", "chat_id", msg.Chat.ID, "error", err)
			return
		}
	}

	b.log.Info("persona replied",
		"chat_id", msg.Chat.ID, "user_id", msg.From.ID,
		"messages", len(reply.Messages), "duration", time.Since(start))
}

// keepTyping refreshes the typing indicator until ctx is cancelled.
func (b *Bot) keepTyping(ctx context.Context, chatID int64) {
	_ = b.api.sendChatAction(ctx, chatID, "typing")
	ticker := time.NewTicker(typingRefresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = b.api.sendChatAction(ctx, chatID, "typing")
		}
	}
}
