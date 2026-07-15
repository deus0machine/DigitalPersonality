package cli

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

const maxTypingPause = 6 * time.Second

// Ask sends a message to the digital persona and prints its reply as a burst
// of separate messages, paced by the person's real intra-burst pause statistics.
//
// Requires OLLAMA_CHAT_MODEL. Retrieval uses hybrid (BM25+vector) when
// OLLAMA_EMBEDDING_MODEL is set, otherwise falls back to BM25+Rerank.
func (r *Runner) Ask(ctx context.Context, args []string) error {
	if r.personaSvc == nil {
		return fmt.Errorf("OLLAMA_CHAT_MODEL is not set — persona requires a chat model (e.g. OLLAMA_CHAT_MODEL=gemma3:4b)")
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: ask \"<сообщение>\"")
	}
	query := strings.Join(args, " ")

	printHeader(fmt.Sprintf("Ask: %q", query))
	fmt.Println("  Persona is typing...")
	fmt.Println()

	start := time.Now()
	reply, err := r.personaSvc.Reply(ctx, query)
	if err != nil {
		return fmt.Errorf("ask: %w", err)
	}

	for i, msg := range reply.Messages {
		if i > 0 {
			pause := samplePause(reply.GapP50Seconds, reply.GapP90Seconds)
			time.Sleep(pause)
		}
		fmt.Printf("  → %s\n", msg)
	}

	fmt.Printf("\n  (%d message(s), generated in %s)\n\n",
		len(reply.Messages), time.Since(start).Round(time.Millisecond))
	return nil
}

// samplePause picks a pause between p50 and p90 of the person's real
// intra-burst gaps, capped so CLI usage never feels stuck.
func samplePause(p50, p90 float64) time.Duration {
	if p90 < p50 {
		p90 = p50
	}
	seconds := p50 + rand.Float64()*(p90-p50)
	return min(time.Duration(seconds*float64(time.Second)), maxTypingPause)
}
