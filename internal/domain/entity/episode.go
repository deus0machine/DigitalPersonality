package entity

import "time"

// EpisodeType classifies the conversational pattern of an episode.
// Determined after segmentation by analysing participant count, duration, reply density.
type EpisodeType string

const (
	// EpisodeDialogue — bidirectional exchange between two participants.
	EpisodeDialogue EpisodeType = "dialogue"

	// EpisodeMonologue — one participant sends multiple messages without response.
	EpisodeMonologue EpisodeType = "monologue"

	// EpisodeBurst — rapid back-and-forth (all messages within ~5 minutes).
	EpisodeBurst EpisodeType = "burst"

	// EpisodeThread — structured reply chain; messages reference each other.
	EpisodeThread EpisodeType = "thread"

	// EpisodeAsync — conversation spanning multiple days; async cadence.
	EpisodeAsync EpisodeType = "async"

	// EpisodeGroup — three or more distinct participants.
	EpisodeGroup EpisodeType = "group"
)

// SegmentMethod records why the segmenter placed a boundary before this episode.
type SegmentMethod string

const (
	SegmentInitial    SegmentMethod = "initial"        // first episode in chat
	SegmentTimeHard   SegmentMethod = "time_gap_hard"  // gap ≥ HardGapThreshold
	SegmentTimeMedium SegmentMethod = "time_gap_medium" // gap ≥ MediumGapThreshold
	SegmentTimeSoft   SegmentMethod = "time_gap_soft"  // gap ≥ SoftGapThreshold
	SegmentSizeLimit  SegmentMethod = "size_limit"     // MaxSize reached; forced split
	SegmentDayChange  SegmentMethod = "day_boundary"   // crossed midnight amplifier
)

// Episode is a coherent conversational memory unit — the fundamental chunk of
// autobiographical memory. It represents a scene, event, or discussion that a
// person would recall as one thing ("that argument on Tuesday",
// "when we planned the trip", "the meme chain").
//
// Design goals:
//   - Episodes are immutable after creation (messages don't move between episodes).
//   - Summaries and importance scores are computed asynchronously and stored here.
//   - EmotionalValence and Importance are null until the respective pipeline runs.
type Episode struct {
	ID   int64
	ChatID int64

	// Temporal span
	StartedAt time.Time
	EndedAt   time.Time

	// Classification
	Type         EpisodeType
	MessageCount int
	ParticipantIDs []int64

	// Segmentation provenance
	SegmentedBy SegmentMethod
	Confidence  float32

	// Future: async-computed enrichments (null until pipeline runs)
	Importance       *float32 // 0–1; nil = not yet scored
	EmotionalValence string   // positive|negative|neutral|ambiguous; "" = not yet labeled
	Summary          string   // human-readable; "" = not yet summarized
	SummaryModel     string   // which model generated the summary

	CreatedAt time.Time
	UpdatedAt time.Time
}

// EpisodeMessage links a message to its episode with intra-episode ordering.
// The position field preserves the exact conversation order without relying on
// DB row order or timestamp ordering (timestamps can be equal for rapid messages).
type EpisodeMessage struct {
	EpisodeID int64
	MessageID int64
	Position  int
}

// EpisodeSemanticDoc is the Layer 2 (Semantic) representation of an episode.
// It holds a direction-annotated concatenation of member messages' normalized text,
// suitable for generating a single embedding that represents the whole episode.
//
// Format example:
//
//	→ hey when are you free this week
//	← not sure maybe wednesday
//	→ works for me what time
//	← afternoon would be great
//
// The → / ← markers give the embedding model conversational direction context.
type EpisodeSemanticDoc struct {
	EpisodeID     int64
	SemanticText  string
	TokenCount    int
	SkipEmbedding bool // true for single short messages or sticker-only episodes
	CreatedAt     time.Time
}
