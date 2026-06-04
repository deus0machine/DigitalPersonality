package cli

import (
	"context"
	"fmt"
	"strings"
)

// RetrieveVector runs semantic retrieval over utterance_embeddings via pgvector ANN search
// and prints the top results ranked by cosine similarity.
//
// Requires OLLAMA_EMBEDDING_MODEL to be set and a populated utterance_embeddings table
// (run embed-utterances first).
func (r *Runner) RetrieveVector(ctx context.Context, args []string) error {
	if r.vectorSvc == nil {
		return fmt.Errorf("OLLAMA_EMBEDDING_MODEL is not set — vector commands require it")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: retrieve-vector \"<query>\"")
	}
	query := strings.Join(args, " ")

	results, stats, err := r.vectorSvc.RetrieveWithStats(ctx, query, 0, retrieveDefaultLimit)
	if err != nil {
		return fmt.Errorf("retrieve-vector: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieve Vector: %q", query))
	fmt.Printf("  Utterances searched: %d  (build: %s  score: %s)\n\n",
		stats.UtterancesBuilt,
		stats.BuildDuration.Round(1e6),
		stats.ScoreDuration.Round(1e6),
	)

	if len(results) == 0 {
		fmt.Println("  No results found.")
		fmt.Println("  Hint: run embed-utterances first if the table is empty.")
		return nil
	}

	printSeparator()
	for i, res := range results {
		u := res.Utterance
		timeStr := u.StartedAt.Format("2006-01-02 15:04")
		if u.MessageCount > 1 {
			timeStr += " → " + u.EndedAt.Format("15:04")
		}
		dir := "←"
		if u.IsOutgoing {
			dir = "→"
		}
		fmt.Printf("\nRank #%d   similarity=%.4f   %s   chat=%s   msgs=%d\n",
			i+1, res.Score, dir, u.ChatTitle, u.MessageCount)
		fmt.Printf("  %s\n", timeStr)
		fmt.Printf("  %s\n", truncate(u.Text, 200))
	}
	fmt.Println()
	return nil
}
