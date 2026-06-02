package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/digital-personality/internal/application/utterance"
)

// UtteranceStats prints quality metrics for the utterance grouping algorithm.
// If chatID is omitted, reports across all in-window messages.
func (r *Runner) UtteranceStats(ctx context.Context, args []string) error {
	msgs, label, err := r.fetchUtteranceMsgs(ctx, args)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		fmt.Printf("\nNo in-window messages found (%s)\n", label)
		return nil
	}

	gap := time.Duration(r.utteranceCfg.GapSeconds) * time.Second
	utts := utterance.Build(msgs, gap)

	printHeader(fmt.Sprintf("Utterance Stats  %s  gap=%ds", label, r.utteranceCfg.GapSeconds))
	fmt.Printf("  Raw messages (in-window):  %d\n", len(msgs))
	fmt.Printf("  Utterances built:          %d\n", len(utts))

	if len(utts) == 0 {
		fmt.Println("  No utterances with semantic content.")
		return nil
	}

	counts := uttCounts(utts)
	sort.Ints(counts)
	fmt.Println()
	fmt.Println("  Messages per utterance:")
	fmt.Printf("    Mean:    %.2f\n", uttMean(counts))
	fmt.Printf("    Median:  %d\n", uttPercentile(counts, 50))
	fmt.Printf("    P90:     %d\n", uttPercentile(counts, 90))
	fmt.Printf("    Max:     %d\n", counts[len(counts)-1])

	d1, d2, d35, d610, d10p := 0, 0, 0, 0, 0
	for _, c := range counts {
		switch {
		case c == 1:
			d1++
		case c == 2:
			d2++
		case c <= 5:
			d35++
		case c <= 10:
			d610++
		default:
			d10p++
		}
	}
	n := len(utts)
	fmt.Println()
	fmt.Println("  Size distribution:")
	fmt.Printf("    1 msg:      %5d  (%s)\n", d1, pct(d1, n))
	fmt.Printf("    2 msgs:     %5d  (%s)\n", d2, pct(d2, n))
	fmt.Printf("    3-5 msgs:   %5d  (%s)\n", d35, pct(d35, n))
	fmt.Printf("    6-10 msgs:  %5d  (%s)\n", d610, pct(d610, n))
	fmt.Printf("    >10 msgs:   %5d  (%s)\n", d10p, pct(d10p, n))

	var hasVoice, voiceOnly, mixed, voiceInBurst int
	for _, u := range utts {
		if !u.HasVoice {
			continue
		}
		hasVoice++
		if u.VoiceCount == u.MessageCount {
			voiceOnly++
		} else {
			mixed++
		}
		if u.MessageCount > 1 {
			voiceInBurst++
		}
	}
	fmt.Println()
	fmt.Println("  Voice utterances:")
	fmt.Printf("    Has voice:          %5d  (%s)\n", hasVoice, pct(hasVoice, n))
	fmt.Printf("    Voice only:         %5d  (%s)\n", voiceOnly, pct(voiceOnly, n))
	fmt.Printf("    Mixed (voice+text):  %5d  (%s)\n", mixed, pct(mixed, n))
	fmt.Printf("    Voice in burst:     %5d  (%s)\n", voiceInBurst, pct(voiceInBurst, n))

	return nil
}

// CompareGaps shows utterance statistics for four gap thresholds to help
// choose the right UTTERANCE_GAP_SECONDS value for a specific chat.
func (r *Runner) CompareGaps(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: compare-gaps <chat-id>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat-id %q: %w", args[0], err)
	}

	msgs, err := r.utteranceRepo.FetchInWindowMessages(ctx, chatID)
	if err != nil {
		return fmt.Errorf("fetch messages: %w", err)
	}
	if len(msgs) == 0 {
		fmt.Printf("\nNo in-window messages for chat %d\n", chatID)
		return nil
	}

	printHeader(fmt.Sprintf("Gap Comparison  chat=%d  (%d raw messages)", chatID, len(msgs)))
	fmt.Printf("  %5s  │  %10s  │  %13s  │  %8s\n", "GAP", "UTTERANCES", "AVG MSGS/UTT", "MULTI%")
	printSeparator()

	for _, gapSec := range []int{30, 60, 120, 300} {
		utts := utterance.Build(msgs, time.Duration(gapSec)*time.Second)
		if len(utts) == 0 {
			fmt.Printf("  %4ds  │  %10s  │  %13s  │  %8s\n", gapSec, "—", "—", "—")
			continue
		}
		counts := uttCounts(utts)
		avg := uttMean(counts)
		multi := 0
		for _, c := range counts {
			if c > 1 {
				multi++
			}
		}
		fmt.Printf("  %4ds  │  %10d  │  %13.2f  │  %7.1f%%\n",
			gapSec, len(utts), avg, float64(multi)*100/float64(len(utts)))
	}

	return nil
}

// InspectBursts shows the top-50 longest utterances (by message count) for a chat.
// Use this to check whether unrelated thoughts are being merged into one group.
func (r *Runner) InspectBursts(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inspect-bursts <chat-id>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat-id %q: %w", args[0], err)
	}

	msgs, err := r.utteranceRepo.FetchInWindowMessages(ctx, chatID)
	if err != nil {
		return fmt.Errorf("fetch messages: %w", err)
	}

	gap := time.Duration(r.utteranceCfg.GapSeconds) * time.Second
	all := utterance.Build(msgs, gap)

	var multis []utterance.Utterance
	for _, u := range all {
		if u.MessageCount > 1 {
			multis = append(multis, u)
		}
	}
	sort.Slice(multis, func(i, j int) bool {
		return multis[i].MessageCount > multis[j].MessageCount
	})

	printHeader(fmt.Sprintf("Top Bursts  chat=%d  gap=%ds", chatID, r.utteranceCfg.GapSeconds))
	fmt.Printf("  Total utterances:        %d\n", len(all))
	fmt.Printf("  Multi-message utterances: %d\n\n", len(multis))

	if len(multis) == 0 {
		fmt.Println("  No multi-message utterances found.")
		return nil
	}

	limit := min(50, len(multis))
	fmt.Printf("  %4s  %5s  %9s  %s\n", "#", "MSGS", "DURATION", "TEXT")
	printSeparator()

	for i, u := range multis[:limit] {
		dur := u.EndedAt.Sub(u.StartedAt).Round(time.Second)
		dir := "←"
		if u.IsOutgoing {
			dir = "→"
		}
		voice := ""
		if u.HasVoice {
			voice = fmt.Sprintf(" [🎙%d]", u.VoiceCount)
		}
		fmt.Printf("  %4d  %5d  %9s  %s%s %s\n",
			i+1, u.MessageCount, formatDuration(dur), dir, voice, truncate(u.Text, 76))
	}

	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (r *Runner) fetchUtteranceMsgs(ctx context.Context, args []string) ([]utterance.MessageInput, string, error) {
	if len(args) > 0 {
		chatID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid chat-id %q: %w", args[0], err)
		}
		msgs, err := r.utteranceRepo.FetchInWindowMessages(ctx, chatID)
		return msgs, fmt.Sprintf("chat=%d", chatID), err
	}
	msgs, err := r.utteranceRepo.FetchAllInWindowMessages(ctx)
	return msgs, "all chats", err
}

func uttCounts(utts []utterance.Utterance) []int {
	out := make([]int, len(utts))
	for i, u := range utts {
		out[i] = u.MessageCount
	}
	return out
}

func uttMean(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}

// uttPercentile uses the nearest-rank method on a pre-sorted slice.
func uttPercentile(sorted []int, p int) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
