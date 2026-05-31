package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Validate prints a global quality report for the collected memory.
// Automatic warnings are emitted when metrics fall outside expected ranges.
func (r *Runner) Validate(ctx context.Context) error {
	stats, err := r.svc.Validate(ctx)
	if err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	printHeader("Memory Validation Report")

	winPct := 0.0
	outPct := 0.0
	if stats.TotalMessages > 0 {
		winPct = float64(stats.InWindowMessages) / float64(stats.TotalMessages) * 100
		outPct = float64(stats.OutgoingMessages) / float64(stats.TotalMessages) * 100
	}

	fmt.Printf("  Messages total:        %d\n", stats.TotalMessages)
	fmt.Printf("  In memory window:      %d  (%.1f%%)\n", stats.InWindowMessages, winPct)
	fmt.Printf("  Outgoing (user):       %d  (%.1f%%)\n", stats.OutgoingMessages, outPct)
	fmt.Printf("  Episodes:              %d  (avg %.1f msgs/episode)\n", stats.TotalEpisodes, stats.AvgEpisodeSize)
	fmt.Printf("  Personality signals:   %d\n", stats.TotalSignals)

	if len(stats.ChatsBySurface) > 0 {
		fmt.Println()
		fmt.Println("  Chats by surface:")
		for surface, cnt := range stats.ChatsBySurface {
			fmt.Printf("    %-30s  %d\n", surface, cnt)
		}
	}

	// Automatic warnings.
	var warnings []string
	if stats.TotalMessages > 0 && winPct < 10.0 {
		warnings = append(warnings,
			fmt.Sprintf("only %.1f%% of messages are in memory windows — window computation may not have run", winPct))
	}
	if winPct > 95.0 && stats.TotalMessages > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%.1f%% of messages are in memory windows — verify social/passive_consumption windowing is active", winPct))
	}
	if stats.TotalSignals == 0 {
		warnings = append(warnings, "no personality signals found — Layer 3 extraction may not have run")
	}
	if stats.TotalMessages > 100 && stats.TotalEpisodes == 0 {
		warnings = append(warnings, "no episodes found despite sufficient messages — Layer 4 segmentation may not have run")
	} else if stats.TotalMessages > 500 && stats.TotalEpisodes > 0 {
		if float64(stats.TotalEpisodes)/float64(stats.TotalMessages) < 0.005 {
			warnings = append(warnings,
				fmt.Sprintf("suspiciously few episodes (%d for %d messages) — check episode builder thresholds",
					stats.TotalEpisodes, stats.TotalMessages))
		}
	}
	if len(stats.HighScoreEmpty) > 0 {
		names := make([]string, 0, len(stats.HighScoreEmpty))
		for _, cs := range stats.HighScoreEmpty {
			names = append(names, fmt.Sprintf("%q (score %.2f)", truncate(cs.Title, 24), cs.Score))
		}
		warnings = append(warnings,
			fmt.Sprintf("%d high-score chat(s) have no messages: %s",
				len(stats.HighScoreEmpty), strings.Join(names, ", ")))
	}

	if len(warnings) > 0 {
		fmt.Println()
		fmt.Printf("  WARNINGS (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  [!] %s\n", w)
		}
	} else if stats.TotalMessages > 0 {
		fmt.Println()
		fmt.Println("  All checks passed.")
	}

	// Top-20 chats by volume.
	top, err := r.svc.TopChatsByVolume(ctx, 20)
	if err != nil {
		return fmt.Errorf("top chats: %w", err)
	}
	if len(top) == 0 {
		fmt.Println("\n  No messages found. Run sync first.")
		return nil
	}

	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Top %d Chats by Volume", len(top))))
	fmt.Printf("  %-5s  %-34s  %-22s  %6s  %6s  %4s  %4s\n",
		"Score", "Chat", "Surface", "Total", "InWin", "Out%", "Ep")
	printSeparator()
	for _, e := range top {
		oPct := 0
		if e.Total > 0 {
			oPct = e.Outgoing * 100 / e.Total
		}
		fmt.Printf("  %.2f   %-34s  %-22s  %6d  %6d  %3d%%  %4d\n",
			e.Score,
			truncate(e.Title, 34),
			truncate(formatSurface(e.Surface), 22),
			e.Total,
			e.InWindow,
			oPct,
			e.EpisodeCount,
		)
	}

	return nil
}

