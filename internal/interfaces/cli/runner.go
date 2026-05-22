package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/digital-personality/internal/application/retrieval"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/infrastructure/postgres"
	pgrepo "github.com/digital-personality/internal/infrastructure/postgres/repository"
)

// Runner wires up the minimal dependencies for CLI inspection commands.
// It requires only a PostgreSQL connection — no Telegram session needed.
type Runner struct {
	svc *retrieval.Service
	db  *postgres.DB
}

// New creates a Runner connected to PostgreSQL.
func New(ctx context.Context, cfg *config.CLIConfig, log *slog.Logger) (*Runner, error) {
	db, err := postgres.Connect(ctx, cfg.Postgres, log)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	repo := pgrepo.NewRetrievalRepository(db.Pool)
	svc := retrieval.NewService(repo)
	return &Runner{svc: svc, db: db}, nil
}

// Close releases the database connection pool.
func (r *Runner) Close() {
	r.db.Close()
}
