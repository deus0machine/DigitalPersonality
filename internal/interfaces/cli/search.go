package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/retrieval"
)

// Search runs FTS + trigram search over messages and prints ranked results.
func (r *Runner) Search(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: search <query>")
	}
	query := strings.Join(args, " ")
	hits, err := r.svc.SearchMessages(ctx, retrieval.Query{Text: query, Limit: 20})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if len(hits) == 0 {
		fmt.Printf("\nSearch: %q → no results\n", query)
		fmt.Println("Hint: try shorter keywords, or use 'similar' for fuzzy matching.")
		return nil
	}

	fmt.Printf("\nSearch: %q → %d result(s)\n", query, len(hits))
	printSeparator()

	for i, h := range hits {
		fmt.Printf("\n  #%-2d  %s  rank %.2f  %s  %s\n",
			i+1,
			formatMatchType(h.MatchType),
			h.Rank,
			formatDirection(h.IsOutgoing),
			formatTime(h.SentAt),
		)
		chatLine := fmt.Sprintf("       Chat: %s (%s)", h.ChatTitle, formatSurface(h.Surface))
		if h.EpisodeID != 0 {
			chatLine += fmt.Sprintf("  ep#%d", h.EpisodeID)
		}
		if h.MediaKind != "" && h.MediaKind != "none" {
			chatLine += fmt.Sprintf("  [%s]", h.MediaKind)
		}
		fmt.Println(chatLine)
		if h.Text != "" {
			fmt.Printf("       %q\n", truncate(h.Text, maxSnippetLen))
		}
	}
	fmt.Println()
	return nil
}
