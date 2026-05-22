package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/retrieval"
)

// Episodes runs FTS over episode_semantic text and prints ranked results.
func (r *Runner) Episodes(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: episodes <query>")
	}
	query := strings.Join(args, " ")
	hits, err := r.svc.SearchEpisodes(ctx, retrieval.Query{Text: query, Limit: 20})
	if err != nil {
		return fmt.Errorf("search episodes: %w", err)
	}

	if len(hits) == 0 {
		fmt.Printf("\nEpisodes: %q → no results\n", query)
		return nil
	}

	fmt.Printf("\nEpisodes: %q → %d result(s)\n", query, len(hits))
	printSeparator()

	for i, h := range hits {
		var durationStr string
		if !h.StartedAt.IsZero() && !h.EndedAt.IsZero() {
			d := h.EndedAt.Sub(h.StartedAt)
			if d > 0 {
				durationStr = " (" + formatDuration(d) + ")"
			}
		}

		endTime := "—"
		if !h.EndedAt.IsZero() {
			endTime = h.EndedAt.Format("15:04")
		}

		fmt.Printf("\n  #%-2d  [fts | %.2f]  %-12s  %s → %s%s  (%d msgs)\n",
			i+1, h.Rank,
			string(h.Type),
			formatTime(h.StartedAt),
			endTime,
			durationStr,
			h.MessageCount,
		)
		fmt.Printf("       Chat: %s (%s)\n", h.ChatTitle, formatSurface(h.Surface))
		if h.SemanticText != "" {
			fmt.Printf("       %q\n", truncate(h.SemanticText, maxSnippetLen))
		}
	}
	fmt.Println()
	return nil
}
