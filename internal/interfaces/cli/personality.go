package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/digital-personality/internal/application/retrieval"
)

// Personality prints personality analytics.
// Without args: one-line summary per chat.
// With a chat-id: full detailed report for that chat.
func (r *Runner) Personality(ctx context.Context, args []string) error {
	if len(args) > 0 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid chat-id %q: %w", args[0], err)
		}
		rep, err := r.svc.ChatReport(ctx, id)
		if err != nil {
			return fmt.Errorf("personality: %w", err)
		}
		printDetailedReport(*rep)
		return nil
	}

	reports, err := r.svc.AllReports(ctx)
	if err != nil {
		return fmt.Errorf("personality: %w", err)
	}
	if len(reports) == 0 {
		fmt.Println("\nNo personality data found. Run sync first.")
		return nil
	}
	printPersonalityOverview(reports)
	return nil
}

// printPersonalityOverview renders a compact one-line-per-chat summary table.
func printPersonalityOverview(reports []retrieval.PersonalityReport) {
	printHeader("Personality Overview")
	fmt.Printf("  %-6s  %-22s  %6s  %4s  %4s  %-10s  %5s  %s\n",
		"Score", "Surface", "Msgs", "Out%", "Ep", "Length", "Peak", "Title")
	printSeparator()

	for _, rep := range reports {
		outPct := 0
		if rep.TotalMessages > 0 {
			outPct = rep.OutgoingCount * 100 / rep.TotalMessages
		}
		fmt.Printf("  %.2f   %-22s  %6d  %3d%%  %4d  %-10s  %5s  %s\n",
			rep.Score,
			truncate(formatSurface(rep.Surface), 22),
			rep.TotalMessages,
			outPct,
			rep.EpisodeCount,
			truncate(dominantLengthClass(rep.LengthClassDist), 10),
			peakHour(rep.HourDistribution),
			rep.Title,
		)
	}
	printSeparator()
	fmt.Printf("\n  %d chat(s). Use 'personality <chat-id>' for a detailed report.\n\n", len(reports))
}

// printDetailedReport renders the full analytics for one chat.
func printDetailedReport(rep retrieval.PersonalityReport) {
	printHeader(fmt.Sprintf("Personality Report — %s  (chat_id=%d)", rep.Title, rep.ChatID))
	fmt.Printf("  Surface: %-22s  Score: %.2f\n", formatSurface(rep.Surface), rep.Score)
	fmt.Println()

	outPctStr := ""
	if rep.TotalMessages > 0 {
		outPctStr = fmt.Sprintf(" (%s)", pct(rep.OutgoingCount, rep.TotalMessages))
	}
	fmt.Printf("  Messages : %d total · %d outgoing%s · %d forwarded · %d edited\n",
		rep.TotalMessages, rep.OutgoingCount, outPctStr,
		rep.ForwardedCount, rep.EditedCount,
	)
	fmt.Printf("  Episodes : %d\n", rep.EpisodeCount)
	fmt.Println()

	// Active hours
	if len(rep.HourDistribution) > 0 {
		fmt.Println("  Active Hours (outgoing):")
		maxV := 0
		for _, v := range rep.HourDistribution {
			if v > maxV {
				maxV = v
			}
		}
		for h := range 24 {
			v := rep.HourDistribution[h]
			if v == 0 {
				continue
			}
			b := bar(v, maxV, barWidth)
			fmt.Printf("    %02d:00  %-*s  %d\n", h, barWidth, b, v)
		}
		fmt.Println()
	}

	// Length distribution
	if len(rep.LengthClassDist) > 0 {
		fmt.Println("  Message Length Distribution (outgoing):")
		order := []string{"tiny", "short", "medium", "long", "very_long"}
		total := 0
		for _, v := range rep.LengthClassDist {
			total += v
		}
		maxV := 0
		for _, v := range rep.LengthClassDist {
			if v > maxV {
				maxV = v
			}
		}
		for _, cls := range order {
			v, ok := rep.LengthClassDist[cls]
			if !ok {
				continue
			}
			b := bar(v, maxV, barWidth)
			fmt.Printf("    %-10s  %-*s  %4d  %s\n", cls, barWidth, b, v, pct(v, total))
		}
		fmt.Println()
	}

	// Top emoji
	if len(rep.TopEmoji) > 0 {
		fmt.Println("  Top Emoji:")
		type kv struct {
			k string
			v int
		}
		pairs := make([]kv, 0, len(rep.TopEmoji))
		for k, v := range rep.TopEmoji {
			pairs = append(pairs, kv{k, v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
		limit := min(10, len(pairs))
		for _, p := range pairs[:limit] {
			fmt.Printf("    %s  %d\n", p.k, p.v)
		}
		fmt.Println()
	}

	// Top slang
	if len(rep.TopSlang) > 0 {
		fmt.Println("  Top Slang Markers:")
		type kv struct {
			k string
			v int
		}
		pairs := make([]kv, 0, len(rep.TopSlang))
		for k, v := range rep.TopSlang {
			pairs = append(pairs, kv{k, v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
		limit := min(10, len(pairs))
		for _, p := range pairs[:limit] {
			fmt.Printf("    %-20s  %d\n", p.k, p.v)
		}
		fmt.Println()
	}

	printSeparator()
}
