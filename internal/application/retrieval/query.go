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

// ─── Media inspect types ─────────────────────────────────────────────────────

// MediaKindStat is one row in the global media-kind overview table.
type MediaKindStat struct {
	Kind     string // "text" | "voice" | "video" | "sticker" | "photo" | "round" | ...
	Total    int
	InWindow int
	Outgoing int
}

// MediaChatEntry is one chat row in a per-kind top-chats list.
type MediaChatEntry struct {
	ChatID   int64
	Title    string
	Surface  entity.PersonalitySurface
	Total    int
	InWindow int
	Outgoing int
}

// MediaSurfaceEntry is the count for one personality surface in a per-kind breakdown.
type MediaSurfaceEntry struct {
	Surface  entity.PersonalitySurface
	Total    int
	InWindow int
}

// StickerEmoticonEntry is a top-N emoticon row.
type StickerEmoticonEntry struct {
	Emoticon string
	Count    int
}

// MediaKindDetail is the full breakdown for voice or round media.
type MediaKindDetail struct {
	Total            int
	InWindow         int
	Outgoing         int
	TopChats         []MediaChatEntry // top 5 overall
	TopInterpersonal []MediaChatEntry // top 5 interpersonal surface
	TopSocial        []MediaChatEntry // top 5 social surface
}

// StickerDetail is the breakdown specific to sticker media.
type StickerDetail struct {
	Total        int
	InWindow     int
	Outgoing     int
	TopEmoticons []StickerEmoticonEntry
	TopChats     []MediaChatEntry
	BySurface    []MediaSurfaceEntry
}

// PhotoDetail is the breakdown specific to photo media.
type PhotoDetail struct {
	Total     int
	InWindow  int
	Outgoing  int
	TopChats  []MediaChatEntry
	BySurface []MediaSurfaceEntry
}

// MediaInspectReport is the full media audit snapshot.
type MediaInspectReport struct {
	KindStats []MediaKindStat
	Voice     MediaKindDetail
	Round     MediaKindDetail
	Sticker   StickerDetail
	Photo     PhotoDetail
}

// VoiceChatEntry is one row in the per-chat voice message breakdown.
type VoiceChatEntry struct {
	ChatID        int64
	Title         string
	Surface       entity.PersonalitySurface
	Score         float32
	VoiceCount    int
	OutgoingCount int
	InWindowCount int // voice messages with in_memory_window=TRUE
}

// VoiceSurfaceEntry is the voice in_window count for one personality surface.
type VoiceSurfaceEntry struct {
	Surface      entity.PersonalitySurface
	VoiceInWindow int
}

// VoiceStats is a global summary of voice messages across the database.
type VoiceStats struct {
	TotalVoice    int
	OutgoingVoice int
	VoiceInWindow int              // voice with in_memory_window=TRUE
	BySurface     []VoiceSurfaceEntry
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
	// Window composition breakdown.
	OutgoingInWindow int // user messages inside the window
	IncomingInWindow int // foreign messages inside the window
	// IsolatedInWindow: in-window non-outgoing messages that are reply targets
	// of outgoing messages but are NOT within ±window rows of any anchor.
	// Non-zero indicates step-3 reply targets outside the row-proximity windows.
	IsolatedInWindow int
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

	// Outgoing in-window stickers sent in this chat.
	StickerCount int

	// Top sticker emoticons in outgoing messages: emoticon → count.
	TopStickers map[string]int
}
