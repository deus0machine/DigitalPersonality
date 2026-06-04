package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/digital-personality/internal/application/utterance"
)

const (
	embedMinTokens  = 10 // utterances shorter than this are skipped (noise)
	embedBatchDelay = 200 * time.Millisecond
)

// EmbedUtterances generates and stores embeddings for all qualifying in-window utterances.
//
// Qualifying = in_memory_window=TRUE, non-deleted, token_count >= embedMinTokens.
// Idempotent: already-embedded utterances are skipped via FilterUnembedded.
//
// Stats (total / filtered / pending) are always printed regardless of whether
// Ollama is configured, so the command works as a dry-run corpus audit tool.
// Actual embedding only starts if OLLAMA_EMBEDDING_MODEL is set.
//
// Gap drift detection: if UTTERANCE_GAP_SECONDS differs from the value stored in
// existing embeddings, the command exits with an actionable error message.
func (r *Runner) EmbedUtterances(ctx context.Context, _ []string) error {
	// ── 1. Load corpus ────────────────────────────────────────────────────────

	msgs, err := r.utteranceRepo.FetchAllInWindowMessages(ctx)
	if err != nil {
		return fmt.Errorf("fetch messages: %w", err)
	}

	gap := time.Duration(r.utteranceCfg.GapSeconds) * time.Second
	utts := utterance.Build(msgs, gap)

	// ── 2. Filter by minimum token length ────────────────────────────────────

	var candidates []utterance.EmbeddingCandidate
	for _, u := range utts {
		if len([]rune(u.Text))/4 < embedMinTokens {
			continue
		}
		candidates = append(candidates, utterance.EmbeddingCandidate{
			FirstMessageID: u.FirstMessageID,
			Text:           u.Text,
			GapSeconds:     r.utteranceCfg.GapSeconds,
		})
	}

	// ── 3. Filter already-embedded ───────────────────────────────────────────

	allIDs := make([]int64, len(candidates))
	for i, c := range candidates {
		allIDs[i] = c.FirstMessageID
	}
	pendingIDs, err := r.embeddingRepo.FilterUnembedded(ctx, allIDs)
	if err != nil {
		return fmt.Errorf("filter unembedded: %w", err)
	}
	pendingSet := make(map[int64]struct{}, len(pendingIDs))
	for _, id := range pendingIDs {
		pendingSet[id] = struct{}{}
	}
	pending := candidates[:0]
	for _, c := range candidates {
		if _, ok := pendingSet[c.FirstMessageID]; ok {
			pending = append(pending, c)
		}
	}

	// ── 4. Print stats (always, even without API key) ─────────────────────────

	printHeader("Embed Utterances")
	fmt.Printf("  Gap:                 %ds\n", r.utteranceCfg.GapSeconds)
	fmt.Printf("  Total utterances:    %d\n", len(utts))
	fmt.Printf("  After token filter:  %d  (≥%d approx tokens)\n", len(candidates), embedMinTokens)
	fmt.Printf("  Pending embedding:   %d\n", len(pending))

	if r.embedder == nil {
		fmt.Println("\n  OLLAMA_EMBEDDING_MODEL is not set — stats only, no embedding performed.")
		return nil
	}

	if len(pending) == 0 {
		fmt.Println("\n  Nothing to embed — corpus is up to date.")
		return nil
	}
	fmt.Println()

	// ── 5. Check gap drift (only when about to embed) ─────────────────────────

	storedGap, err := r.embeddingRepo.StoredGapSeconds(ctx)
	if err != nil {
		return fmt.Errorf("check stored gap: %w", err)
	}
	if storedGap != 0 && storedGap != r.utteranceCfg.GapSeconds {
		return fmt.Errorf(
			"gap mismatch: stored=%ds current=%ds\n"+
				"  Embeddings were built with a different gap and are now stale.\n"+
				"  Fix: DELETE FROM utterance_embeddings;  then re-run embed-utterances",
			storedGap, r.utteranceCfg.GapSeconds,
		)
	}

	// ── 6. Batch embed + save ─────────────────────────────────────────────────

	modelName := r.ollamaCfg.EmbeddingModel
	batchSize := r.ollamaCfg.EmbedBatchSize
	total := len(pending)
	done := 0

	for i := 0; i < total; i += batchSize {
		end := min(i+batchSize, total)
		batch := pending[i:end]

		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.Text
		}

		vectors, err := r.embedder.EmbedTexts(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}

		if err := r.embeddingRepo.SaveBatch(ctx, batch, vectors, modelName); err != nil {
			return fmt.Errorf("save batch [%d:%d]: %w", i, end, err)
		}

		done += len(batch)
		fmt.Printf("\r  Progress: %d / %d  (batch=%d)", done, total, batchSize)

		if end < total {
			time.Sleep(embedBatchDelay)
		}
	}

	fmt.Printf("\r  Done: %d utterances embedded.        \n\n", done)
	return nil
}
