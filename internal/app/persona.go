package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/digital-personality/internal/application/persona"
	"github.com/digital-personality/internal/application/utterance"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/infrastructure/ollama"
	"github.com/digital-personality/internal/infrastructure/postgres"
	pgrepo "github.com/digital-personality/internal/infrastructure/postgres/repository"
)

// BuildPersonaService assembles the full persona stack for delivery layers
// (Telegram bot, future HTTP API). Retrieval is hybrid (BM25+Rerank + vector
// via RRF) when an embedding model is configured, BM25+Rerank otherwise.
//
// The returned *postgres.DB must be closed by the caller.
func BuildPersonaService(ctx context.Context, cfg *config.CLIConfig, log *slog.Logger) (*persona.Service, *postgres.DB, error) {
	if cfg.Ollama.ChatModel == "" {
		return nil, nil, fmt.Errorf("OLLAMA_CHAT_MODEL is not set — persona requires a chat model")
	}

	db, err := postgres.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}

	utRepo := pgrepo.NewUtteranceRepository(db.Pool)
	bm25 := utterance.NewBM25Scorer()
	rerank := utterance.NewRerankScorer(bm25, cfg.Rerank.K, cfg.Rerank.Cap)

	var retriever persona.Retriever = utterance.NewRetrievalService(utRepo, rerank, cfg.Utterance)
	if cfg.Ollama.EmbeddingModel != "" {
		embRepo := pgrepo.NewUtteranceEmbeddingRepository(db.Pool)
		embedder := ollama.New(cfg.Ollama.Host, cfg.Ollama.EmbeddingModel)
		vectorScorer := utterance.NewVectorScorer(embRepo, embedder)
		hybrid := utterance.NewHybridScorer(rerank, vectorScorer)
		retriever = utterance.NewRetrievalService(utRepo, hybrid, cfg.Utterance)
	}

	svc := persona.NewService(
		retriever,
		pgrepo.NewStyleRepository(db.Pool),
		ollama.NewChat(cfg.Ollama.Host, cfg.Ollama.ChatModel),
		cfg.Utterance.GapSeconds,
	)
	return svc, db, nil
}
