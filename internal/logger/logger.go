package logger

import (
	"context"
	"log/slog"
	"os"
)

type contextKey struct{}

// New builds a structured JSON logger at the given level.
// In development ("debug" level) it uses a human-readable text handler.
func New(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl, AddSource: lvl == slog.LevelDebug}

	var handler slog.Handler
	if lvl == slog.LevelDebug {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// WithContext embeds a logger into a context for propagation across layers.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// FromContext extracts the logger from context.
// Falls back to the default slog logger if none is set.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}
