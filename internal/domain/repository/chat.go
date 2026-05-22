package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type ChatRepository interface {
	Upsert(ctx context.Context, chat *entity.Chat) error
	GetByID(ctx context.Context, id int64) (*entity.Chat, error)
	ListAll(ctx context.Context) ([]*entity.Chat, error)

	// UpdateRelevance persists the relevance score and personality surface classification
	// produced by the ChatRelevanceScorer. Called after every sync run so the DB
	// always reflects the latest scoring decision (scores can change as signals improve).
	UpdateRelevance(ctx context.Context, chatID int64, score float32, reason string, surface entity.PersonalitySurface) error
}
