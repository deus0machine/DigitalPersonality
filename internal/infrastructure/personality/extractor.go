// Package personality provides the PersonalityExtractor implementation.
// It extracts communication fingerprint signals from raw messages WITHOUT
// any normalization — emoji, punctuation, capitalization are the data here.
package personality

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

// SignalExtractor implements port.PersonalityExtractor.
// All methods are pure functions — no I/O, no state.
type SignalExtractor struct{}

// New returns a ready-to-use SignalExtractor.
func New() port.PersonalityExtractor {
	return &SignalExtractor{}
}

// Extract derives all applicable personality signals from a single raw message.
// Returns an empty slice for messages with no extractable signal (e.g. deleted).
func (e *SignalExtractor) Extract(msg *entity.Message) []entity.PersonalitySignal {
	if msg.ID == 0 {
		return nil
	}

	var signals []entity.PersonalitySignal

	// ── Emoji usage ──────────────────────────────────────────────────────────
	if emojis := collectEmoji(msg.Text); len(emojis) > 0 {
		signals = append(signals, makeSignal(msg.ID, entity.SignalEmojiUsage,
			entity.EmojiUsageValue(emojis)))
	}

	// ── Emoji-only flag ───────────────────────────────────────────────────────
	if msg.IsEmojiOnly() || (msg.Text == "" && msg.MediaKind == entity.MediaKindSticker) {
		signals = append(signals, makeSignal(msg.ID, entity.SignalEmojiOnly, true))
	}

	// ── Punctuation style ─────────────────────────────────────────────────────
	if punc := extractPunctuation(msg.Text); len(punc) > 0 {
		signals = append(signals, makeSignal(msg.ID, entity.SignalPunctuation,
			entity.PunctuationStyleValue(punc)))
	}

	// ── Capitalization ────────────────────────────────────────────────────────
	if cap := analyzeCapitalization(msg.Text); cap != "" {
		signals = append(signals, makeSignal(msg.ID, entity.SignalCapitalization, cap))
	}

	// ── Length class ──────────────────────────────────────────────────────────
	signals = append(signals, makeSignal(msg.ID, entity.SignalLengthClass,
		classifyLength(msg.Text)))

	// ── Media kind ────────────────────────────────────────────────────────────
	if msg.MediaKind != entity.MediaKindNone {
		signals = append(signals, makeSignal(msg.ID, entity.SignalMediaKind,
			string(msg.MediaKind)))
	}

	// ── Sticker usage ─────────────────────────────────────────────────────────
	if msg.StickerMeta != nil {
		signals = append(signals, makeSignal(msg.ID, entity.SignalStickerUsage,
			entity.StickerUsageValue{
				SetName:  msg.StickerMeta.SetName,
				Emoticon: msg.StickerMeta.Emoticon,
			}))
	}

	// ── Repeated character sequences (e.g. "!!!", "...", "ааааа") ─────────────
	if rep := detectRepeatedSequences(msg.Text); len(rep) > 0 {
		signals = append(signals, makeSignal(msg.ID, entity.SignalRepeatedChars, rep))
	}

	// ── Slang / informal markers (Russian informal vocabulary) ────────────────
	if slang := detectSlangMarkers(msg.Text); len(slang) > 0 {
		signals = append(signals, makeSignal(msg.ID, entity.SignalSlangMarkers, slang))
	}

	// ── Has media ─────────────────────────────────────────────────────────────
	if msg.MediaKind != entity.MediaKindNone {
		signals = append(signals, makeSignal(msg.ID, entity.SignalHasMedia, true))
	}

	return signals
}

// ─── Signal construction ──────────────────────────────────────────────────────

func makeSignal(msgID int64, t entity.SignalType, value any) entity.PersonalitySignal {
	raw, _ := json.Marshal(value)
	return entity.PersonalitySignal{
		MessageID: msgID,
		Type:      t,
		Value:     raw,
	}
}

// ─── Feature extractors ───────────────────────────────────────────────────────

