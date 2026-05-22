package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"

	appepisode "github.com/digital-personality/internal/application/episode"
	"github.com/digital-personality/internal/application/sync"
	"github.com/digital-personality/internal/config"
	infraepisode "github.com/digital-personality/internal/infrastructure/episode"
	"github.com/digital-personality/internal/infrastructure/normalizer"
	"github.com/digital-personality/internal/infrastructure/personality"
	"github.com/digital-personality/internal/infrastructure/postgres"
	pgrepo "github.com/digital-personality/internal/infrastructure/postgres/repository"
	tginfra "github.com/digital-personality/internal/infrastructure/telegram"
	"github.com/digital-personality/internal/logger"
)

const migrationsPath = "migrations"

// App owns all top-level dependencies and orchestrates startup / shutdown.
type App struct {
	cfg *config.Config
	log *slog.Logger
	db  *postgres.DB
}

// New builds a fully-wired App. Call Run to start it.
func New(cfg *config.Config, log *slog.Logger) *App {
	return &App{cfg: cfg, log: log}
}

// Run starts the application and blocks until SIGINT/SIGTERM or a fatal error.
func (a *App) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := a.initDB(ctx); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer a.shutdown()

	// ── Layer 1: Repositories ─────────────────────────────────────────────────
	userRepo := pgrepo.NewUserRepository(a.db.Pool)
	chatRepo := pgrepo.NewChatRepository(a.db.Pool)
	msgRepo := pgrepo.NewMessageRepository(a.db.Pool)
	semanticRepo := pgrepo.NewSemanticRepository(a.db.Pool)
	personRepo := pgrepo.NewPersonalityRepository(a.db.Pool)
	episodeRepo := pgrepo.NewEpisodeRepository(a.db.Pool)

	// ── Infrastructure: Telegram gateway ──────────────────────────────────────
	tgClient := tginfra.New(a.cfg.Telegram, a.log)

	// ── Infrastructure: normalizer + personality extractor ───────────────────
	// Both are pure CPU-bound implementations — no I/O, safe to construct inline.
	textNormalizer := normalizer.New()
	signalExtractor := personality.New()
	segmenter := infraepisode.New()

	// ── Application: episode builder ──────────────────────────────────────────
	episodeBuilder := appepisode.NewBuilder(episodeRepo, segmenter, textNormalizer, a.log)

	// ── Application: relevance scorer ────────────────────────────────────────
	// Scores each dialog 0.0–1.0 based on ownership signals and chat type.
	// Own channels and Saved Messages score highest; passive broadcast channels lowest.
	scorer := sync.NewChatRelevanceScorer()

	// ── Application: sync engine ──────────────────────────────────────────────
	engine := sync.NewEngine(
		tgClient,
		textNormalizer,
		signalExtractor,
		episodeBuilder,
		scorer,
		msgRepo,
		chatRepo,
		userRepo,
		semanticRepo,
		personRepo,
		a.log,
	)

	a.log.Info("application started")

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := engine.RunBackfill(gctx); err != nil {
			return fmt.Errorf("backfill: %w", err)
		}
		a.log.Info("backfill complete — stopping")
		stop()
		return nil
	})

	if err := g.Wait(); err != nil && err != context.Canceled {
		return fmt.Errorf("application error: %w", err)
	}
	return nil
}

func (a *App) initDB(ctx context.Context) error {
	if err := postgres.Migrate(a.cfg.Postgres.DSN(), migrationsPath, a.log); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	db, err := postgres.Connect(ctx, a.cfg.Postgres, a.log)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	a.db = db
	return nil
}

func (a *App) shutdown() {
	a.log.Info("shutting down")
	if a.db != nil {
		a.db.Close()
	}
}

// LoggerFromCfg returns a context-enriched slog logger.
func LoggerFromCfg(cfg *config.AppConfig) *slog.Logger {
	return logger.New(cfg.LogLevel).With("app", cfg.Name, "env", cfg.Env)
}
