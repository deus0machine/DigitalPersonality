package cli

import (
	"context"
	"fmt"
)

// Chats lists all synced chats with relevance scores and basic stats.
func (r *Runner) Chats(ctx context.Context) error {
	reports, err := r.svc.AllReports(ctx)
	if err != nil {
		return fmt.Errorf("chats: %w", err)
	}

	if len(reports) == 0 {
		fmt.Println("\nNo chats found. Run sync first.")
		return nil
	}

	printHeader("Synced Chats")
	fmt.Printf("  %-6s  %-22s  %6s  %4s  %4s  %s\n",
		"Score", "Surface", "Msgs", "Out%", "Ep", "Title")
	printSeparator()

	totalMsgs := 0
	for _, rep := range reports {
		outPct := 0
		if rep.TotalMessages > 0 {
			outPct = rep.OutgoingCount * 100 / rep.TotalMessages
		}
		fmt.Printf("  %.2f   %-22s  %6d  %3d%%  %4d  %s\n",
			rep.Score,
			truncate(formatSurface(rep.Surface), 22),
			rep.TotalMessages,
			outPct,
			rep.EpisodeCount,
			rep.Title,
		)
		totalMsgs += rep.TotalMessages
	}

	printSeparator()
	fmt.Printf("\n  Total: %d chat(s) · %d messages\n\n", len(reports), totalMsgs)
	return nil
}
