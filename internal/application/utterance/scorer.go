package utterance

import (
	"context"
	"time"
)

// SearchResult is one ranked result from the retrieval pipeline.
type SearchResult struct {
	Utterance Utterance
	Score     float64
}

// Scorer ranks utterances against a query. Implementations must be stateless
// with respect to the utterance corpus — the corpus is passed per call.
//
// Current implementation: BM25Scorer (lexical baseline).
// Future implementation: EmbeddingScorer (semantic, via pgvector or external API).
// RetrievalService code does not change when the scorer is swapped.
type Scorer interface {
	Score(ctx context.Context, query string, utterances []Utterance) ([]SearchResult, error)
}

// RetrievalStats carries pipeline timing and volume metrics for debugging.
type RetrievalStats struct {
	RawMessages     int
	UtterancesBuilt int
	AvgUtteranceLen float64 // average whitespace-token count per utterance
	BuildDuration   time.Duration
	ScoreDuration   time.Duration
}
