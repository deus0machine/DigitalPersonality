package entity

import (
	"encoding/json"
	"time"
)

// MediaKind classifies the primary media attachment of a message.
// Empty string means text-only.
type MediaKind string

const (
	MediaKindNone     MediaKind = ""
	MediaKindPhoto    MediaKind = "photo"
	MediaKindVideo    MediaKind = "video"
	MediaKindVoice    MediaKind = "voice"
	MediaKindRound    MediaKind = "round"    // video message / round
	MediaKindSticker  MediaKind = "sticker"  // animated or static sticker
	MediaKindDocument MediaKind = "document" // file
	MediaKindPoll     MediaKind = "poll"
	MediaKindGeo      MediaKind = "geo"
	MediaKindContact  MediaKind = "contact"
)

// MessageEntity represents a Telegram text-formatting entity (bold, italic, link, etc.)
// Stored as part of the raw layer — never modified.
type MessageEntity struct {
	Type   string // bold | italic | code | pre | url | mention | hashtag | cashtag | ...
	Offset int    // UTF-16 code-unit offset
	Length int    // UTF-16 code-unit length
	URL    string // only for type="url" or type="text_url"
}

// ReactionCount is a single emoji reaction with its count.
type ReactionCount struct {
	Emoji string
	Count int
}

// StickerInfo holds personality-relevant metadata for sticker messages.
type StickerInfo struct {
	SetName  string // sticker pack name (e.g. "PizzaStickers")
	Emoticon string // associated emoji (e.g. "😂")
}

// Message is the raw-layer record of a Telegram message.
// It stores everything as received — no field is normalised or stripped.
// "ок", "👍", short replies, emoji storms, repeated punctuation — all preserved.
type Message struct {
	ID         int64
	TelegramID int64
	ChatID     int64
	SenderID   int64  // 0 for anonymous channel posts and deleted accounts
	ReplyToID  int64  // 0 if not a reply

	// Raw text exactly as received — emojis, capitalization, punctuation intact.
	Text string

	// RawData is a full snapshot JSON of the original Telegram message payload.
	RawData json.RawMessage

	// Rich personality-relevant metadata extracted at ingestion time.
	Entities    []MessageEntity // text formatting entities
	Reactions   []ReactionCount // emoji reactions received on this message
	StickerMeta *StickerInfo   // non-nil when MediaKind == MediaKindSticker
	MediaKind   MediaKind

	// Forward metadata — what the user curates and shares is a personality signal.
	// IsForwarded=true means this message originated elsewhere.
	IsForwarded   bool
	ForwardFromID int64     // original sender ID; 0 for anonymous/channel forwards
	ForwardDate   time.Time // original send time (zero if not forwarded)

	// EditDate is non-zero if the message was subsequently edited.
	// Editing behavior (frequency, latency) is a personality trait.
	EditDate time.Time

	SentAt     time.Time
	SyncedAt   time.Time
	IsOutgoing bool
	IsDeleted  bool
}

// IsEmojiOnly returns true when the message text consists entirely of emoji characters
// with no alphabetic or numeric content. Used for personality signal classification.
func (m *Message) IsEmojiOnly() bool {
	if m.Text == "" {
		return false
	}
	for _, r := range m.Text {
		if !isEmojiRune(r) && r != ' ' && r != '‍' && r != '️' {
			return false
		}
	}
	return true
}

// IsVeryShort returns true for messages under 5 meaningful runes (excluding spaces).
// Such messages are valuable for personality (affirmations like "ок", "👍") but
// are typically not useful for semantic embedding.
func (m *Message) IsVeryShort() bool {
	count := 0
	for _, r := range m.Text {
		if r != ' ' && r != '\n' && r != '\t' {
			count++
			if count >= 5 {
				return false
			}
		}
	}
	return true
}

// isEmojiRune reports whether a rune is in a known emoji Unicode block.
func isEmojiRune(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc symbols and pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and map
		(r >= 0x1F700 && r <= 0x1F77F) || // Alchemical symbols
		(r >= 0x1F780 && r <= 0x1F7FF) || // Geometric shapes extended
		(r >= 0x1F800 && r <= 0x1F8FF) || // Supplemental arrows
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental symbols
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and pictographs extended-A
		(r >= 0x2600 && r <= 0x26FF) || // Misc symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x1F1E0 && r <= 0x1F1FF) // Regional indicator (flags)
}

// SyncCursor tracks the last synchronized message position per chat.
type SyncCursor struct {
	ChatID        int64
	LastMessageID int64
	SyncedAt      time.Time
}
