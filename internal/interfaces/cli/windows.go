package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const (
	previewAnchors = 3
	previewBefore  = 5
	previewAfter   = 5
)

// Windows shows memory window coverage.
// Without args: summary for all chats.
// With chat-id: detailed view with sample anchor windows.
func (r *Runner) Windows(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return r.windowsSummary(ctx)
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat-id %q: must be an integer", args[0])
	}
	return r.windowsDetail(ctx, chatID)
}

func (r *Runner) windowsSummary(ctx context.Context) error {
	stats, err := r.svc.WindowStats(ctx, 0)
	if err != nil {
		return fmt.Errorf("window stats: %w", err)
	}
	if len(stats) == 0 {
		fmt.Println("No messages found. Run sync first.")
		return nil
	}

	printHeader("Memory Window Coverage")
	fmt.Printf("  %-34s %-20s %6s  %6s  %7s  %5s  %7s\n",
		"CHAT", "SURFACE", "TOTAL", "IN-WIN", "ANCHORS", "%RET", "PENDING")
	printSeparator()

	var totalMsg, totalWin, totalPending int
	for _, s := range stats {
		retPct := retainedPct(s.InWindowCount, s.TotalMessages)
		pendingStr := ""
		if s.PendingRebuild > 0 {
			pendingStr = fmt.Sprintf("%d!", s.PendingRebuild)
		}
		fmt.Printf("  %-34s %-20s %6d  %6d  %7d  %4.1f%%  %7s\n",
			truncate(s.ChatTitle, 34),
			formatSurface(s.Surface),
			s.TotalMessages,
			s.InWindowCount,
			s.AnchorCount,
			retPct,
			pendingStr,
		)
		totalMsg += s.TotalMessages
		totalWin += s.InWindowCount
		totalPending += s.PendingRebuild
	}

	printSeparator()
	fmt.Printf("  %-34s %-20s %6d  %6d  %7s  %4.1f%%  %7d\n",
		"TOTAL", "",
		totalMsg, totalWin, "-",
		retainedPct(totalWin, totalMsg),
		totalPending,
	)

	if totalPending > 0 {
		fmt.Printf("\n  [!] %d messages need retroactive rebuild — run sync to trigger WindowExpander\n", totalPending)
	}
	return nil
}

func (r *Runner) windowsDetail(ctx context.Context, chatID int64) error {
	stats, err := r.svc.WindowStats(ctx, chatID)
	if err != nil {
		return fmt.Errorf("window stats: %w", err)
	}
	if len(stats) == 0 {
		return fmt.Errorf("chat %d not found or has no messages", chatID)
	}
	s := stats[0]

	printHeader(fmt.Sprintf("Window Detail: %q [%s]", s.ChatTitle, formatSurface(s.Surface)))
	fmt.Printf("  Total messages:    %d\n", s.TotalMessages)
	fmt.Printf("  In memory window:  %d  (%.1f%% retained)\n",
		s.InWindowCount, retainedPct(s.InWindowCount, s.TotalMessages))
	fmt.Printf("  Outgoing anchors:  %d\n", s.AnchorCount)
	if s.PendingRebuild > 0 {
		fmt.Printf("  Pending rebuild:   %d  [!] — run sync\n", s.PendingRebuild)
	} else {
		fmt.Printf("  Pending rebuild:   0\n")
	}

	if s.AnchorCount == 0 {
		fmt.Println("\n  No outgoing messages — window computation has no anchors.")
		return nil
	}

	anchors, err := r.svc.WindowAnchors(ctx, chatID, previewBefore, previewAfter, previewAnchors)
	if err != nil {
		return fmt.Errorf("window anchors: %w", err)
	}

	shown := min(previewAnchors, s.AnchorCount)
	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Sample Windows (%d of %d anchors)", shown, s.AnchorCount)))

	for i, anchor := range anchors {
		fmt.Printf("\n[Anchor %d]\n", i+1)
		for _, msg := range anchor.Messages {
			dir := "←"
			tag := ""
			if msg.IsOutgoing {
				dir = "→"
				tag = "  [ANCHOR]"
			} else if !msg.InWindow {
				tag = "  [outside]"
			}
			fmt.Printf("  %s %s  %s%s\n",
				dir,
				msg.SentAt.Format("2006-01-02 15:04"),
				truncate(msg.Text, 55),
				tag,
			)
		}
	}
	return nil
}

func retainedPct(inWindow, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(inWindow) / float64(total) * 100
}

func sectionLine(label string) string {
	prefix := "─── " + label + " "
	pad := max(sepWidth-len(prefix), 0)
	return prefix + strings.Repeat("─", pad)
}