// collectEmoji returns all emoji runes found in s, in order of appearance.
// Repeats are preserved — "😂😂😂" returns ["😂","😂","😂"].
func collectEmoji(s string) []string {
	var result []string
	for _, r := range s {
		if isEmojiRune(r) {
			result = append(result, string(r))
		}
	}
	return result
}

// extractPunctuation detects emphasis punctuation patterns.
// "Понял!!!" → {"!!!": 1}  |  "Что?.." → {"?": 1, "..": 1}
func extractPunctuation(s string) map[string]int {
	result := make(map[string]int)

	emphasis := []string{"!!!", "!!", "...", "..", "?!", "!?", "???", "??"}
	lower := strings.ToLower(s)
	for _, p := range emphasis {
		count := strings.Count(lower, p)
		if count > 0 {
			result[p] = count
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// analyzeCapitalization classifies the capitalization pattern of the message text.
// Ignores emoji and punctuation; looks only at letter characters.
func analyzeCapitalization(s string) entity.CapitalizationValue {
	var upper, lower, total int
	for _, r := range s {
		if unicode.IsLetter(r) {
			total++
			if unicode.IsUpper(r) {
				upper++
			} else if unicode.IsLower(r) {
				lower++
			}
		}
	}
	if total == 0 {
		return ""
	}

	switch {
	case upper == total:
		return entity.CapAllCaps
	case upper == 0:
		return entity.CapLower
	case isFirstLetterUpper(s) && lower > upper:
		return entity.CapSentence
	default:
		return entity.CapMixed
	}
}

func isFirstLetterUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return unicode.IsUpper(r)
		}
	}
	return false
}

// classifyLength categorizes message length for personality analysis.
func classifyLength(s string) entity.LengthClass {
	count := 0
	for _, r := range s {
		if !unicode.IsSpace(r) {
			count++
		}
	}
	switch {
	case count < 5:
		return entity.LengthVeryShort
	case count <= 30:
		return entity.LengthShort
	case count <= 150:
		return entity.LengthMedium
	default:
		return entity.LengthLong
	}
}

// detectRepeatedSequences finds repeated character runs that signal emphasis.
// "аааааа" → {"а":6}, "!!!" already caught by punctuation, so focus on letters.
func detectRepeatedSequences(s string) map[string]int {
	if s == "" {
		return nil
	}
	result := make(map[string]int)

	runes := []rune(s)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if !unicode.IsLetter(r) {
			i++
			continue
		}
		j := i + 1
		for j < len(runes) && runes[j] == r {
			j++
		}
		if j-i >= 3 { // at least 3 repetitions
			result[string(r)] = j - i
		}
		i = j
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// detectSlangMarkers identifies common Russian informal markers.
// This is a lightweight signal — NLP-based detection is a future enhancement.
var russianSlang = map[string]struct{}{
	"ахах": {}, "хаха": {}, "хахаха": {}, "ахахах": {},
	"ок": {}, "окей": {}, "ок.": {},
	"ага": {}, "угу": {}, "ну": {},
	"да": {}, "нет": {}, "не": {},
	"лол": {}, "кек": {}, "ору": {},
	"норм": {}, "нормально": {}, "збс": {},
	"пон": {}, "понял": {}, "понятно": {},
	"кст": {}, "кстати": {},
	"имхо": {}, "имхо,": {},
	"щас": {}, "сейчас": {},
	"ладн": {}, "ладно": {},
}

func detectSlangMarkers(s string) []string {
	if s == "" {
		return nil
	}
	words := strings.Fields(strings.ToLower(s))
	seen := make(map[string]bool)
	var found []string
	for _, w := range words {
		// strip trailing punctuation for lookup
		clean := strings.Trim(w, ".,!?;:")
		if _, ok := russianSlang[clean]; ok && !seen[clean] {
			found = append(found, clean)
			seen[clean] = true
		}
	}
	return found
}

// isEmojiRune mirrors the check in normalizer; kept local to avoid coupling.
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
		(r >= 0x1F1E0 && r <= 0x1F1FF) ||
		r == 0x200D || r == 0xFE0F
}
