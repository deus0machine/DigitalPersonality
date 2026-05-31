// Package port defines output ports that the sync application layer depends on.
// Infrastructure packages implement these interfaces; domain and application
// packages reference only these clean contracts — no gotd/td types leak through.
package port

import (
	"context"
	"encoding/json"
	"time"

	"github.com/digital-personality/internal/domain/entity"
)

// ─── Telegram DTO types ───────────────────────────────────────────────────────
// These types live at the port boundary: rich enough to carry all personality
// signal, clean enough to have zero infrastructure dependency.

// DialogInfo is a domain-neutral representation of a Telegram dialog.
// Ownership and participation signals are extracted by infrastructure and carried
// here so the application-layer scorer can make relevance decisions without I/O.
type DialogInfo struct {
	ID         int64
	Type       entity.ChatType
	Title      string
	Username   string
	AccessHash int64 // opaque to application; used by infrastructure for InputPeer construction

	// Ownership / participation signals — extracted from Telegram metadata.
	IsCreator   bool // user is the creator of this chat or channel
	IsAdmin     bool // user has admin rights (but did not create)
	IsBot       bool // counterpart is a bot (private dialogs only)
	IsBroadcast bool // Telegram broadcast flag: channel cannot receive user posts
}

// UserInfo is a domain-neutral representation of a Telegram user.
type UserInfo struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	Phone     string
	IsSelf    bool
}

// MessageEntity is a text-formatting or annotation span within a message.
// Preserving these is critical: bold, italic, code, mentions, URLs etc.
// are part of the user's communication style.
type MessageEntity struct {
	Type   string // bold|italic|code|pre|url|text_url|mention|hashtag|spoiler|...
	Offset int
	Length int
	URL    string // populated for type=text_url
}

// ReactionStat is a single emoji reaction with its aggregate count.
// Which emoji a user reacts with is a strong personality signal.
type ReactionStat struct {
	Emoji string
	Count int
}

// StickerInfo carries personality-relevant sticker metadata.
// Sticker choice is a primary communication style marker.
type StickerInfo struct {
	SetName  string // sticker pack name (requires separate API call to resolve)
	Emoticon string // associated emoji (e.g. "😂")
}

// IncomingMessage is a domain-neutral Telegram message crossing the port boundary.
// All personality-relevant fields are preserved:
//   - Text:        raw as received (emoji, caps, punctuation intact)
//   - Entities:    formatting/annotation spans
//   - Reactions:   which emoji others used to react
//   - MediaKind:   sticker/voice/photo/etc. (stickers are personality, not semantic)
//   - Sticker:     sticker metadata if MediaKind == "sticker"
//   - IsForwarded: curated/shared content signals worldview and taste
//   - EditDate:    non-zero if the user revised this message
type IncomingMessage struct {
	TelegramID int64
	ChatID     int64
	SenderID   int64           // 0 if anonymous / channel post / deleted account
	ReplyToID  int64           // 0 if not a reply
	Text       string          // raw text — never cleaned at this layer
	RawData    json.RawMessage // full snapshot for future mining
	SentAt     time.Time
	IsOutgoing bool

	// Forward metadata — curated content is a strong worldview/taste signal.
	IsForwarded   bool
	ForwardFromID int64     // original sender; 0 for anonymous/channel forwards
	ForwardDate   time.Time // original send time

	// EditDate is non-zero if the message was subsequently edited.
	EditDate time.Time

	// Personality-relevant metadata — preserved from Telegram, never stripped.
	Entities  []MessageEntity
	Reactions []ReactionStat
	MediaKind string       // entity.MediaKind values as string
	Sticker   *StickerInfo // non-nil when MediaKind == "sticker"
}

// HistoryRequest describes parameters for a paginated history fetch.
type HistoryRequest struct {
	Dialog   DialogInfo
	OffsetID int64 // 0 = start from the newest message; paginate by decreasing ID
	Limit    int
}

// HistoryPage is one page of message history returned by GetHistory.
type HistoryPage struct {
	Messages []IncomingMessage
	// Participants contains all users referenced in this page (senders, reply targets, etc.).
	// The sync engine upserts them before processing Messages to prevent FK violations
	// on messages.sender_id. Populated from the Users slice in the Telegram API response.
	Participants []UserInfo
	MinID        int64 // smallest TelegramID in this page — use as next OffsetID
	HasMore      bool  // false when no more messages exist below MinID
}

// TelegramGateway is the output port consumed by the sync application layer.
//
// Contract:
//   - Run establishes the MTProto connection and authenticates once.
//   - Self, ListDialogs, GetHistory MUST be called from within the Run handler.
//   - Implementations handle reconnect, session persistence and flood-wait internally.
type TelegramGateway interface {
	// Run connects, authenticates, then invokes fn with a live session.
	// fn may call any other method on this interface.
	// Run returns when fn returns or ctx is cancelled.
	Run(ctx context.Context, fn func(ctx context.Context) error) error

	// Self returns the authenticated account's profile.
	Self(ctx context.Context) (*UserInfo, error)

	// ListDialogs returns all accessible dialogs (private chats, groups, channels).
	// Results are not guaranteed to be in any order.
	ListDialogs(ctx context.Context) ([]DialogInfo, error)

	// GetHistory returns one page of message history for a dialog.
	// Messages are ordered newest-first within the page.
	GetHistory(ctx context.Context, req HistoryRequest) (*HistoryPage, error)
}
