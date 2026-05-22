package entity

import "time"

type Embedding struct {
	ID        int64
	MessageID int64
	Model     string
	Vector    []float32
	CreatedAt time.Time
}

// EmbeddingRequest is a work item passed to the embedding pipeline.
type EmbeddingRequest struct {
	MessageID int64
	Text      string
	Model     string
}

// SearchResult pairs a message with its vector similarity score.
type SearchResult struct {
	Message    Message
	Similarity float32
}
