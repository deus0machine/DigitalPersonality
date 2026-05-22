// Package normalizer provides the SemanticNormalizer implementation.
// It creates a stripped, lowercased text view of a message for embedding/search —
// without modifying or judging the raw message in any way.
package normalizer

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

// TextNormalizer implements port.SemanticNormalizer.
// It strips emoji, lowercases text, and normalizes whitespace.
// For the personality layer, it deliberately does nothing — that's the extractor's job.
type TextNormalizer struct{}

// New returns a ready-to-use TextNormalizer.
func New() port.SemanticNormalizer {
	return &TextNormalizer{}
}

// Normalize produces a SemanticDocument from a raw message.
// It never returns nil and never modifies the input.
func (n *TextNormalizer) Normalize(msg *entity.Message) *entity.SemanticDocument {
	text := stripEmoji(msg.Text)
	text = strings.ToLower(text)
	text = collapseWhitespace(text)

	tokenCount := countTokens(text)
	lang := detectLanguage(text)
	skip := shouldSkip(msg, text, tokenCount)

	return &entity.SemanticDocument{
		MessageID:      msg.ID,
		NormalizedText: text,
		Language:       lang,
		TokenCount:     tokenCount,
		SkipEmbedding:  skip,
	}
}

// shouldSkip returns true when the message has no semantic retrieval value.
// These messages are still stored and get personality signals — they just won't
// be sent to the embedding model.
func shouldSkip(msg *entity.Message, normalizedText string, tokenCount int) bool {
	// Stickers are pure personality signal — no text to embed.
	if msg.MediaKind == entity.MediaKindSticker {
		return true
	}
	// Voice/round video — transcription not implemented yet.
	if msg.MediaKind == entity.MediaKindVoice || msg.MediaKind == entity.MediaKindRound {
		return true
	}
	// After emoji stripping, if nothing meaningful remains, skip.
	if strings.TrimSpace(normalizedText) == "" {
		return true
	}
	// Very short normalized text (≤ 3 words) rarely carries semantic information
	// useful for retrieval, but they ARE personality signals ("ок", "да", "ага").
	if tokenCount < 3 {
		return true
	}
	return false
}

// stripEmoji removes emoji characters from s.
// The result may have extra spaces where emoji were — collapseWhitespace handles that.
func stripEmoji(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isEmojiRune(r) && !isVariationSelector(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// collapseWhitespace trims and reduces all runs of whitespace to a single space.
func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := true // true = skip leading spaces
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	result := b.String()
	if len(result) > 0 && result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}
	return result
}

// countTokens splits on whitespace and counts non-empty tokens.
func countTokens(s string) int {
	return len(strings.Fields(s))
}

// detectLanguage uses a simple Cyrillic/Latin heuristic.
// A proper language detection library can replace this in Phase 5.
func detectLanguage(s string) string {
	if s == "" {
		return ""
	}
	var cyrillic, latin int
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case r >= 'a' && r <= 'z':
			latin++
		}
	}
	switch {
	case cyrillic == 0 && latin == 0:
		return ""
	case cyrillic > 0 && latin == 0:
		return "ru"
	case latin > 0 && cyrillic == 0:
		return "en"
	default:
		return "mixed"
	}
}

// isEmojiRune checks common emoji Unicode blocks.
// This covers the vast majority of emoji without external dependencies.
func isEmojiRune(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F700 && r <= 0x1F77F) ||
		(r >= 0x1F780 && r <= 0x1F7FF) ||
		(r >= 0x1F800 && r <= 0x1F8FF) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x1FA00 && r <= 0x1FA6F) ||
		(r >= 0x1FA70 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0x1F1E0 && r <= 0x1F1FF) || // flags
		r == 0x200D || // zero-width joiner (used in emoji sequences)
		r == 0xFE0F // variation selector-16 (text → emoji presentation)
}

// isVariationSelector covers variation selectors that modify emoji presentation.
func isVariationSelector(r rune) bool {
	return r >= 0xFE00 && r <= 0xFE0F
}

// ensure utf8 is used (imported for future use in more complex normalization).
var _ = utf8.RuneLen
