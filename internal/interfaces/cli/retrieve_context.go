package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/digital-personality/internal/application/utterance"
)

const contextDefaultWindow = 2

// RetrieveContext runs BM25 retrieval and displays each hit with surrounding
// utterances from the same chat for richer memory recall inspection.
//
// Usage: retrieve-context "<query>" [--window N]
func (r *Runner) RetrieveContext(ctx context.Context, args []string) error {
	query, window, err := parseContextArgs(args)
	if err != nil {
		return err
	}

	results, err := r.utSvc.RetrieveWithContext(ctx, query, 0, retrieveDefaultLimit, window)
	if err != nil {
		return fmt.Errorf("retrieve-context: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieval Context: %q  (window=%d)", query, window))
	if len(results) == 0 {
		fmt.Println("  No results found.")
		return nil
	}
	fmt.Printf("  %d result(s)\n", len(results))

	for i, cr := range results {
		printSeparator()
		fmt.Printf("\nRank #%d   score=%.3f   chat=%s\n\n",
			i+1, cr.Hit.Score, cr.Hit.Utterance.ChatTitle)
		printContextBlock(cr)
	}
	printSeparator()
	fmt.Println()
	return nil
}

// RetrieveContextDebug is like RetrieveContext but prepends pipeline metrics.
//
// Usage: retrieve-context-debug "<query>" [--window N]
func (r *Runner) RetrieveContextDebug(ctx context.Context, args []string) error {
	query, window, err := parseContextArgs(args)
	if err != nil {
		return err
	}

	results, stats, err := r.utSvc.RetrieveWithContextAndStats(ctx, query, 0, retrieveDefaultLimit, window)
	if err != nil {
		return fmt.Errorf("retrieve-context-debug: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieval Context Debug: %q  (window=%d)", query, window))
	fmt.Println("  Pipeline stats:")
	fmt.Printf("    Raw messages:       %d\n", stats.RawMessages)
	fmt.Printf("    Utterances built:   %d\n", stats.UtterancesBuilt)
	fmt.Printf("    Avg utt length:     %.1f tokens\n", stats.AvgUtteranceLen)
	fmt.Printf("    Build time:         %s\n", stats.BuildDuration.Round(1e6))
	fmt.Printf("    Score time:         %s\n", stats.ScoreDuration.Round(1e6))
	fmt.Printf("    Context window:     %d\n", stats.ContextWindow)
	fmt.Printf("    Avg context tokens: %.1f\n", stats.AvgContextTokens)
	fmt.Printf("    Context build time: %s\n", stats.ContextDuration.Round(1e6))
	printSeparator()

	if len(results) == 0 {
		fmt.Println("  No results found.")
		return nil
	}
	fmt.Printf("  %d result(s)\n", len(results))

	for i, cr := range results {
		printSeparator()
		fmt.Printf("\nRank #%d   score=%.3f   chat=%s\n\n",
			i+1, cr.Hit.Score, cr.Hit.Utterance.ChatTitle)
		printContextBlock(cr)
	}
	printSeparator()
	fmt.Println()
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func printContextBlock(cr utterance.ContextResult) {
	offset := -len(cr.Before)
	for _, u := range cr.Before {
		printContextLine(fmt.Sprintf("[%+d]", offset), u)
		offset++
	}
	printContextLine("[HIT]", cr.Hit.Utterance)
	offset = 1
	for _, u := range cr.After {
		printContextLine(fmt.Sprintf("[+%d]", offset), u)
		offset++
	}
	fmt.Println()
}

func printContextLine(label string, u utterance.Utterance) {
	dir := "←"
	if u.IsOutgoing {
		dir = "→"
	}
	timeStr := u.StartedAt.Format("15:04")
	burst := ""
	if u.MessageCount > 1 {
		burst = fmt.Sprintf(" (%dm)", u.MessageCount)
	}
	text := truncate(u.Text, 120)
	fmt.Printf("  %-5s  %s %s%s  %s\n", label, dir, timeStr, burst, text)
}

// parseContextArgs extracts query and optional --window N from CLI args.
func parseContextArgs(args []string) (query string, window int, err error) {
	window = contextDefaultWindow
	var queryParts []string

	for i := 0; i < len(args); i++ {
		if args[i] == "--window" {
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("--window requires a numeric value")
			}
			w, e := strconv.Atoi(args[i+1])
			if e != nil || w <= 0 {
				return "", 0, fmt.Errorf("--window must be a positive integer")
			}
			window = w
			i++
		} else {
			queryParts = append(queryParts, args[i])
		}
	}

	query = strings.Join(queryParts, " ")
	if strings.TrimSpace(query) == "" {
		return "", 0, fmt.Errorf("usage: retrieve-context \"<query>\" [--window N]")
	}
	return query, window, nil
}
