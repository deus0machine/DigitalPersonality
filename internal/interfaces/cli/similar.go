package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/retrieval"
)

// Similar finds messages whose text is trigram-similar to the given sample.
// Useful for discovering recurring speech patterns and phrases.
func (r *Runner) Similar(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: similar <text>")
	}
	sample := strings.Join(args, " ")
	hits, err := r.svc.FindSimilar(ctx, sample, retrieval.Query{Limit: 20})
	if err != nil {
		return fmt.Errorf("similar: %w", err)
	}

	if len(hits) == 0 {
		fmt.Printf("\nSimilar to %q → no results\n", sample)
		fmt.Println("Hint: trigram threshold is 0.30. Try a shorter or more distinctive phrase.")
		return nil
	}

	fmt.Printf("\nSimilar to %q → %d result(s)\n", sample, len(hits))
	fmt.Println("(searching for recurring speech patterns — exact match excluded)")
	printSeparator()

	for i, h := range hits {
		fmt.Printf("\n  #%-2d  similarity %.2f  %s  %s\n",
			i+1, h.Rank,
			formatDirection(h.IsOutgoing),
			formatTime(h.SentAt),
		)
		fmt.Printf("       Chat: %s (%s)\n", h.ChatTitle, formatSurface(h.Surface))
		if h.Text != "" {
			fmt.Printf("       %q\n", truncate(h.Text, maxSnippetLen))
		}
	}
	fmt.Println()
	return nil
}
