package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type EmbeddingRepository interface {
	// Save stores an embedding; deduplicates on (message_id, model).
	Save(ctx context.Context, emb *entity.Embedding) error

	// GetByMessageID returns all embeddings for a message.
	GetByMessageID(ctx context.Context, messageID int64) ([]*entity.Embedding, error)

	// SearchSimilar returns the top-k most similar messages by cosine similarity.
	SearchSimilar(ctx context.Context, vector []float32, model string, topK int) ([]*entity.SearchResult, error)

	// ListUnembedded returns message IDs that have no embedding for the given model.
	ListUnembedded(ctx context.Context, model string, limit int) ([]int64, error)
}
