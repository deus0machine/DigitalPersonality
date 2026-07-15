package utterance

import (
	"context"
	"errors"
	"math"
	"testing"
)

// stubScorer returns a fixed ranked list regardless of query or corpus.
type stubScorer struct {
	results []SearchResult
	err     error
}

func (s stubScorer) Score(context.Context, string, []Utterance) ([]SearchResult, error) {
	return s.results, s.err
}

func hit(firstMessageID int64, score float64) SearchResult {
	return SearchResult{
		Utterance: Utterance{FirstMessageID: firstMessageID},
		Score:     score,
	}
}

func rrf(rank int) float64 { return 1.0 / (defaultRRFK + float64(rank)) }

func TestHybridScorerFusesByRankNotScore(t *testing.T) {
	// Lexical: doc1 rank1, doc2 rank2. Vector: doc2 rank1, doc3 rank2.
	// Raw scores are wildly incomparable on purpose — RRF must ignore them.
	lexical := stubScorer{results: []SearchResult{hit(1, 99999.0), hit(2, 88888.0)}}
	vector := stubScorer{results: []SearchResult{hit(2, 0.71), hit(3, 0.69)}}

	got, err := NewHybridScorer(lexical, vector).Score(context.Background(), "q", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d results, want 3", len(got))
	}

	// doc2 appears in both lists → highest fused score.
	if got[0].Utterance.FirstMessageID != 2 {
		t.Errorf("rank 1 = doc%d, want doc2 (found by both scorers)", got[0].Utterance.FirstMessageID)
	}
	wantDoc2 := rrf(2) + rrf(1)
	if math.Abs(got[0].Score-wantDoc2) > 1e-12 {
		t.Errorf("doc2 score = %v, want %v (1/(k+2) + 1/(k+1))", got[0].Score, wantDoc2)
	}

	// doc1 (lexical rank1) beats doc3 (vector rank2).
	if got[1].Utterance.FirstMessageID != 1 || got[2].Utterance.FirstMessageID != 3 {
		t.Errorf("tail order = doc%d, doc%d; want doc1, doc3",
			got[1].Utterance.FirstMessageID, got[2].Utterance.FirstMessageID)
	}
}

func TestHybridScorerTieBreaksByFirstMessageID(t *testing.T) {
	// Same ranks in disjoint lists → identical fused scores.
	lexical := stubScorer{results: []SearchResult{hit(7, 1.0)}}
	vector := stubScorer{results: []SearchResult{hit(3, 1.0)}}

	got, err := NewHybridScorer(lexical, vector).Score(context.Background(), "q", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Utterance.FirstMessageID != 3 {
		t.Errorf("tie-break order = doc%d first, want doc3 (lower FirstMessageID)", got[0].Utterance.FirstMessageID)
	}
}

func TestHybridScorerPropagatesErrors(t *testing.T) {
	boom := errors.New("boom")
	ok := stubScorer{results: []SearchResult{hit(1, 1.0)}}
	bad := stubScorer{err: boom}

	if _, err := NewHybridScorer(bad, ok).Score(context.Background(), "q", nil); !errors.Is(err, boom) {
		t.Errorf("lexical error not propagated: %v", err)
	}
	if _, err := NewHybridScorer(ok, bad).Score(context.Background(), "q", nil); !errors.Is(err, boom) {
		t.Errorf("vector error not propagated: %v", err)
	}
}

func TestHybridScorerEmptyInputs(t *testing.T) {
	got, err := NewHybridScorer(stubScorer{}, stubScorer{}).Score(context.Background(), "q", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %d results from two empty scorers, want 0", len(got))
	}
}
