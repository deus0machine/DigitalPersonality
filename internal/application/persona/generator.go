package persona

import (
	"context"
	"encoding/json"
)

// GenerateRequest is one stateless LLM generation call.
type GenerateRequest struct {
	System string
	User   string
	// Format optionally constrains the output to a JSON schema
	// (supported by Ollama structured outputs). Nil = free text.
	Format json.RawMessage
	// MaxTokens caps generation length; 0 = provider default.
	MaxTokens int
}

// Generator produces text from a prompt. Implementations are stateless;
// all persona context arrives through the request.
// Implemented by infrastructure/ollama.ChatClient.
type Generator interface {
	Generate(ctx context.Context, req GenerateRequest) (string, error)
}
