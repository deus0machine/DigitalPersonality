// Package retrieval provides the use case for querying the personality memory.
// It uses PostgreSQL full-text search and trigram similarity — no embeddings required.
// This is the retrieval foundation layer; vector-based retrieval will be additive.
package retrieval

import (
	"time"

	"github.com/digital-personality/internal/domain/entity"
)

// Query describes a retrieval request.
// All fields are optional: unset fields do not constrain the search.
type Query struct {
	// Text is the search input, interpreted as a websearch query:
	// spaces = AND, quotes = phrase, minus = NOT.
	// Example: `"когда ты" свободен -завтра`
	Text string

	// ChatID restricts results to a single chat. 0 = all chats.
	ChatID int64

	// Surface restricts results to a personality surface. "" = all surfaces.
	Surface entity.PersonalitySurface

	// IsOutgoing: nil = both directions, &true = only outgoing, &false = only incoming.
	IsOutgoing *bool

	// MediaKind restricts to a specific media type. "" = any.
	MediaKind string

	// Since / Until bound the time range. Zero value = unbounded.
	Since time.Time
	Until time.Time

	// SimilarityThreshold for trigram search (0.0–1.0). Default 0.30 if zero.
	SimilarityThreshold float32

	Limit int // 0 → uses service default (20)
}

// MessageHit is one result from a message search.
type MessageHit struct {
	MessageID  int64
	ChatID     int64
	ChatTitle  string
	Surface    entity.PersonalitySurface
	Text       string
	SentAt     time.Time
	IsOutgoing bool
	IsForwarded bool
	MediaKind  string

	// Retrieval metadata
	Rank      float32 // FTS ts_rank or trigram similarity score
	MatchType string  // "fts" | "trigram"
	EpisodeID int64   // 0 if not linked to an episode
}

// EpisodeHit is one result from an episode search.
type EpisodeHit struct {
	EpisodeID    int64
	ChatID       int64
	ChatTitle    string
	Surface      entity.PersonalitySurface
	Type         entity.EpisodeType
	SemanticText string
	MessageCount int
	StartedAt    time.Time
	EndedAt      time.Time
	Rank         float32
}

// PersonalityReport is a per-chat analytics snapshot.
type PersonalityReport struct {
	ChatID   int64
	Title    string
	Surface  entity.PersonalitySurface
	Score    float32

	TotalMessages   int
	OutgoingCount   int
	ForwardedCount  int
	EditedCount     int
	EpisodeCount    int

	// Time-of-day distribution: hour (0–23) → outgoing message count.
	HourDistribution map[int]int

	// Top emoji used in outgoing messages: emoji → count.
	TopEmoji map[string]int

	// Writing length distribution: class → count.
	LengthClassDist map[string]int

	// Top slang markers: marker → count.
	TopSlang map[string]int
}
