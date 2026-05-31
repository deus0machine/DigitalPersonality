package main

import (
	"context"
	"fmt"
	"os"

	"github.com/joho/godotenv"

	"github.com/digital-personality/internal/app"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/interfaces/cli"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	args := os.Args[1:]
	if len(args) == 0 || args[0] == "sync" {
		return runSync()
	}
	return runCLI(args)
}

// runSync starts the Telegram backfill pipeline (original behavior).
func runSync() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := app.LoggerFromCfg(&cfg.App)
	return app.New(cfg, log).Run(context.Background())
}

// runCLI dispatches inspection commands that only need the database.
func runCLI(args []string) error {
	cfg, err := config.LoadCLI()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := app.LoggerFromCfg(&cfg.App)
	ctx := context.Background()

	runner, err := cli.New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("init cli: %w", err)
	}
	defer runner.Close()

	switch args[0] {
	case "search":
		return runner.Search(ctx, args[1:])
	case "episodes":
		return runner.Episodes(ctx, args[1:])
	case "similar":
		return runner.Similar(ctx, args[1:])
	case "personality":
		return runner.Personality(ctx, args[1:])
	case "chats":
		return runner.Chats(ctx)
	case "windows":
		return runner.Windows(ctx, args[1:])
	case "validate":
		return runner.Validate(ctx)
	case "inspect-chat":
		return runner.InspectChat(ctx, args[1:])
	case "voice-stats":
		return runner.VoiceStats(ctx)
	default:
		return fmt.Errorf("unknown command %q\n\nUsage:\n  sync                      Run Telegram backfill (default)\n  search <query>            Search messages (FTS → trigram fallback)\n  episodes <query>          Search episodes by semantic text\n  similar <text>            Find messages with similar phrasing\n  personality [chat-id]     Show personality analytics\n  chats                     List all synced chats with scores\n  windows [chat-id]         Show memory window coverage and sample anchors\n  validate                  Run memory quality checks and show top-20 chats\n  inspect-chat <chat-id>    Detailed per-chat diagnostic with sample windows\n  voice-stats               Voice message count and top-20 chats by voice volume", args[0])
	}
}
