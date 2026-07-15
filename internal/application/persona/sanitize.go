package persona

import (
	"strings"
	"unicode"
)

// sanitizeHistoryWindow is how many of the persona's own recent messages are
// checked for repeats. Small models loop on their own replies fed back
// through dialog history ("не поня" × 5) — the prompt rule alone is not
// reliably followed, so repeats are dropped programmatically.
const sanitizeHistoryWindow = 8

// sanitizeMessages drops generated messages that repeat the incoming query
// (verbatim echo), an earlier message of the same burst, or one of the
// persona's recent replies. Order is preserved.
func sanitizeMessages(msgs []string, query string, history []Turn) []string {
	recent := recentPersonaTexts(history, sanitizeHistoryWindow)

	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if isRepeat(m, query) {
			continue
		}
		repeated := false
		for _, kept := range out {
			if isRepeat(m, kept) {
				repeated = true
				break
			}
		}
		for _, prev := range recent {
			if repeated {
				break
			}
			if isRepeat(m, prev) {
				repeated = true
			}
		}
		if !repeated {
			out = append(out, m)
		}
	}
	return out
}

func recentPersonaTexts(history []Turn, limit int) []string {
	var texts []string
	for i := len(history) - 1; i >= 0 && len(texts) < limit; i-- {
		if history[i].FromPersona {
			texts = append(texts, history[i].Text)
		}
	}
	return texts
}

// isRepeat reports whether two messages say the same thing.
// Besides exact normalized equality it catches truncated self-repeats
// ("не поня" vs "не понял") via containment with a length-ratio guard.
func isRepeat(a, b string) bool {
	na, nb := normalizeMessage(a), normalizeMessage(b)

	// Emoji-only or punctuation-only messages normalize to "" —
	// compare them raw so a repeated bare "🥺" is still caught.
	if na == "" || nb == "" {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	if na == nb {
		return true
	}

	shorter, longer := na, nb
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if !strings.Contains(longer, shorter) {
		return false
	}
	// Containment counts as a repeat only when lengths are comparable —
	// a short reply legitimately quoting one word of a long message is fine.
	return len([]rune(shorter))*10 >= len([]rune(longer))*6
}

// normalizeMessage lowercases and keeps only letters, digits and single
// spaces, so punctuation and emoji do not defeat repeat detection.
func normalizeMessage(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}
