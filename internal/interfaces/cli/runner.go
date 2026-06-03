package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/digital-personality/internal/application/retrieval"
	"github.com/digital-personality/internal/application/utterance"
	"github.com/digital-personality/internal/config"
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
	utSvc         *utterance.RetrievalService // uses BM25+Rerank by default
	db            *postgres.DB
}

// New creates a Runner connected to PostgreSQL.
func New(ctx context.Context, cfg *config.CLIConfig, log *slog.Logger) (*Runner, error) {
	db, err := postgres.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	repo := pgrepo.NewRetrievalRepository(db.Pool)
	svc := retrieval.NewService(repo)
	utRepo := pgrepo.NewUtteranceRepository(db.Pool)

	// retrieve / retrieve-context use BM25+Rerank by default.
	// retrieve-audit creates its own BM25-only service for baseline comparison.
	bm25 := utterance.NewBM25Scorer()
	rerank := utterance.NewRerankScorer(bm25, cfg.Rerank.K, cfg.Rerank.Cap)
	utSvc := utterance.NewRetrievalService(utRepo, rerank, cfg.Utterance)

	return &Runner{
		svc:           svc,
		utteranceRepo: utRepo,
		utteranceCfg:  cfg.Utterance,
		rerankCfg:     cfg.Rerank,
		utSvc:         utSvc,
		db:            db,
	}, nil
}

// Close releases the database connection pool.
func (r *Runner) Close() {
	r.db.Close()
}
