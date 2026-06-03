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

// ContextResult wraps a SearchResult with surrounding utterances from the same chat.
// Before and After are ordered chronologically (oldest first).
// Both slices are bounded to the same chat as Hit — context never crosses chat boundaries.
type ContextResult struct {
	Hit    SearchResult
	Before []Utterance // up to window utterances before hit, same chat
	After  []Utterance // up to window utterances after hit, same chat
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
// Context-specific fields are zero when context retrieval was not used.
type RetrievalStats struct {
	RawMessages      int
	UtterancesBuilt  int
	AvgUtteranceLen  float64 // average whitespace-token count per utterance
	BuildDuration    time.Duration
	ScoreDuration    time.Duration
	ContextWindow    int           // window size; 0 if context retrieval not used
	AvgContextTokens float64       // avg tokens across Before+Hit+After per result
	ContextDuration  time.Duration // time to extract all context windows
}
