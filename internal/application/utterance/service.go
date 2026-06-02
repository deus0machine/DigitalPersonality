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
