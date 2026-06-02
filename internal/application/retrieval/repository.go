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

	// WindowStats returns per-chat memory window coverage statistics.
	// chatID=0 returns stats for all chats that have at least one message.
	WindowStats(ctx context.Context, chatID int64) ([]WindowStat, error)

	// WindowAnchors returns sample participation windows for a chat.
	// Returns up to anchorLimit anchors, each with up to windowBefore/windowAfter context messages.
	WindowAnchors(ctx context.Context, chatID int64, windowBefore, windowAfter, anchorLimit int) ([]WindowAnchor, error)

	// WindowAnchorsDistributed returns up to 3 participation windows sampled across
	// the full temporal range of a chat: early (~10th percentile), middle (~50th),
	// and late (last anchor). Provides better coverage than consecutive-first sampling.
	WindowAnchorsDistributed(ctx context.Context, chatID int64, windowBefore, windowAfter int) ([]WindowAnchor, error)

	// ValidationStats returns global quality metrics across all collected memory.
	ValidationStats(ctx context.Context) (*ValidationStats, error)

	// TopChatsByVolume returns up to limit chats ordered by total message count.
	TopChatsByVolume(ctx context.Context, limit int) ([]TopChatEntry, error)

	// ChatInspect returns a detailed diagnostic snapshot for a single chat.
	ChatInspect(ctx context.Context, chatID int64) (*ChatInspectReport, error)

	// VoiceStats returns the global voice message count and per-chat breakdown.
	VoiceStats(ctx context.Context) (*VoiceStats, error)

	// MediaInspect returns a comprehensive media audit across all message types.
	MediaInspect(ctx context.Context) (*MediaInspectReport, error)
}
