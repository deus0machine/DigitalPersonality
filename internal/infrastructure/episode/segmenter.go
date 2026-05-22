// Package episode provides the rule-based EpisodeSegmenter implementation.
// The segmenter is a pure function — no I/O, no mutable state between calls.
package episode

import (
	"math"
	"time"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

// Config holds all tunable segmentation parameters.
// Exposed so callers can customize per-chat-type without changing the algorithm.
type Config struct {
	// Time gap tiers and their boundary scores.
	HardGap        time.Duration // gap ≥ this → near-certain boundary
	MediumGap      time.Duration
	SoftGap        time.Duration
	HardGapScore   float32 // 0–1 score for each tier
	MediumGapScore float32
	SoftGapScore   float32

	// Subtracted from score when curr replies to a message inside the current segment.
	// Keeps reply threads together across time gaps.
	ReplyChainBonus float32

	// Multiplier applied to score when the message crosses a calendar day boundary.
	// Amplifies weak signals; cannot create a boundary where score == 0.
	DayBoundaryMultiplier float32

	// Boundary is placed when combined score ≥ BoundaryThreshold.
	BoundaryThreshold float32

	// MinSize: segments below this are merged into the preceding one.
	// MaxSize: segments at this size get a forced boundary (score = 1.0).
	MinSize int
	MaxSize int
}

// DefaultConfig is tuned for personal Telegram usage patterns.
var DefaultConfig = Config{
	HardGap:   4 * time.Hour,
	MediumGap: 1 * time.Hour,
	SoftGap:   20 * time.Minute,

	HardGapScore:   0.90,
	MediumGapScore: 0.48,
	SoftGapScore:   0.18,

	ReplyChainBonus: 0.38,

	DayBoundaryMultiplier: 1.40,

	BoundaryThreshold: 0.50,

	MinSize: 2,
	MaxSize: 60,
}

// segment is the internal working unit during segmentation.
// Converted to port.EpisodeSegment in the final pass.
type segment struct {
	msgs       []*entity.Message
	msgIDSet   map[int64]struct{} // O(1) reply-chain membership test
	method     entity.SegmentMethod
	confidence float32
}

func newSegment(first *entity.Message, method entity.SegmentMethod, confidence float32) segment {
	return segment{
		msgs:       []*entity.Message{first},
		msgIDSet:   map[int64]struct{}{first.ID: {}},
		method:     method,
		confidence: confidence,
	}
}

func (s *segment) add(m *entity.Message) {
	s.msgs = append(s.msgs, m)
	s.msgIDSet[m.ID] = struct{}{}
}

func (s *segment) absorb(other segment) {
	for _, m := range other.msgs {
		s.add(m)
	}
}

// RuleBasedSegmenter implements port.EpisodeSegmenter via additive boundary scoring.
type RuleBasedSegmenter struct {
	cfg Config
}

// New returns a segmenter with default configuration.
func New() port.EpisodeSegmenter {
	return NewWithConfig(DefaultConfig)
}

// NewWithConfig returns a segmenter with custom configuration.
func NewWithConfig(cfg Config) port.EpisodeSegmenter {
	return &RuleBasedSegmenter{cfg: cfg}
}

// Segment partitions a chronologically-sorted message slice into episodic segments.
// Every input message appears in exactly one output segment, in original order.
//
// Three-pass algorithm:
//  1. Boundary detection: score each consecutive pair; split when score ≥ threshold.
//  2. Tiny-segment merge: merge segments below MinSize into the preceding one.
//  3. Type classification + conversion to port.EpisodeSegment.
func (s *RuleBasedSegmenter) Segment(messages []*entity.Message) []port.EpisodeSegment {
	if len(messages) == 0 {
		return nil
	}
	if len(messages) == 1 {
		return []port.EpisodeSegment{{
			Messages:   messages,
			Type:       entity.EpisodeMonologue,
			Method:     entity.SegmentInitial,
			Confidence: 1.0,
		}}
	}

	// ── Pass 1: boundary detection ────────────────────────────────────────────
	segments := []segment{newSegment(messages[0], entity.SegmentInitial, 1.0)}

	for i := 1; i < len(messages); i++ {
		prev := messages[i-1]
		curr := messages[i]
		cur := &segments[len(segments)-1]

		score, method := s.boundaryScore(prev, curr, cur.msgIDSet)

		if len(cur.msgs) >= s.cfg.MaxSize {
			score = 1.0
			method = entity.SegmentSizeLimit
		}

		if score >= s.cfg.BoundaryThreshold {
			segments = append(segments, newSegment(curr, method, score))
		} else {
			cur.add(curr)
		}
	}

	// ── Pass 2: merge tiny segments ───────────────────────────────────────────
	segments = s.mergeTiny(segments)

	// ── Pass 3: classify and emit ─────────────────────────────────────────────
	result := make([]port.EpisodeSegment, len(segments))
	for i, seg := range segments {
		result[i] = port.EpisodeSegment{
			Messages:   seg.msgs,
			Type:       classifyType(seg.msgs),
			Method:     seg.method,
			Confidence: seg.confidence,
		}
	}
	return result
}

// boundaryScore computes the 0–1 boundary score for the transition prev → curr.
func (s *RuleBasedSegmenter) boundaryScore(
	prev, curr *entity.Message,
	episodeMsgIDs map[int64]struct{},
) (float32, entity.SegmentMethod) {
	gap := curr.SentAt.Sub(prev.SentAt)

	var score float32
	var method entity.SegmentMethod

	// Time gap component.
	switch {
	case gap >= s.cfg.HardGap:
		score, method = s.cfg.HardGapScore, entity.SegmentTimeHard
	case gap >= s.cfg.MediumGap:
		score, method = s.cfg.MediumGapScore, entity.SegmentTimeMedium
	case gap >= s.cfg.SoftGap:
		score, method = s.cfg.SoftGapScore, entity.SegmentTimeSoft
	}

	// Reply chain suppressor: curr replies to something inside the current segment.
	if curr.ReplyToID != 0 {
		if _, ok := episodeMsgIDs[curr.ReplyToID]; ok {
			score -= s.cfg.ReplyChainBonus
			if score < 0 {
				score = 0
			}
		}
	}

	// Day boundary amplifier: crossing midnight boosts weak signals.
	if score > 0 && !sameDay(prev.SentAt, curr.SentAt) {
		score = float32(math.Min(float64(score)*float64(s.cfg.DayBoundaryMultiplier), 1.0))
		if method == "" {
			method = entity.SegmentDayChange
		}
	}

	return score, method
}

// mergeTiny merges segments with fewer than MinSize messages into the preceding one.
func (s *RuleBasedSegmenter) mergeTiny(segs []segment) []segment {
	if s.cfg.MinSize <= 1 || len(segs) <= 1 {
		return segs
	}
	out := make([]segment, 0, len(segs))
	for _, seg := range segs {
		if len(seg.msgs) < s.cfg.MinSize && len(out) > 0 {
			out[len(out)-1].absorb(seg)
		} else {
			out = append(out, seg)
		}
	}
	return out
}

// classifyType derives EpisodeType from message and timing patterns.
func classifyType(msgs []*entity.Message) entity.EpisodeType {
	if len(msgs) == 0 {
		return entity.EpisodeDialogue
	}
	senders := make(map[int64]struct{}, 4)
	var hasReplies bool
	for _, m := range msgs {
		if m.SenderID != 0 {
			senders[m.SenderID] = struct{}{}
		}
		if m.ReplyToID != 0 {
			hasReplies = true
		}
	}
	duration := msgs[len(msgs)-1].SentAt.Sub(msgs[0].SentAt)

	switch {
	case len(senders) >= 3:
		return entity.EpisodeGroup
	case len(senders) == 1:
		return entity.EpisodeMonologue
	case duration > 24*time.Hour:
		return entity.EpisodeAsync
	case hasReplies:
		return entity.EpisodeThread
	case duration < 5*time.Minute && len(msgs) >= 4:
		return entity.EpisodeBurst
	default:
		return entity.EpisodeDialogue
	}
}

func sameDay(a, b time.Time) bool {
	ya, ma, da := a.UTC().Date()
	yb, mb, db := b.UTC().Date()
	return ya == yb && ma == mb && da == db
}
