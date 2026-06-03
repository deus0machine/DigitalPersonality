package utterance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/digital-personality/internal/config"
)

// RetrievalService fetches in-window messages, builds utterances at runtime,
// and ranks them against a query using the configured Scorer.
//
// Swapping the Scorer (BM25 → embeddings) requires no changes here.
type RetrievalService struct {
	repo   Repository
	scorer Scorer
	cfg    config.UtteranceConfig
}

// NewRetrievalService wires a RetrievalService with the given scorer.
func NewRetrievalService(repo Repository, scorer Scorer, cfg config.UtteranceConfig) *RetrievalService {
	return &RetrievalService{repo: repo, scorer: scorer, cfg: cfg}
}

// Retrieve returns the top-limit results for query. chatID=0 searches all chats.
func (s *RetrievalService) Retrieve(ctx context.Context, query string, chatID int64, limit int) ([]SearchResult, error) {
	results, _, err := s.RetrieveWithStats(ctx, query, chatID, limit)
	return results, err
}

// RetrieveWithStats is like Retrieve but also returns pipeline timing metrics.
func (s *RetrievalService) RetrieveWithStats(ctx context.Context, query string, chatID int64, limit int) ([]SearchResult, RetrievalStats, error) {
	if strings.TrimSpace(query) == "" {
		return nil, RetrievalStats{}, fmt.Errorf("query must not be empty")
	}
	if limit <= 0 {
		limit = 10
	}

	// Fetch raw messages
	var msgs []MessageInput
	var err error
	if chatID == 0 {
		msgs, err = s.repo.FetchAllInWindowMessages(ctx)
	} else {
		msgs, err = s.repo.FetchInWindowMessages(ctx, chatID)
	}
	if err != nil {
		return nil, RetrievalStats{}, fmt.Errorf("fetch messages: %w", err)
	}

	// Build utterances
	gap := time.Duration(s.cfg.GapSeconds) * time.Second
	t0 := time.Now()
	utterances := Build(msgs, gap)
	buildDur := time.Since(t0)

	// Compute avg utterance length
	totalTok := 0
	for _, u := range utterances {
		totalTok += len(strings.Fields(u.Text))
	}
	avgLen := 0.0
	if len(utterances) > 0 {
		avgLen = float64(totalTok) / float64(len(utterances))
	}

	stats := RetrievalStats{
		RawMessages:     len(msgs),
		UtterancesBuilt: len(utterances),
		AvgUtteranceLen: avgLen,
		BuildDuration:   buildDur,
	}

	if len(utterances) == 0 {
		return nil, stats, nil
	}

	// Score
	t1 := time.Now()
	results, err := s.scorer.Score(ctx, query, utterances)
	stats.ScoreDuration = time.Since(t1)
	if err != nil {
		return nil, stats, fmt.Errorf("score: %w", err)
	}

	if limit < len(results) {
		results = results[:limit]
	}

	return results, stats, nil
}

// RetrieveWithContext is like Retrieve but wraps each hit with `window` utterances
// before and after from the same chat, for richer memory recall display.
func (s *RetrievalService) RetrieveWithContext(ctx context.Context, query string, chatID int64, limit, window int) ([]ContextResult, error) {
	results, _, err := s.RetrieveWithContextAndStats(ctx, query, chatID, limit, window)
	return results, err
}

// RetrieveWithContextAndStats is like RetrieveWithContext but also returns pipeline metrics.
func (s *RetrievalService) RetrieveWithContextAndStats(ctx context.Context, query string, chatID int64, limit, window int) ([]ContextResult, RetrievalStats, error) {
	if window <= 0 {
		window = 2
	}

	scored, stats, err := s.RetrieveWithStats(ctx, query, chatID, limit)
	if err != nil {
		return nil, stats, err
	}
	if len(scored) == 0 {
		return nil, stats, nil
	}

	// Re-build the full utterance slice for context lookup.
	// RetrieveWithStats already built it internally; we repeat the fetch+build
	// here to keep the API clean. The cost is acceptable for a prototype.
	var msgs []MessageInput
	if chatID == 0 {
		msgs, err = s.repo.FetchAllInWindowMessages(ctx)
	} else {
		msgs, err = s.repo.FetchInWindowMessages(ctx, chatID)
	}
	if err != nil {
		return nil, stats, fmt.Errorf("fetch messages for context: %w", err)
	}
	gap := time.Duration(s.cfg.GapSeconds) * time.Second
	utts := Build(msgs, gap)

	t2 := time.Now()
	contextResults := make([]ContextResult, 0, len(scored))
	totalContextTok := 0

	for _, hit := range scored {
		pos := hit.Utterance.Position
		before := contextBefore(utts, pos, hit.Utterance.ChatID, window)
		after := contextAfter(utts, pos, hit.Utterance.ChatID, window)

		for _, u := range before {
			totalContextTok += len(strings.Fields(u.Text))
		}
		totalContextTok += len(strings.Fields(hit.Utterance.Text))
		for _, u := range after {
			totalContextTok += len(strings.Fields(u.Text))
		}

		contextResults = append(contextResults, ContextResult{
			Hit:    hit,
			Before: before,
			After:  after,
		})
	}

	stats.ContextWindow = window
	stats.ContextDuration = time.Since(t2)
	if len(scored) > 0 {
		stats.AvgContextTokens = float64(totalContextTok) / float64(len(scored))
	}

	return contextResults, stats, nil
}

// contextBefore returns up to window utterances before position pos from the same chat,
// ordered chronologically (oldest first).
func contextBefore(utts []Utterance, pos int, chatID int64, window int) []Utterance {
	var collected []Utterance
	for i := pos - 1; i >= 0 && len(collected) < window; i-- {
		if utts[i].ChatID == chatID {
			collected = append(collected, utts[i])
		}
	}
	// collected is newest-first; reverse to chronological order
	for lo, hi := 0, len(collected)-1; lo < hi; lo, hi = lo+1, hi-1 {
		collected[lo], collected[hi] = collected[hi], collected[lo]
	}
	return collected
}

// contextAfter returns up to window utterances after position pos from the same chat,
// ordered chronologically.
func contextAfter(utts []Utterance, pos int, chatID int64, window int) []Utterance {
	var collected []Utterance
	for i := pos + 1; i < len(utts) && len(collected) < window; i++ {
		if utts[i].ChatID == chatID {
			collected = append(collected, utts[i])
		}
	}
	return collected
}
