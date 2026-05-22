package repository

import (
	"context"
	"time"

	"github.com/digital-personality/internal/domain/entity"
)

// MessageFilter constrains list queries.
type MessageFilter struct {
	ChatID     int64
	SenderID   int64
	Since      time.Time
	Until      time.Time
	IsOutgoing *bool
	Limit      int
	Offset     int
}

type MessageRepository interface {
	// Upsert inserts or updates a message; deduplicates on (telegram_id, chat_id).
	Upsert(ctx context.Context, msg *entity.Message) (*entity.Message, error)

	// GetByID returns a message by internal ID.
	GetByID(ctx context.Context, id int64) (*entity.Message, error)

	// GetByTelegramID returns a message by (telegram_id, chat_id).
	GetByTelegramID(ctx context.Context, telegramID, chatID int64) (*entity.Message, error)

	// List returns messages matching the filter.
	List(ctx context.Context, filter MessageFilter) ([]*entity.Message, error)

	// GetCursor returns the last-sync position for a chat.
	GetCursor(ctx context.Context, chatID int64) (*entity.SyncCursor, error)

	// SaveCursor persists the sync cursor.
	SaveCursor(ctx context.Context, cursor *entity.SyncCursor) error

	// MarkDeleted soft-deletes a message.
	MarkDeleted(ctx context.Context, telegramID, chatID int64) error
}
