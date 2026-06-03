// Package utterance provides runtime grouping of consecutive messages into
// semantic utterances. No database writes occur — utterances are in-memory DTOs.
//
// Behavioural layer (raw messages) and semantic layer (utterances) coexist:
// utterances are a view over messages, never a replacement.
package utterance

import (
	"context"
	"strings"
	"time"
)

// MessageInput is the data the builder needs per message.
// Fetched via Repository; carries both identity and semantic fields.
type MessageInput struct {
	ID             int64
	ChatID         int64
	ChatTitle      string // for display in search results
	AuthorID       int64
	SentAt         time.Time
	NormalizedText string
	TokenCount     int
	IsOutgoing     bool
	MediaKind      string // e.g. "voice", "sticker", "" for plain text
}

// Utterance is one complete communicative turn by a single author.
// May span multiple raw messages when the author sends in rapid succession.
type Utterance struct {
	Position     int    // stable index in the Build() result slice; set post-construction
	AuthorID     int64
	ChatID       int64
	ChatTitle    string
	StartedAt    time.Time
	EndedAt      time.Time
	Text         string // space-joined normalized_text of non-empty messages
	MessageCount int    // total messages in group (including empty ones)
	IsOutgoing   bool
	HasVoice     bool // any message in group is a voice message
	VoiceCount   int  // number of voice messages in group
}

// Repository is the data-access port for fetching messages to build utterances.
type Repository interface {
	// FetchInWindowMessages returns all in-window, non-deleted messages for a chat,
	// ordered by sent_at ASC, id ASC, joined with their semantic records.
	FetchInWindowMessages(ctx context.Context, chatID int64) ([]MessageInput, error)

	// FetchAllInWindowMessages returns in-window, non-deleted messages across all chats.
	FetchAllInWindowMessages(ctx context.Context) ([]MessageInput, error)
}

// Build groups msgs into utterances using two rules:
//  1. A different AuthorID always starts a new utterance.
//  2. A gap greater than `gap` between consecutive same-author messages starts a new utterance.
//
// Groups whose every message has TokenCount == 0 are silently dropped —
// they carry no semantic content worth embedding.
// Messages with empty text within a non-empty group are included in MessageCount
// but excluded from the joined Text.
func Build(msgs []MessageInput, gap time.Duration) []Utterance {
	if len(msgs) == 0 {
		return nil
	}

	var (
		result []Utterance
		group  []MessageInput
	)

	flush := func() {
		if u, ok := toUtterance(group); ok {
			result = append(result, u)
		}
		group = group[:0]
	}

	group = append(group, msgs[0])

	for _, m := range msgs[1:] {
		last := group[len(group)-1]
		sameAuthor := m.AuthorID == group[0].AuthorID
		withinGap := m.SentAt.Sub(last.SentAt) <= gap

		if sameAuthor && withinGap {
			group = append(group, m)
		} else {
			flush()
			group = append(group, m)
		}
	}
	flush()

	for i := range result {
		result[i].Position = i
	}
	return result
}

func toUtterance(group []MessageInput) (Utterance, bool) {
	if len(group) == 0 {
		return Utterance{}, false
	}

	var (
		parts      []string
		voiceCount int
	)
	for _, m := range group {
		if m.MediaKind == "voice" {
			voiceCount++
		}
		if m.TokenCount > 0 && strings.TrimSpace(m.NormalizedText) != "" {
			parts = append(parts, m.NormalizedText)
		}
	}
	if len(parts) == 0 {
		return Utterance{}, false
	}

	return Utterance{
		AuthorID:     group[0].AuthorID,
		ChatID:       group[0].ChatID,
		ChatTitle:    group[0].ChatTitle,
		StartedAt:    group[0].SentAt,
		EndedAt:      group[len(group)-1].SentAt,
		Text:         strings.Join(parts, " "),
		MessageCount: len(group),
		IsOutgoing:   group[0].IsOutgoing,
		HasVoice:     voiceCount > 0,
		VoiceCount:   voiceCount,
	}, true
}
