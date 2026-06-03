package utterance

import (
	"context"
	"sort"
	"strings"
)

// RerankScorer wraps any Scorer and multiplies each result's score by a
// length-sigmoid factor: n_eff / (n_eff + K), where n_eff = min(tokens, Cap).
//
// This corrects BM25's built-in bias toward short documents: BM25 length
// normalisation (b=0.75) already over-rewards a 1-token utterance by ~3-8×
// relative to a 30-token utterance containing the same term. The sigmoid
// inverts that bias without adding unbounded bonuses.
//
// At K=10:
//   n=1  → ×0.09   n=10 → ×0.50
//   n=5  → ×0.33   n=20 → ×0.67   n=50 → ×0.83
type RerankScorer struct {
	inner Scorer
	k     float64
	capN  int
}

// NewRerankScorer wraps inner with the length-sigmoid reranker.
// k is the inflection point (token count where multiplier = 0.5); use 10.
// capN is the max token count fed to the formula; use 100.
func NewRerankScorer(inner Scorer, k float64, capN int) *RerankScorer {
	if k <= 0 {
		k = 10
	}
	if capN <= 0 {
		capN = 100
	}
	return &RerankScorer{inner: inner, k: k, capN: capN}
}

// Score delegates to the inner scorer, applies the length multiplier to every
// result, and returns them re-sorted by final score.
// The inner scorer is expected to return ALL results with score > 0 (no limit),
// so the reranker can promote utterances from any position, not just top-K.
func (s *RerankScorer) Score(ctx context.Context, query string, utterances []Utterance) ([]SearchResult, error) {
	results, err := s.inner.Score(ctx, query, utterances)
	if err != nil {
		return nil, err
	}

	for i := range results {
		n := float64(min(len(strings.Fields(results[i].Utterance.Text)), s.capN))
		if n == 0 {
			results[i].Score = 0
			continue
		}
		results[i].Score *= n / (n + s.k)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Drop zero-score results that may have been created by n==0 edge case.
	for len(results) > 0 && results[len(results)-1].Score == 0 {
		results = results[:len(results)-1]
	}

	return results, nil
}
