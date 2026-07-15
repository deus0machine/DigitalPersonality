package utterance

import (
	"context"
	"fmt"
	"sort"
)

// defaultRRFK is the standard RRF smoothing constant: large enough that
// rank differences deep in the list matter little, small enough that
// top ranks dominate. 60 is the value from the original RRF paper.
const defaultRRFK = 60.0

// HybridScorer fuses a lexical and a vector Scorer via Reciprocal Rank Fusion:
//
//	fused(d) = Σ_i 1 / (k + rank_i(d))
//
// Documents found by both scorers accumulate both contributions, so agreement
// between lexical and semantic ranking pushes a result up. Scores from the
// underlying scorers are ignored — only ranks are used, which makes fusion
// robust to incomparable score scales (BM25 vs cosine similarity).
type HybridScorer struct {
	lexical Scorer
	vector  Scorer
	k       float64
}

// NewHybridScorer creates a HybridScorer with the standard RRF constant k=60.
func NewHybridScorer(lexical, vector Scorer) *HybridScorer {
	return &HybridScorer{lexical: lexical, vector: vector, k: defaultRRFK}
}

func (s *HybridScorer) Score(ctx context.Context, query string, utterances []Utterance) ([]SearchResult, error) {
	lex, err := s.lexical.Score(ctx, query, utterances)
	if err != nil {
		return nil, fmt.Errorf("lexical score: %w", err)
	}
	vec, err := s.vector.Score(ctx, query, utterances)
	if err != nil {
		return nil, fmt.Errorf("vector score: %w", err)
	}

	type fused struct {
		utt   Utterance
		score float64
	}
	byID := make(map[int64]*fused, len(lex)+len(vec))
	accumulate := func(ranked []SearchResult) {
		for rank, r := range ranked {
			id := r.Utterance.FirstMessageID
			f, ok := byID[id]
			if !ok {
				f = &fused{utt: r.Utterance}
				byID[id] = f
			}
			f.score += 1.0 / (s.k + float64(rank+1))
		}
	}
	accumulate(lex)
	accumulate(vec)

	results := make([]SearchResult, 0, len(byID))
	for _, f := range byID {
		results = append(results, SearchResult{Utterance: f.utt, Score: f.score})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Utterance.FirstMessageID < results[j].Utterance.FirstMessageID
	})
	return results, nil
}
