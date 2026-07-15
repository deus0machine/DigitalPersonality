package persona

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/digital-personality/internal/application/utterance"
)

const (
	defaultMemoryLimit = 8
	maxReplyMessages   = 6
)

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

// Reply generates the persona's answer to an incoming message.
func (s *Service) Reply(ctx context.Context, query string) (*Reply, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	memories, err := s.retriever.Retrieve(ctx, query, 0, s.memoryLimit)
	if err != nil {
		return nil, fmt.Errorf("retrieve memories: %w", err)
	}

	profile, err := s.style.LoadStyleProfile(ctx, s.burstGapSec)
	if err != nil {
		return nil, fmt.Errorf("load style profile: %w", err)
	}

	raw, err := s.generator.Generate(ctx, GenerateRequest{
		System: BuildSystemPrompt(profile),
		User:   BuildUserPrompt(query, memories),
		Format: json.RawMessage(replySchema),
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
