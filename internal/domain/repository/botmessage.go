package repository

import (
	"context"
	"time"

	"github.com/digital-personality/internal/domain/entity"
)

// BotDialogSummary is one interlocutor's conversation overview.
type BotDialogSummary struct {
	UserID   int64
	Username string
	Messages int
	LastAt   time.Time
}

// BotMessageRepository persists and queries the bot conversation log.
type BotMessageRepository interface {
	Save(ctx context.Context, msg *entity.BotMessage) error

	// ListByUser returns one interlocutor's dialog, oldest first.
	ListByUser(ctx context.Context, userID int64, limit int) ([]entity.BotMessage, error)

	// ListDialogs returns per-interlocutor summaries, most recent first.
	ListDialogs(ctx context.Context) ([]BotDialogSummary, error)
}
