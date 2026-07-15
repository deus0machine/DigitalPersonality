package persona

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/digital-personality/internal/application/utterance"
)

const (
	defaultMemoryLimit = 5
	maxReplyMessages   = 6

	// memoryOverfetch compensates for the outgoing-only and content filters:
	// retrieval returns both directions and plenty of filler ("ага", "понял"),
	// but the prompt needs the person's own content-bearing messages —
	// incoming ones caused impersonation, filler ones caused empty loops.
	memoryOverfetch = 6

	// minMemoryWords drops filler utterances from the prompt: a memory that
	// carries no content ("ага, понял") teaches the model nothing but loops.
	minMemoryWords = 4

	// maxGenerateTokens caps generation length: bursts are short by design,
	// and shorter generations are dramatically faster on CPU.
	maxGenerateTokens = 256
)

// Turn is one entry of the live dialog between the persona and its
// interlocutor. Delivery layers accumulate turns and pass them back so the
// persona keeps conversational context (the LLM itself stays stateless).
type Turn struct {
	FromPersona bool
	Text        string
}

// Retriever fetches ranked memories for a query. Satisfied by
// utterance.RetrievalService (hybrid, vector, or BM25 — persona is agnostic).
type Retriever interface {
	Retrieve(ctx context.Context, query string, chatID int64, limit int) ([]utterance.SearchResult, error)
}

// Reply is one persona answer: an ordered burst of Telegram-style messages
// plus pacing hints sampled from the person's real intra-burst pauses.
type Reply struct {
	Messages []string

	// Pacing hints for delivery: realistic pause range between messages.
	GapP50Seconds float64
	GapP90Seconds float64
}

// SamplePause returns a random pause within the persona's real intra-burst
// pause range [P50, P90], capped at max so delivery never feels stuck.
// Delivery layers call this between consecutive burst messages.
func (r *Reply) SamplePause(max time.Duration) time.Duration {
	p50, p90 := r.GapP50Seconds, r.GapP90Seconds
	if p90 < p50 {
		p90 = p50
	}
	seconds := p50 + rand.Float64()*(p90-p50)
	return min(time.Duration(seconds*float64(time.Second)), max)
}

// Service is the persona use case: retrieve memories → build prompt →
// generate a style-faithful multi-message reply.
type Service struct {
	retriever   Retriever
	style       StyleRepository
	generator   Generator
	burstGapSec int
	memoryLimit int
}

// NewService wires the persona pipeline. burstGapSeconds must equal the
// utterance gap so style statistics match the retrieval corpus.
func NewService(retriever Retriever, style StyleRepository, generator Generator, burstGapSeconds int) *Service {
	return &Service{
		retriever:   retriever,
		style:       style,
		generator:   generator,
		burstGapSec: burstGapSeconds,
		memoryLimit: defaultMemoryLimit,
	}
}

// Reply generates the persona's answer to a standalone incoming message.
func (s *Service) Reply(ctx context.Context, query string) (*Reply, error) {
	return s.ReplyWithHistory(ctx, query, nil)
}

// ReplyWithHistory generates the persona's answer given the live dialog so far.
// history is ordered oldest-first and must not include the current query.
func (s *Service) ReplyWithHistory(ctx context.Context, query string, history []Turn) (*Reply, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	retrieved, err := s.retriever.Retrieve(ctx, query, 0, s.memoryLimit*memoryOverfetch)
	if err != nil {
		return nil, fmt.Errorf("retrieve memories: %w", err)
	}

	// Only the person's own content-bearing messages go into the prompt:
	// incoming utterances made the model speak as the interlocutors
	// ("Сереж, а ты меня любишь?"), filler ones fed empty-reply loops.
	memories := make([]utterance.SearchResult, 0, s.memoryLimit)
	for _, m := range retrieved {
		if !m.Utterance.IsOutgoing {
			continue
		}
		if len(strings.Fields(m.Utterance.Text)) < minMemoryWords {
			continue
		}
		memories = append(memories, m)
		if len(memories) >= s.memoryLimit {
			break
		}
	}

	profile, err := s.style.LoadStyleProfile(ctx, s.burstGapSec)
	if err != nil {
		return nil, fmt.Errorf("load style profile: %w", err)
	}

	raw, err := s.generator.Generate(ctx, GenerateRequest{
		System:    BuildSystemPrompt(profile),
		User:      BuildUserPrompt(query, memories, history),
		Format:    json.RawMessage(replySchema),
		MaxTokens: maxGenerateTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("generate reply: %w", err)
	}

	messages := parseReplyMessages(raw)
	if len(messages) == 0 {
		return nil, fmt.Errorf("generator returned no usable messages")
	}

	return &Reply{
		Messages:      messages,
		GapP50Seconds: profile.GapP50Seconds,
		GapP90Seconds: profile.GapP90Seconds,
	}, nil
}

// parseReplyMessages extracts the message burst from generator output.
// Falls back to treating the whole output as a single message when the
// model ignored the JSON contract — a degraded reply beats an error.
func parseReplyMessages(raw string) []string {
	var parsed struct {
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil && len(parsed.Messages) > 0 {
		messages := make([]string, 0, len(parsed.Messages))
		for _, m := range parsed.Messages {
			if trimmed := strings.TrimSpace(m); trimmed != "" {
				messages = append(messages, trimmed)
			}
		}
		if len(messages) > maxReplyMessages {
			messages = messages[:maxReplyMessages]
		}
		return messages
	}

	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		return []string{trimmed}
	}
	return nil
}
