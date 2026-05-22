package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/config"
)

// DB wraps pgxpool and owns the connection lifecycle.
type DB struct {
	Pool *pgxpool.Pool
	log  *slog.Logger
}

// Connect creates and validates a pool using the provided config.
func Connect(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) (*DB, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgxpool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Info("postgres connected", "host", cfg.Host, "db", cfg.DB)
	return &DB{Pool: pool, log: log}, nil
}

// Close gracefully shuts down the connection pool.
func (db *DB) Close() {
	db.log.Info("closing postgres pool")
	db.Pool.Close()
}

// Migrate runs all pending up-migrations from the given source path.
func Migrate(dsn, migrationsPath string, log *slog.Logger) error {
	m, err := migrate.New("file://"+migrationsPath, dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	log.Info("migrations applied", "version", version, "dirty", dirty)
	return nil
}
