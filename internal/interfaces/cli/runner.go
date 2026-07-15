package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/digital-personality/internal/application/persona"
	"github.com/digital-personality/internal/application/retrieval"
	"github.com/digital-personality/internal/application/utterance"
	"github.com/digital-personality/internal/config"
	domrepo "github.com/digital-personality/internal/domain/repository"
	"github.com/digital-personality/internal/infrastructure/ollama"
	"github.com/digital-personality/internal/infrastructure/postgres"
	pgrepo "github.com/digital-personality/internal/infrastructure/postgres/repository"
)

// Runner wires up the minimal dependencies for CLI inspection commands.
// It requires only a PostgreSQL connection — no Telegram session needed.
type Runner struct {
	svc           *retrieval.Service
	utteranceRepo utterance.Repository
	utteranceCfg  config.UtteranceConfig
	rerankCfg     config.RerankConfig
	ollamaCfg     config.OllamaConfig
	utSvc         *utterance.RetrievalService // BM25+Rerank (default retrieval)
	embeddingRepo utterance.UtteranceEmbeddingRepository
	embedder      utterance.Embedder          // nil when OLLAMA_EMBEDDING_MODEL is empty
	vectorSvc     *utterance.RetrievalService // nil when OLLAMA_EMBEDDING_MODEL is empty
	hybridSvc     *utterance.RetrievalService // nil when OLLAMA_EMBEDDING_MODEL is empty
	personaSvc    *persona.Service            // nil when OLLAMA_CHAT_MODEL is empty
	botMsgRepo    domrepo.BotMessageRepository
	db            *postgres.DB
}

// New creates a Runner connected to PostgreSQL.
// Vector/embed commands are enabled only when cfg.Ollama.EmbeddingModel is non-empty.
func New(ctx context.Context, cfg *config.CLIConfig, log *slog.Logger) (*Runner, error) {
	db, err := postgres.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	repo := pgrepo.NewRetrievalRepository(db.Pool)
	svc := retrieval.NewService(repo)
	utRepo := pgrepo.NewUtteranceRepository(db.Pool)
	embRepo := pgrepo.NewUtteranceEmbeddingRepository(db.Pool)

	bm25 := utterance.NewBM25Scorer()
	rerank := utterance.NewRerankScorer(bm25, cfg.Rerank.K, cfg.Rerank.Cap)
	utSvc := utterance.NewRetrievalService(utRepo, rerank, cfg.Utterance)

	var embedder utterance.Embedder
	var vectorSvc, hybridSvc *utterance.RetrievalService
	if cfg.Ollama.EmbeddingModel != "" {
		embedder = ollama.New(cfg.Ollama.Host, cfg.Ollama.EmbeddingModel)
		vectorScorer := utterance.NewVectorScorer(embRepo, embedder)
		vectorSvc = utterance.NewRetrievalService(utRepo, vectorScorer, cfg.Utterance)
		hybridScorer := utterance.NewHybridScorer(rerank, vectorScorer)
		hybridSvc = utterance.NewRetrievalService(utRepo, hybridScorer, cfg.Utterance)
	}

	var personaSvc *persona.Service
	if cfg.Ollama.ChatModel != "" {
		// Hybrid retrieval when embeddings are available, BM25+Rerank otherwise.
		var retriever persona.Retriever = utSvc
		if hybridSvc != nil {
			retriever = hybridSvc
		}
		personaSvc = persona.NewService(
			retriever,
			pgrepo.NewStyleRepository(db.Pool),
			ollama.NewChat(cfg.Ollama.Host, cfg.Ollama.ChatModel),
			cfg.Utterance.GapSeconds,
		)
	}

	return &Runner{
		svc:           svc,
		utteranceRepo: utRepo,
		utteranceCfg:  cfg.Utterance,
		rerankCfg:     cfg.Rerank,
		ollamaCfg:     cfg.Ollama,
		utSvc:         utSvc,
		embeddingRepo: embRepo,
		embedder:      embedder,
		vectorSvc:     vectorSvc,
		hybridSvc:     hybridSvc,
		personaSvc:    personaSvc,
		botMsgRepo:    pgrepo.NewBotMessageRepository(db.Pool),
		db:            db,
	}, nil
}

// Close releases the database connection pool.
func (r *Runner) Close() {
	r.db.Close()
}
