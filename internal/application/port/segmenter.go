package port

import (
	"github.com/digital-personality/internal/domain/entity"
)

// EpisodeSegment is the output of one segmentation run: a cohesive group of
// messages that the segmenter determined belong to the same conversational episode.
type EpisodeSegment struct {
	Messages  []*entity.Message

	// Classification and provenance
	Type       entity.EpisodeType
	Method     entity.SegmentMethod // dominant boundary signal that started this segment
	Confidence float32              // 0–1; boundary score that triggered the split
}

// EpisodeSegmenter partitions a chronologically sorted slice of messages
// from a single chat into coherent episode segments.
//
// Contract:
//   - Input messages MUST be sorted by sent_at ascending and belong to one chat.
//   - Returns at least one segment if input is non-empty.
//   - The returned segments partition the input: every input message appears in
//     exactly one segment, in original order.
//   - Implementation is pure (no I/O); all state lives in the segment slice.
//   - Implementations must handle interruptions, async gaps, and reply chains.
//
// Why this lives at the port boundary:
//   - The segmentation algorithm is an infrastructure concern (tunable heuristics)
//     but the application layer drives when and how segmentation runs.
//   - Keeping it behind a port allows the algorithm to be replaced (e.g., with an
//     ML-based segmenter) without changing the EpisodeBuilder use case.
type EpisodeSegmenter interface {
	Segment(messages []*entity.Message) []EpisodeSegment
}
