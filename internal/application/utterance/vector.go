package utterance

import (
	"context"
	"fmt"
)

const defaultVectorTopK = 50

// VectorScorer retrieves utterances by semantic similarity via pgvector ANN search.
// Implements Scorer so it can be used with RetrievalService without modification.
//
// Score embeds the query, searches utterance_embeddings for nearest neighbours,
// then maps results back to in-memory utterances by FirstMessageID.
// Orphan embeddings (message no longer in-window) are silently skipped.
type VectorScorer struct {
	repo     UtteranceEmbeddingRepository
	embedder Embedder
	topK     int // candidates fetched from pgvector; RetrievalService applies final limit
}

// NewVectorScorer creates a VectorScorer with the default candidate pool size.
func NewVectorScorer(repo UtteranceEmbeddingRepository, embedder Embedder) *VectorScorer {
	return &VectorScorer{repo: repo, embedder: embedder, topK: defaultVectorTopK}
}

func (s *VectorScorer) Score(ctx context.Context, query string, utterances []Utterance) ([]SearchResult, error) {
	byID := make(map[int64]Utterance, len(utterances))
	for _, u := range utterances {
		if u.FirstMessageID != 0 {
			byID[u.FirstMessageID] = u
		}
	}

	vec, err := s.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	hits, err := s.repo.SearchByVector(ctx, vec, s.topK)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}

	results := make([]SearchResult, 0, len(hits))
	for _, h := range hits {
		utt, ok := byID[h.FirstMessageID]
		if !ok {
			continue // orphan embedding — message no longer in_memory_window
		}
		results = append(results, SearchResult{
			Utterance: utt,
			Score:     1.0 - h.Distance, // cosine distance → similarity; higher = better
		})
	}
	return results, nil
}