// InspectChat prints a detailed diagnostic report for a single chat,
// including message counts, window coverage, and sample anchor windows.
func (r *Runner) InspectChat(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: inspect-chat <chat-id>")
	}
	chatID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat-id %q: must be an integer", args[0])
	}

	rep, err := r.svc.InspectChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("inspect chat: %w", err)
	}

	printHeader(fmt.Sprintf("Chat Inspection: %s", rep.Title))

	winPct := 0.0
	outPct := 0.0
	if rep.Total > 0 {
		winPct = float64(rep.InWindow) / float64(rep.Total) * 100
		outPct = float64(rep.Outgoing) / float64(rep.Total) * 100
	}

	fmt.Printf("  Chat ID:            %d\n", rep.ChatID)
	fmt.Printf("  Surface:            %s\n", formatSurface(rep.Surface))
	fmt.Printf("  Score:              %.2f\n", rep.Score)
	fmt.Println()
	fmt.Printf("  Messages total:     %d\n", rep.Total)
	fmt.Printf("  Outgoing (user):    %d  (%.1f%%)\n", rep.Outgoing, outPct)
	fmt.Printf("  In memory window:   %d  (%.1f%%)\n", rep.InWindow, winPct)

	// ── Episode section ───────────────────────────────────────────────────────
	fmt.Printf("\n%s\n", sectionLine("Episode Quality"))
	if rep.EpisodeCount == 0 {
		fmt.Println("  No episodes found — run sync to trigger episode builder.")
	} else {
		fmt.Printf("  Count:    %d\n", rep.EpisodeCount)
		fmt.Printf("  Min:      %d msg\n", rep.EpisodeMin)
		fmt.Printf("  Avg:      %.1f msg\n", rep.EpisodeAvg)
		fmt.Printf("  Max:      %d msg", rep.EpisodeMax)
		if rep.EpisodeMax > 200 {
			fmt.Print("  [!] very large — possible over-merging")
		}
		fmt.Println()

		if len(rep.LargestEpisodes) > 0 {
			fmt.Println()
			fmt.Printf("  %-10s  %-16s  %-16s  %-8s  %s\n",
				"ID", "Start", "End", "Duration", "Msgs")
			fmt.Println(" ", strings.Repeat("─", 64))
			for _, e := range rep.LargestEpisodes {
				dur := e.EndedAt.Sub(e.StartedAt)
				fmt.Printf("  %-10d  %-16s  %-16s  %-8s  %d\n",
					e.EpisodeID,
					e.StartedAt.Format("2006-01-02 15:04"),
					e.EndedAt.Format("2006-01-02 15:04"),
					formatDuration(dur),
					e.MessageCount,
				)
			}
		}
	}

	// ── Distributed participation windows ─────────────────────────────────────
	anchors, err := r.svc.WindowAnchorsDistributed(ctx, chatID, previewBefore, previewAfter)
	if err != nil {
		return fmt.Errorf("window anchors: %w", err)
	}

	if len(anchors) == 0 {
		fmt.Println("\n  No outgoing messages — no participation windows to display.")
		return nil
	}

	labels := []string{"early", "middle", "late"}
	fmt.Printf("\n%s\n", sectionLine(fmt.Sprintf("Sample Windows — %d anchor(s) distributed across history", len(anchors))))

	for i, anchor := range anchors {
		label := ""
		if i < len(labels) {
			label = " (" + labels[i] + ")"
		}
		fmt.Printf("\n[Anchor %d%s]\n", i+1, label)
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
