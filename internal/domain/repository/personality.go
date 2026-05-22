package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type PersonalityRepository interface {
	// SaveSignals persists a batch of personality signals for one message.
	// Uses UPSERT to handle re-extraction on message updates.
	SaveSignals(ctx context.Context, signals []entity.PersonalitySignal) error

	// GetSignals returns all signals for the given message.
	GetSignals(ctx context.Context, messageID int64) ([]entity.PersonalitySignal, error)

	// GetSignalsByType returns all signals of the given type across messages.
	// Used for aggregation queries (e.g. "all emoji_usage signals for user X").
	GetSignalsByType(ctx context.Context, signalType entity.SignalType, limit int) ([]entity.PersonalitySignal, error)

	// UpsertProfile saves an aggregated personality profile for a user.
	UpsertProfile(ctx context.Context, profile *entity.PersonalityProfile) error

	// GetProfile returns the personality profile for a user.
	GetProfile(ctx context.Context, userID int64) (*entity.PersonalityProfile, error)
}
