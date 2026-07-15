package cli

import (
	"context"
	"fmt"
	"strings"
)

// RetrieveHybrid runs hybrid retrieval: BM25+Rerank and vector search fused
// via Reciprocal Rank Fusion (k=60). Results found by both scorers rank higher.
//
// Requires OLLAMA_EMBEDDING_MODEL and a populated utterance_embeddings table.
func (r *Runner) RetrieveHybrid(ctx context.Context, args []string) error {
	if r.hybridSvc == nil {
		return fmt.Errorf("OLLAMA_EMBEDDING_MODEL is not set — hybrid retrieval requires it")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: retrieve-hybrid \"<query>\"")
	}
	query := strings.Join(args, " ")

	results, stats, err := r.hybridSvc.RetrieveWithStats(ctx, query, 0, retrieveDefaultLimit)
	if err != nil {
		return fmt.Errorf("retrieve-hybrid: %w", err)
	}

	printHeader(fmt.Sprintf("Retrieve Hybrid (RRF): %q", query))
	fmt.Printf("  Utterances searched: %d  (build: %s  score: %s)\n\n",
		stats.UtterancesBuilt,
		stats.BuildDuration.Round(1e6),
		stats.ScoreDuration.Round(1e6),
	)

	if len(results) == 0 {
		fmt.Println("  No results found.")
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
		fmt.Printf("\nRank #%d   rrf=%.4f   %s   chat=%s   msgs=%d\n",
			i+1, res.Score, dir, u.ChatTitle, u.MessageCount)
		fmt.Printf("  %s\n", timeStr)
		fmt.Printf("  %s\n", truncate(u.Text, 200))
	}
	fmt.Println()
	return nil
}
