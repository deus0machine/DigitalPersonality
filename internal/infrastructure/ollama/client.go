// Package ollama provides an HTTP client for the Ollama Embeddings API.
// It implements utterance.Embedder; application layer depends only on that interface.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client calls the Ollama /api/embed endpoint.
// Zero-value is not usable; use New.
type Client struct {
	host  string
	model string
	http  *http.Client
}

// New creates a Client with a 120-second per-request timeout.
// BGE-M3 on CPU can take 10–30 s per batch of 10 texts.
func New(host, model string) *Client {
	return &Client{
		host:  host,
		model: model,
		http:  &http.Client{Timeout: 120 * time.Second},
	}
}

type embedRequest struct {
	Model   string         `json:"model"`
	Input   []string       `json:"input"`
	Options map[string]any `json:"options,omitempty"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

// EmbedTexts embeds texts in a single /api/embed call and returns vectors in input order.
func (c *Client) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// num_gpu=0 pins the embedder to CPU: it is fast enough there for
	// query embedding, and 4GB-class GPUs need every megabyte for the
	// chat model — sharing VRAM pushed chat generation onto the CPU.
	body, err := json.Marshal(embedRequest{
		Model:   c.model,
		Input:   texts,
		Options: map[string]any{"num_gpu": 0},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ollama response: %w", err)
	}

	var result embedResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := result.Error
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("ollama %d: %s", resp.StatusCode, msg)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama returned %d embeddings for %d texts", len(result.Embeddings), len(texts))
	}

	return result.Embeddings, nil
}

// EmbedQuery embeds a single query string.
func (c *Client) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.EmbedTexts(ctx, []string{text})
	if err != nil || len(vecs) == 0 {
		return nil, err
	}
	return vecs[0], nil
}
