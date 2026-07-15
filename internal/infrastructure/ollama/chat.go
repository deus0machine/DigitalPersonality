package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/digital-personality/internal/application/persona"
)

// ChatClient calls the Ollama /api/chat endpoint.
// Implements persona.Generator; application layer depends only on that interface.
type ChatClient struct {
	host  string
	model string
	http  *http.Client
}

// NewChat creates a ChatClient with a 5-minute per-request timeout —
// local generation on CPU can be slow for long persona prompts.
func NewChat(host, model string) *ChatClient {
	return &ChatClient{
		host:  host,
		model: model,
		http:  &http.Client{Timeout: 5 * time.Minute},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string          `json:"model"`
	Messages []chatMessage   `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   json.RawMessage `json:"format,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Error   string      `json:"error,omitempty"`
}

// Generate runs one stateless chat completion and returns the raw content.
func (c *ChatClient) Generate(ctx context.Context, req persona.GenerateRequest) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: req.System},
			{Role: "user", Content: req.User},
		},
		Stream: false,
		Format: req.Format,
		Options: map[string]any{
			"temperature": 0.8,
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshal ollama chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ollama chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("ollama chat request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read ollama chat response: %w", err)
	}

	var result chatResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode ollama chat response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := result.Error
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("ollama chat %d: %s", resp.StatusCode, msg)
	}

	return result.Message.Content, nil
}
