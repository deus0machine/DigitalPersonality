package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/digital-personality/internal/application/utterance"
)

const inspectUtteranceLimit = 40

// InspectUtterances groups in-window messages for a chat into utterances and
// prints a sample of results with a summary. Useful for validating grouping quality.
func (r *Runner) InspectUtterances(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inspect-utterances <chat-id>")
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
		fmt.Printf("\nNo in-window messages found for chat %d\n", chatID)
		return nil
	}

	gap := time.Duration(r.utteranceCfg.GapSeconds) * time.Second
	utterances := utterance.Build(msgs, gap)

	printHeader(fmt.Sprintf("Utterance Inspect  chat=%d", chatID))
	fmt.Printf("  Raw messages (in-window): %d\n", len(msgs))
	fmt.Printf("  Utterances built:         %d  (gap=%ds)\n", len(utterances), r.utteranceCfg.GapSeconds)

	if len(utterances) == 0 {
		fmt.Println("  No utterances with semantic content found.")
		return nil
	}

	printUtteranceSummary(utterances)
	printSeparator()

	limit := inspectUtteranceLimit
	if len(utterances) < limit {
		limit = len(utterances)
	}
	fmt.Printf("\n  First %d utterances:\n\n", limit)

	for i, u := range utterances[:limit] {
		dir := "←"
		if u.IsOutgoing {
			dir = "→"
		}
		burst := ""
		if u.MessageCount > 1 {
			burst = fmt.Sprintf(" [burst:%d]", u.MessageCount)
		}
		dur := u.EndedAt.Sub(u.StartedAt).Round(time.Second)
		timeRange := u.StartedAt.Format("01-02 15:04:05")
		if u.MessageCount > 1 {
			timeRange += fmt.Sprintf(" +%s", formatDuration(dur))
		}

		fmt.Printf("  [%3d] %s  %s%s\n", i+1, dir, timeRange, burst)
		fmt.Printf("        %s\n\n", truncate(u.Text, 110))
	}

	if len(utterances) > inspectUtteranceLimit {
		fmt.Printf("  … %d more utterances not shown\n", len(utterances)-inspectUtteranceLimit)
	}

	return nil
}

func printUtteranceSummary(utterances []utterance.Utterance) {
	var (
		outgoing   int
		multiMsg   int
		totalMsgs  int
		totalToks  int
	)
	for _, u := range utterances {
		if u.IsOutgoing {
			outgoing++
		}
		if u.MessageCount > 1 {
			multiMsg++
		}
		totalMsgs += u.MessageCount
		totalToks += len([]rune(u.Text)) // rough char count as proxy for tokens
	}
	incoming := len(utterances) - outgoing
	avgMsgs := float64(totalMsgs) / float64(len(utterances))

	fmt.Println()
	fmt.Printf("  Outgoing utterances:      %d\n", outgoing)
	fmt.Printf("  Incoming utterances:      %d\n", incoming)
	fmt.Printf("  Multi-message (bursts):   %d  (%.0f%%)\n",
		multiMsg, float64(multiMsg)*100/float64(len(utterances)))
	fmt.Printf("  Avg messages/utterance:   %.2f\n", avgMsgs)
}
