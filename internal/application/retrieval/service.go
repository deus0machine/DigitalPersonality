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

func applyDefaults(q Query) Query {
	if q.Limit <= 0 {
		q.Limit = defaultLimit
	}
	if q.SimilarityThreshold == 0 {
		q.SimilarityThreshold = 0.30
	}
	return q
}
