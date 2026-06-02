package retrieval

import (
	"context"
	"fmt"
)

const defaultLimit = 20

// Service provides message and episode retrieval over the personality memory.
// It delegates all I/O to Repository and adds only query-level defaults.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SearchMessages runs a full-text + trigram search against the messages table.
// FTS is attempted first; trigram similarity runs as a fallback when FTS returns nothing.
func (s *Service) SearchMessages(ctx context.Context, q Query) ([]MessageHit, error) {
	q = applyDefaults(q)
	hits, err := s.repo.SearchMessages(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	return hits, nil
}

// SearchEpisodes runs FTS against episode_semantic text.
func (s *Service) SearchEpisodes(ctx context.Context, q Query) ([]EpisodeHit, error) {
	q = applyDefaults(q)
	hits, err := s.repo.SearchEpisodes(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	return hits, nil
}

// FindSimilar returns messages whose text is trigram-similar to sample.
// Useful for identifying recurring phrases and style patterns.
func (s *Service) FindSimilar(ctx context.Context, sample string, q Query) ([]MessageHit, error) {
	if sample == "" {
		return nil, fmt.Errorf("sample text is required")
	}
	q = applyDefaults(q)
	hits, err := s.repo.FindSimilar(ctx, sample, q)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}
	return hits, nil
}

// ChatReport returns a personality analytics snapshot for one chat.
func (s *Service) ChatReport(ctx context.Context, chatID int64) (*PersonalityReport, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("chatID is required")
	}
	r, err := s.repo.PersonalityReport(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("personality report chat=%d: %w", chatID, err)
	}
	return r, nil
}

// AllReports returns one analytics snapshot per synced chat.
func (s *Service) AllReports(ctx context.Context) ([]PersonalityReport, error) {
	reports, err := s.repo.AllPersonalityReports(ctx)
	if err != nil {
		return nil, fmt.Errorf("all personality reports: %w", err)
	}
	return reports, nil
}

// WindowStats returns per-chat memory window coverage statistics.
// chatID=0 returns stats for all chats.
func (s *Service) WindowStats(ctx context.Context, chatID int64) ([]WindowStat, error) {
	stats, err := s.repo.WindowStats(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("window stats: %w", err)
	}
	return stats, nil
}

// WindowAnchors returns sample participation windows for a chat.
func (s *Service) WindowAnchors(ctx context.Context, chatID int64, windowBefore, windowAfter, anchorLimit int) ([]WindowAnchor, error) {
	anchors, err := s.repo.WindowAnchors(ctx, chatID, windowBefore, windowAfter, anchorLimit)
	if err != nil {
		return nil, fmt.Errorf("window anchors chat=%d: %w", chatID, err)
	}
	return anchors, nil
}

// WindowAnchorsDistributed returns up to 3 participation windows distributed
// across the full temporal span of the chat (early / middle / late).
func (s *Service) WindowAnchorsDistributed(ctx context.Context, chatID int64, windowBefore, windowAfter int) ([]WindowAnchor, error) {
	anchors, err := s.repo.WindowAnchorsDistributed(ctx, chatID, windowBefore, windowAfter)
	if err != nil {
		return nil, fmt.Errorf("window anchors distributed chat=%d: %w", chatID, err)
	}
	return anchors, nil
}

// Validate returns global quality metrics for the collected memory.
func (s *Service) Validate(ctx context.Context) (*ValidationStats, error) {
	v, err := s.repo.ValidationStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("validation stats: %w", err)
	}
	return v, nil
}

// TopChatsByVolume returns up to limit chats ranked by message volume.
func (s *Service) TopChatsByVolume(ctx context.Context, limit int) ([]TopChatEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	entries, err := s.repo.TopChatsByVolume(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("top chats by volume: %w", err)
	}
	return entries, nil
}

// InspectChat returns a detailed per-chat diagnostic snapshot.
func (s *Service) InspectChat(ctx context.Context, chatID int64) (*ChatInspectReport, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("chatID is required")
	}
	r, err := s.repo.ChatInspect(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("inspect chat %d: %w", chatID, err)
	}
	return r, nil
}

// MediaInspect returns a comprehensive media audit across all message types.
func (s *Service) MediaInspect(ctx context.Context) (*MediaInspectReport, error) {
	r, err := s.repo.MediaInspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("media inspect: %w", err)
	}
	return r, nil
}

// VoiceStats returns the global voice message count and top-20 chats by voice volume.
func (s *Service) VoiceStats(ctx context.Context) (*VoiceStats, error) {
	v, err := s.repo.VoiceStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("voice stats: %w", err)
	}
	return v, nil
}

func applyDefaults(q Query) Query {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	if q.SimilarityThreshold == 0 {
		q.SimilarityThreshold = 0.30
	}
	return q
}
