// Package persona assembles the digital personality: it retrieves memories,
// builds a style-grounded prompt, and generates replies that imitate the
// person's real communication style — including multi-message bursts.
//
// The LLM is a stateless generator. Personality emerges from memory,
// retrieval, and the style profile — never from hardcoded prompt personality.
package persona

import "context"

// StyleProfile is a quantitative snapshot of the person's outgoing
// communication style, aggregated over all in-window messages.
type StyleProfile struct {
	// LengthDist: length class → share of outgoing messages (0.0–1.0).
	// Classes: tiny (≤10 chars), short (≤50), medium (≤200), long (≤500), very_long.
	LengthDist map[string]float64

	// TopSlang are the person's most frequent slang markers, descending.
	TopSlang []string

	// TopEmoji are the person's most frequent emoji, descending.
	TopEmoji []string

	// Burst statistics: how many messages the person sends in one turn.
	AvgBurstSize float64
	P90BurstSize float64

	// Intra-burst pause statistics in seconds — pauses between consecutive
	// messages of the same outgoing burst. Used to pace multi-message replies.
	GapP50Seconds float64
	GapP90Seconds float64
}

// StyleRepository loads the aggregated style profile from storage.
type StyleRepository interface {
	// LoadStyleProfile aggregates outgoing in-window messages.
	// burstGapSeconds defines the burst boundary (same as UTTERANCE_GAP_SECONDS).
	LoadStyleProfile(ctx context.Context, burstGapSeconds int) (*StyleProfile, error)
}
