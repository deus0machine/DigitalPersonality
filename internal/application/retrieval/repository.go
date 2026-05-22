package retrieval

import "context"

// Repository is the data-access port consumed by RetrievalService.
// Implementations live in infrastructure/postgres — nothing here imports pgx.
type Repository interface {
	// SearchMessages executes FTS + trigram search and returns ranked hits.
	// FTS is preferred; trigram similarity is a fallback when FTS yields nothing.
	SearchMessages(ctx context.Context, q Query) ([]MessageHit, error)

	// SearchEpisodes searches episode_semantic text using FTS.
	SearchEpisodes(ctx context.Context, q Query) ([]EpisodeHit, error)

	// FindSimilar returns messages whose text is trigram-similar to the given sample.
	// Useful for style clustering and pattern discovery.
	FindSimilar(ctx context.Context, sample string, q Query) ([]MessageHit, error)

	// PersonalityReport builds a per-chat analytics snapshot.
	PersonalityReport(ctx context.Context, chatID int64) (*PersonalityReport, error)

	// AllPersonalityReports returns one report per synced chat, ordered by relevance.
	AllPersonalityReports(ctx context.Context) ([]PersonalityReport, error)
}
