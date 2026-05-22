package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type SemanticRepository interface {
	// Upsert inserts or replaces the semantic document for a message.
	Upsert(ctx context.Context, doc *entity.SemanticDocument) error

	// GetByMessageID returns the semantic document for the given message.
	GetByMessageID(ctx context.Context, messageID int64) (*entity.SemanticDocument, error)

	// ListPendingEmbedding returns message IDs whose semantic document exists
	// but has no embedding yet (skip_embedding = FALSE, not in embeddings table).
	ListPendingEmbedding(ctx context.Context, model string, limit int) ([]int64, error)
}
