package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type EpisodeRepository interface {
	// Create persists a new episode and returns its generated ID.
	Create(ctx context.Context, episode *entity.Episode) (int64, error)

	// LinkMessages links a batch of messages to an episode in a single transaction.
	LinkMessages(ctx context.Context, links []entity.EpisodeMessage) error

	// GetByID returns an episode by its internal ID.
	GetByID(ctx context.Context, id int64) (*entity.Episode, error)

	// ListByChat returns episodes for a chat, ordered by started_at descending.
	ListByChat(ctx context.Context, chatID int64, limit, offset int) ([]*entity.Episode, error)

	// ListUnepisodedMessages returns messages in chatID that have not yet been
	// assigned to any episode, ordered by sent_at ascending.
	// This query is episodic in nature and belongs here rather than in MessageRepository.
	ListUnepisodedMessages(ctx context.Context, chatID int64, limit int) ([]*entity.Message, error)

	// UpsertSemantic persists the semantic document for an episode.
	UpsertSemantic(ctx context.Context, doc *entity.EpisodeSemanticDoc) error

	// GetSemantic returns the semantic document for an episode.
	GetSemantic(ctx context.Context, episodeID int64) (*entity.EpisodeSemanticDoc, error)

	// ListPendingEmbedding returns episode IDs whose semantic doc has
	// skip_embedding = FALSE and no matching entry in episode_embeddings.
	ListPendingEmbedding(ctx context.Context, model string, limit int) ([]int64, error)
}
