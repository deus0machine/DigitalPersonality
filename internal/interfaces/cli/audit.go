package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/digital-personality/internal/application/utterance"
)

var auditQueries = []string{
	"отношения",
	"работа",
	"программирование",
	"бог",
	"смерть",
	"любовь",
	"деньги",
	"родители",
	"университет",
	"депрессия",
}

const (
	auditLimit         = 10
	auditLongThreshold = 20
	auditQueryWidth    = 18
)

type auditQueryStats struct {
	results  int
	longPct  float64 // % of results with >20 tokens
	multiPct float64 // % of results with MessageCount > 1
	scoreGap float64 // rank1.Score / rankLast.Score; 0 if < 2 results
}

// RetrieveAudit runs all test queries with both BM25-only and BM25+Rerank scorers
// and prints a side-by-side comparison table.
// This lets you measure the actual improvement before committing to reranking.
func (r *Runner) RetrieveAudit(ctx context.Context, _ []string) error {
	// Build a BM25-only service for baseline — runner's utSvc already uses Rerank.
	bm25Svc := utterance.NewRetrievalService(
		r.utteranceRepo,
		utterance.NewBM25Scorer(),
		r.utteranceCfg,
	)
	rerankSvc := utterance.NewRetrievalService(
		r.utteranceRepo,
		utterance.NewRerankScorer(utterance.NewBM25Scorer(), r.rerankCfg.K, r.rerankCfg.Cap),
		r.utteranceCfg,
	)

	printHeader("Retrieval Audit: BM25 vs BM25+Rerank")
	fmt.Printf("  %d queries  limit=%d  gap=%ds  rerank(k=%.0f, cap=%d)\n\n",
		len(auditQueries), auditLimit, r.utteranceCfg.GapSeconds,
		r.rerankCfg.K, r.rerankCfg.Cap)

	// Two-line header
	fmt.Printf("  %s  RES │  ─── BM25 ──────────── │  ─── Rerank ────────────\n",
		auditPad("QUERY", auditQueryWidth))
	fmt.Printf("  %s      │  LONG%%  MULTI%%    GAP  │  LONG%%  MULTI%%    GAP\n",
		auditPad("", auditQueryWidth))
	printSeparator()

	var (
		bm25Stats    []auditQueryStats
		rerankStats  []auditQueryStats
		wordFreqRerank = make(map[string]int)
		totalRerank  int
	)

	for _, query := range auditQueries {
		bHits, _ := bm25Svc.Retrieve(ctx, query, 0, auditLimit)
		rHits, _ := rerankSvc.Retrieve(ctx, query, 0, auditLimit)

		bs := computeAuditStats(bHits)
		rs := computeAuditStats(rHits)
		bm25Stats = append(bm25Stats, bs)
		rerankStats = append(rerankStats, rs)

		for _, h := range rHits {
			for _, w := range auditTokenize(h.Utterance.Text) {
				wordFreqRerank[w]++
			}
			totalRerank++
		}

		resCount := bs.results
		fmt.Printf("  %s  %3d │  %4.0f%%  %5.0f%%  %5.1f  │  %4.0f%%  %5.0f%%  %5.1f\n",
			auditPad(query, auditQueryWidth), resCount,
			bs.longPct, bs.multiPct, bs.scoreGap,
			rs.longPct, rs.multiPct, rs.scoreGap,
		)
	}

	printSeparator()

	// Summary row
	bSum := avgAuditStats(bm25Stats)
	rSum := avgAuditStats(rerankStats)
	fmt.Printf("  %s      │  %4.0f%%  %5.0f%%  %5.1f  │  %4.0f%%  %5.0f%%  %5.1f\n",
		auditPad("SUMMARY (avg)", auditQueryWidth),
		bSum.longPct, bSum.multiPct, bSum.scoreGap,
		rSum.longPct, rSum.multiPct, rSum.scoreGap,
	)

	// Delta row
	fmt.Printf("  %s      │  %29s │  Δ Long: %+.0fpt  Δ Multi: %+.0fpt  Δ Gap: %+.1f\n",
		auditPad("DELTA", auditQueryWidth), "",
		rSum.longPct-bSum.longPct,
		rSum.multiPct-bSum.multiPct,
		rSum.scoreGap-bSum.scoreGap,
	)

	// Top-20 words (reranked results)
	printSeparator()
	fmt.Printf("\n  Top-20 frequent words in reranked results (%d utterances):\n\n", totalRerank)
	printTopWords(wordFreqRerank, 20)

	// Assessment
	printSeparator()
	fmt.Println()
	longGain := rSum.longPct - bSum.longPct
	switch {
	case longGain >= 15:
		fmt.Printf("  Reranking raised Long%% by +%.0fpt — meaningful improvement.\n", longGain)
		fmt.Println("  BM25+Rerank is a solid baseline. Embeddings will add semantic retrieval on top.")
	case longGain >= 5:
		fmt.Printf("  Reranking raised Long%% by +%.0fpt — modest improvement.\n", longGain)
		fmt.Println("  BM25 ceiling is close. Transition to embeddings is the right next step.")
	default:
		fmt.Printf("  Reranking raised Long%% by only +%.0fpt.\n", longGain)
		fmt.Println("  BM25 ceiling confirmed. Move to embeddings.")
	}
	fmt.Println()
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func computeAuditStats(hits []utterance.SearchResult) auditQueryStats {
	if len(hits) == 0 {
		return auditQueryStats{}
	}
	var longN, multiN int
	for _, h := range hits {
		if len(strings.Fields(h.Utterance.Text)) > auditLongThreshold {
			longN++
		}
		if h.Utterance.MessageCount > 1 {
			multiN++
		}
	}
	gap := 0.0
	if len(hits) >= 2 {
		last := hits[len(hits)-1].Score
		if last > 0 {
			gap = hits[0].Score / last
		}
	}
	n := float64(len(hits))
	return auditQueryStats{
		results:  len(hits),
		longPct:  float64(longN) / n * 100,
		multiPct: float64(multiN) / n * 100,
		scoreGap: gap,
	}
}

func avgAuditStats(rows []auditQueryStats) auditQueryStats {
	if len(rows) == 0 {
		return auditQueryStats{}
	}
	var sumLong, sumMulti, sumGap float64
	for _, r := range rows {
		sumLong += r.longPct
		sumMulti += r.multiPct
		sumGap += r.scoreGap
	}
	n := float64(len(rows))
	return auditQueryStats{
		longPct:  sumLong / n,
		multiPct: sumMulti / n,
		scoreGap: sumGap / n,
	}
}

func printTopWords(freq map[string]int, top int) {
	type wf struct {
		word  string
		count int
	}
	wfs := make([]wf, 0, len(freq))
	for w, c := range freq {
		wfs = append(wfs, wf{w, c})
	}
	sort.Slice(wfs, func(i, j int) bool {
		if wfs[i].count != wfs[j].count {
			return wfs[i].count > wfs[j].count
		}
		return wfs[i].word < wfs[j].word
	})
	if top > len(wfs) {
		top = len(wfs)
	}
	maxCnt := 0
	if top > 0 {
		maxCnt = wfs[0].count
	}
	for i, w := range wfs[:top] {
		b := bar(w.count, maxCnt, 20)
		fmt.Printf("  %3d.  %5d  %-20s  %s\n", i+1, w.count, w.word, b)
	}
}

func auditTokenize(s string) []string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, s)
	return strings.Fields(s)
}

func auditPad(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
