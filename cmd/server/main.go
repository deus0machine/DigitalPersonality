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
	if args[0] == "transcribe" {
		return runTranscribe()
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

// runTranscribe starts the voice transcription backfill via Telegram Premium STT.
func runTranscribe() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := app.LoggerFromCfg(&cfg.App)
	return app.New(cfg, log).RunTranscribe(context.Background())
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
	case "media-inspect":
		return runner.MediaInspect(ctx)
	case "inspect-utterances":
		return runner.InspectUtterances(ctx, args[1:])
	case "utterance-stats":
		return runner.UtteranceStats(ctx, args[1:])
	case "compare-gaps":
		return runner.CompareGaps(ctx, args[1:])
	case "inspect-bursts":
		return runner.InspectBursts(ctx, args[1:])
	case "retrieve":
		return runner.Retrieve(ctx, args[1:])
	case "retrieve-debug":
		return runner.RetrieveDebug(ctx, args[1:])
	case "retrieve-context":
		return runner.RetrieveContext(ctx, args[1:])
	case "retrieve-context-debug":
		return runner.RetrieveContextDebug(ctx, args[1:])
	case "retrieve-audit":
		return runner.RetrieveAudit(ctx, args[1:])
	case "embed-utterances":
		return runner.EmbedUtterances(ctx, args[1:])
	case "retrieve-vector":
		return runner.RetrieveVector(ctx, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\nUsage:\n"+
			"  sync                              Run Telegram backfill (default)\n"+
			"  transcribe                        Backfill voice transcriptions (Telegram Premium)\n"+
			"  search <query>                    Search messages (FTS → trigram fallback)\n"+
			"  episodes <query>                  Search episodes by semantic text\n"+
			"  similar <text>                    Find messages with similar phrasing\n"+
			"  personality [chat-id]             Show personality analytics\n"+
			"  chats                             List all synced chats with scores\n"+
			"  windows [chat-id]                 Show memory window coverage and sample anchors\n"+
			"  validate                          Run memory quality checks and show top-20 chats\n"+
			"  inspect-chat <chat-id>            Detailed per-chat diagnostic with sample windows\n"+
			"  voice-stats                       Voice message count and top-20 chats by voice volume\n"+
			"  media-inspect                     Full media audit: per-kind stats, top chats\n"+
			"  inspect-utterances <chat-id>      Group in-window messages into utterances and preview\n"+
			"  utterance-stats [chat-id]         Quality metrics for utterance grouping\n"+
			"  compare-gaps <chat-id>            Compare grouping at gap=30s/60s/120s/300s\n"+
			"  inspect-bursts <chat-id>          Top-50 longest bursts to check for over-merging\n"+
			"  retrieve <query>                  BM25 retrieval over all in-window utterances\n"+
			"  retrieve-debug <query>            Same as retrieve + pipeline timing stats\n"+
			"  retrieve-context <query>          Retrieval with surrounding utterance context\n"+
			"  retrieve-context-debug <query>    Same + context token/time metrics\n"+
			"  retrieve-audit                    Run 10 test queries and report retrieval quality\n"+
			"  embed-utterances                  Embed utterances via OpenAI (requires OPENAI_API_KEY)\n"+
			"  retrieve-vector <query>           Semantic retrieval via pgvector (requires OPENAI_API_KEY)",
			args[0])
	}
}
