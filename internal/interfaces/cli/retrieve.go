package cli

import (
	"context"
	"fmt"
	"strings"
)

const retrieveDefaultLimit = 10

// Retrieve runs lexical BM25 retrieval over all in-window utterances and
// prints ranked results.
func (r *Runner) Retrieve(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: retrieve \"<query>\"")
	}
	query := strings.Join(args, " ")

	results, err := r.utSvc.Retrieve(ctx, query, 0, retrieveDefaultLimit)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieve: %q", query))
	if len(results) == 0 {
		fmt.Println("  No results found.")
		return nil
	}
	fmt.Printf("  %d result(s)\n", len(results))
	printSeparator()
	for i, res := range results {
		u := res.Utterance
		timeStr := u.StartedAt.Format("2006-01-02 15:04")
		if u.MessageCount > 1 {
			timeStr += " → " + u.EndedAt.Format("15:04")
		}
		fmt.Printf("\nRank #%d   score=%.3f   chat=%s   msgs=%d\n",
			i+1, res.Score, u.ChatTitle, u.MessageCount)
		fmt.Printf("  %s\n", timeStr)
		fmt.Printf("  %s\n", truncate(u.Text, 200))
	}
	fmt.Println()
	return nil
}

// RetrieveDebug is like Retrieve but also prints pipeline metrics above the results.
func (r *Runner) RetrieveDebug(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: retrieve-debug \"<query>\"")
	}
	query := strings.Join(args, " ")

	results, stats, err := r.utSvc.RetrieveWithStats(ctx, query, 0, retrieveDefaultLimit)
	if err != nil {
		return fmt.Errorf("retrieve-debug: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieve Debug: %q", query))
	fmt.Println("  Pipeline stats:")
	fmt.Printf("    Raw messages:      %d\n", stats.RawMessages)
	fmt.Printf("    Utterances built:  %d\n", stats.UtterancesBuilt)
	fmt.Printf("    Avg utt length:    %.1f tokens\n", stats.AvgUtteranceLen)
	fmt.Printf("    Build time:        %s\n", stats.BuildDuration.Round(1e6))
	fmt.Printf("    Score time:        %s\n", stats.ScoreDuration.Round(1e6))
	printSeparator()

	if len(results) == 0 {
		fmt.Println("  No results found.")
		return nil
	}
	fmt.Printf("  %d result(s)\n", len(results))
	printSeparator()
	for i, res := range results {
		u := res.Utterance
		timeStr := u.StartedAt.Format("2006-01-02 15:04")
		if u.MessageCount > 1 {
			timeStr += " → " + u.EndedAt.Format("15:04")
		}
		fmt.Printf("\nRank #%d   score=%.3f   chat=%s   msgs=%d\n",
			i+1, res.Score, u.ChatTitle, u.MessageCount)
		fmt.Printf("  %s\n", timeStr)
		fmt.Printf("  %s\n", truncate(u.Text, 200))
	}
	fmt.Println()
	return nil
}
