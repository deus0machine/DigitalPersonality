package cli

import (
	"context"
	"fmt"

	"github.com/digital-personality/internal/application/utterance"
)

// RetrieveAuditVector compares BM25+Rerank retrieval against pure vector retrieval
// on the standard audit queries and reports NEW% — the share of vector results
// absent from the BM25 top-10. This is the Phase 5.3.1 hybrid decision metric:
// NEW% >= 15 → implement HybridScorer (RRF); below → consider episode embeddings.
func (r *Runner) RetrieveAuditVector(ctx context.Context, _ []string) error {
	if r.vectorSvc == nil {
		return fmt.Errorf("OLLAMA_EMBEDDING_MODEL is not set — vector commands require it")
	}

	printHeader("Retrieval Audit: BM25+Rerank vs Vector")
	fmt.Printf("  %d queries  limit=%d  gap=%ds  model=%s\n\n",
		len(auditQueries), auditLimit, r.utteranceCfg.GapSeconds, r.ollamaCfg.EmbeddingModel)

	fmt.Printf("  %s  BM25  VEC  OVERLAP  NEW   NEW%%  │  HYB: BOTH  LEX  VEC\n",
		auditPad("QUERY", auditQueryWidth))
	printSeparator()

	type vectorOnlyHit struct {
		query string
		hit   utterance.SearchResult
	}
	var (
		totalVec, totalNew int
		samples            []vectorOnlyHit
	)

	for _, query := range auditQueries {
		bHits, err := r.utSvc.Retrieve(ctx, query, 0, auditLimit)
		if err != nil {
			return fmt.Errorf("bm25 retrieve %q: %w", query, err)
		}
		vHits, err := r.vectorSvc.Retrieve(ctx, query, 0, auditLimit)
		if err != nil {
			return fmt.Errorf("vector retrieve %q: %w", query, err)
		}
		hHits, err := r.hybridSvc.Retrieve(ctx, query, 0, auditLimit)
		if err != nil {
			return fmt.Errorf("hybrid retrieve %q: %w", query, err)
		}

		inBM25 := make(map[int64]bool, len(bHits))
		for _, h := range bHits {
			inBM25[h.Utterance.FirstMessageID] = true
		}
		inVec := make(map[int64]bool, len(vHits))
		for _, h := range vHits {
			inVec[h.Utterance.FirstMessageID] = true
		}

		newN := 0
		for _, h := range vHits {
			if !inBM25[h.Utterance.FirstMessageID] {
				newN++
				if newN <= 2 {
					samples = append(samples, vectorOnlyHit{query, h})
				}
			}
		}

		// Hybrid top-10 composition relative to the source top-10 lists.
		var hybBoth, hybLex, hybVec int
		for _, h := range hHits {
			id := h.Utterance.FirstMessageID
			switch {
			case inBM25[id] && inVec[id]:
				hybBoth++
			case inBM25[id]:
				hybLex++
			case inVec[id]:
				hybVec++
			}
		}

		newPct := 0.0
		if len(vHits) > 0 {
			newPct = float64(newN) / float64(len(vHits)) * 100
		}
		totalVec += len(vHits)
		totalNew += newN

		fmt.Printf("  %s  %4d  %3d  %7d  %3d  %4.0f%%  │  %9d  %3d  %3d\n",
			auditPad(query, auditQueryWidth),
			len(bHits), len(vHits), len(vHits)-newN, newN, newPct,
			hybBoth, hybLex, hybVec)
	}

	printSeparator()
	totalPct := 0.0
	if totalVec > 0 {
		totalPct = float64(totalNew) / float64(totalVec) * 100
	}
	fmt.Printf("  %s  %4s  %3d  %7d  %3d  %4.0f%%\n",
		auditPad("TOTAL", auditQueryWidth), "",
		totalVec, totalVec-totalNew, totalNew, totalPct)

	fmt.Println("\n  Vector-only hits (top-2 per query):")
	for _, s := range samples {
		u := s.hit.Utterance
		dir := "←"
		if u.IsOutgoing {
			dir = "→"
		}
		fmt.Printf("\n  [%s]  similarity=%.4f  %s  chat=%s\n",
			s.query, s.hit.Score, dir, u.ChatTitle)
		fmt.Printf("    %s\n", truncate(u.Text, 160))
	}

	printSeparator()
	fmt.Println()
	if totalPct >= 15 {
		fmt.Printf("  NEW%% = %.0f%% — vector search finds results BM25 misses.\n", totalPct)
		fmt.Println("  Proceed with Phase 5.3.1: HybridScorer (RRF, k=60) + audit Hybrid column.")
	} else {
		fmt.Printf("  NEW%% = %.0f%% — vector adds little over BM25 on utterances.\n", totalPct)
		fmt.Println("  Consider Phase 5.4: episode embeddings as the semantic retrieval unit.")
	}
	fmt.Println()
	return nil
}
