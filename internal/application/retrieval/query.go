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

// WindowStat is a per-chat memory window coverage summary.
type WindowStat struct {
	ChatID         int64
	ChatTitle      string
	Surface        entity.PersonalitySurface
	TotalMessages  int
	InWindowCount  int
	AnchorCount    int // outgoing messages that anchor the window
	PendingRebuild int // in-window messages not yet in message_semantic
}

// WindowMessage is one message in a participation window preview.
type WindowMessage struct {
	TelegramID int64
	Text       string
	SentAt     time.Time
	IsOutgoing bool
	InWindow   bool
}

// WindowAnchor is a participation window centered on one outgoing anchor.
// Messages are ordered chronologically; the anchor is the entry where IsOutgoing=true.
type WindowAnchor struct {
	Messages []WindowMessage
}

// VoiceChatEntry is one row in the per-chat voice message breakdown.
type VoiceChatEntry struct {
	ChatID        int64
	Title         string
	Surface       entity.PersonalitySurface
	Score         float32
	VoiceCount    int
	OutgoingCount int
}

// VoiceStats is a global summary of voice messages across the database.
type VoiceStats struct {
	TotalVoice    int
	OutgoingVoice int
	TopChats      []VoiceChatEntry // up to 20 chats by voice count, desc
}

// ValidationStats is a global quality summary of the collected memory.
type ValidationStats struct {
	TotalMessages    int
	InWindowMessages int
	OutgoingMessages int
	TotalEpisodes    int
	AvgEpisodeSize   float64
	TotalSignals     int
	ChatsBySurface   map[string]int
	HighScoreEmpty   []ChatSummary // chats with score > 0.8 and zero messages
}

// ChatSummary is a minimal chat descriptor used in validation warnings.
type ChatSummary struct {
	ChatID  int64
	Title   string
	Surface entity.PersonalitySurface
	Score   float32
}

// TopChatEntry is one row in the top-N chats by message volume table.
type TopChatEntry struct {
	ChatID       int64
	Title        string
	Surface      entity.PersonalitySurface
	Score        float32
	Total        int
	InWindow     int
	Outgoing     int
	EpisodeCount int
}

// EpisodeEntry is a single episode record used in diagnostic output.
type EpisodeEntry struct {
	EpisodeID    int64
	StartedAt    time.Time
	EndedAt      time.Time
	MessageCount int
}

// ChatInspectReport is a detailed per-chat diagnostic snapshot.
type ChatInspectReport struct {
	ChatID       int64
	Title        string
	Surface      entity.PersonalitySurface
	Score        float32
	Total        int
	Outgoing     int
	InWindow     int
	EpisodeCount    int
	EpisodeMin      int
	EpisodeAvg      float64
	EpisodeMax      int
	LargestEpisodes []EpisodeEntry
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
