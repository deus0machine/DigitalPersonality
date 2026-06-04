package utterance

import "context"

// Embedder is the application-layer port for generating vector representations.
// Implemented by infrastructure/openai.Client; application never imports that package.
type Embedder interface {
	// EmbedTexts embeds a batch of texts and returns one vector per text,
	// preserving input order.
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery embeds a single search query string.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// EmbeddingCandidate is an utterance ready to be embedded.
type EmbeddingCandidate struct {
	FirstMessageID int64
	Text           string
	GapSeconds     int
}

// VectorHit is a single result from a pgvector ANN search.
type VectorHit struct {
	FirstMessageID int64
	Distance       float64 // cosine distance ∈ [0, 2]; lower = more similar
}

// UtteranceEmbeddingRepository manages storage and retrieval of utterance embeddings.
// Implemented by infrastructure/postgres/repository; application never imports pgx.
type UtteranceEmbeddingRepository interface {
	// FilterUnembedded returns the subset of ids not yet present in utterance_embeddings.
	FilterUnembedded(ctx context.Context, ids []int64) ([]int64, error)

	// SaveBatch persists a batch of embeddings. Idempotent: ON CONFLICT DO NOTHING.
	SaveBatch(ctx context.Context, batch []EmbeddingCandidate, vectors [][]float32, modelName string) error

	// SearchByVector returns the top-limit utterances nearest to vector by cosine distance.
	SearchByVector(ctx context.Context, vector []float32, limit int) ([]VectorHit, error)

	// StoredGapSeconds returns the gap_seconds value from any existing row, or 0 if the
	// table is empty. Used by embed-utterances to detect configuration drift.
	StoredGapSeconds(ctx context.Context) (int, error)
}
