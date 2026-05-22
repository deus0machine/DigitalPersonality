package entity

import (
	"encoding/json"
	"time"
)

// SignalType identifies a category of personality feature.
// Stored as a TEXT column — extensible without schema changes.
type SignalType string

const (
	// Per-message signals — extracted synchronously during ingestion.

	SignalEmojiUsage      SignalType = "emoji_usage"      // []string: emoji chars found
	SignalPunctuation     SignalType = "punctuation_style" // map[string]int: pattern → count
	SignalCapitalization  SignalType = "capitalization"    // string: all_caps|sentence|lower|mixed
	SignalLengthClass     SignalType = "length_class"      // string: very_short|short|medium|long
	SignalMediaKind       SignalType = "media_kind"        // string: photo|sticker|voice|...
	SignalStickerUsage    SignalType = "sticker_usage"     // {set_name, emoticon}
	SignalSlangMarkers    SignalType = "slang_markers"     // []string: informal words detected
	SignalRepeatedChars   SignalType = "repeated_chars"    // map[string]int: "!!!" → 3
	SignalEmojiOnly       SignalType = "emoji_only"        // bool: message is purely emoji
	SignalHasMedia        SignalType = "has_media"         // bool

	// Future aggregated signals — computed from profile builder (Phase 5+).
	// Listed here for discoverability; not yet extracted.
	SignalResponseTiming SignalType = "response_timing" // timing between messages
	SignalConvRhythm     SignalType = "conv_rhythm"     // burst vs spaced messaging style
)

// PersonalitySignal is a single extracted feature from one message.
// Multiple signals of different types can exist for the same message.
//
// value_json is a flexible JSONB blob whose shape depends on SignalType:
//
//	emoji_usage:      ["😂", "👍", "🔥"]
//	punctuation_style: {"!!!": 1, "...": 2}
//	capitalization:    "all_caps"
//	length_class:      "very_short"
//	sticker_usage:     {"set_name": "...", "emoticon": "😂"}
//	slang_markers:     ["ахах", "ок"]
type PersonalitySignal struct {
	ID          int64
	MessageID   int64
	Type        SignalType
	Value       json.RawMessage
	ExtractedAt time.Time
}

// PersonalityProfile is a per-user aggregated view built from all PersonalitySignals.
// It is re-computed periodically as new messages are ingested.
//
// Features is a flexible JSONB blob; its schema expands over time.
type PersonalityProfile struct {
	UserID      int64
	Features    json.RawMessage
	SignalCount int64
	UpdatedAt   time.Time
}

// ─── Value types for common signals ──────────────────────────────────────────
// These are helpers for constructing Signal.Value — not stored in DB directly.

type EmojiUsageValue []string // list of emoji runes (may repeat)

type PunctuationStyleValue map[string]int // pattern → count, e.g. {"!!!": 2}

type StickerUsageValue struct {
	SetName  string `json:"set_name"`
	Emoticon string `json:"emoticon"`
}

type CapitalizationValue string

const (
	CapAllCaps   CapitalizationValue = "all_caps"   // "ПОНЯЛ"
	CapSentence  CapitalizationValue = "sentence"   // "Понял"
	CapLower     CapitalizationValue = "lower"      // "понял"
	CapMixed     CapitalizationValue = "mixed"      // "пОнЯл" or mixed content
)

type LengthClass string

const (
	LengthVeryShort LengthClass = "very_short" // < 5 non-space runes
	LengthShort     LengthClass = "short"      // 5–30 runes
	LengthMedium    LengthClass = "medium"     // 31–150 runes
	LengthLong      LengthClass = "long"       // > 150 runes
)
