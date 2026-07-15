package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/digital-personality/internal/app"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/infrastructure/postgres"
	pgrepo "github.com/digital-personality/internal/infrastructure/postgres/repository"
	"github.com/digital-personality/internal/interfaces/bot"
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
	if args[0] == "bot" {
		return runBot()
	}
	return runCLI(args)
}

// runBot starts the Telegram bot delivery layer: the digital persona
// answers incoming messages until interrupted.
func runBot() error {
	cfg, err := config.LoadCLI()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := app.LoggerFromCfg(&cfg.App)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The bot owns a write path (bot_messages) — make sure schema is current.
	if err := postgres.Migrate(cfg.Postgres.DSN(), "migrations", log); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	svc, db, err := app.BuildPersonaService(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("build persona: %w", err)
	}
	defer db.Close()

	msgLog := pgrepo.NewBotMessageRepository(db.Pool)
	b, err := bot.New(cfg.Bot.Token, svc, msgLog, cfg.Bot.AllowedUserIDs, log)
	if err != nil {
		return fmt.Errorf("init bot: %w", err)
	}
	return b.Run(ctx)
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
	case "retrieve-audit-vector":
		return runner.RetrieveAuditVector(ctx, args[1:])
	case "embed-utterances":
		return runner.EmbedUtterances(ctx, args[1:])
	case "retrieve-vector":
		return runner.RetrieveVector(ctx, args[1:])
	case "retrieve-hybrid":
		return runner.RetrieveHybrid(ctx, args[1:])
	case "ask":
		return runner.Ask(ctx, args[1:])
	case "bot-log":
		return runner.BotLog(ctx, args[1:])
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
			"  retrieve-audit-vector             Compare BM25+Rerank vs vector on 10 test queries (NEW%%)\n"+
			"  embed-utterances                  Embed utterances via Ollama (requires OLLAMA_EMBEDDING_MODEL)\n"+
			"  retrieve-vector <query>           Semantic retrieval via pgvector (requires OLLAMA_EMBEDDING_MODEL)\n"+
			"  retrieve-hybrid <query>           BM25+Rerank and vector fused via RRF (requires OLLAMA_EMBEDDING_MODEL)\n"+
			"  ask <message>                     Talk to the digital persona (requires OLLAMA_CHAT_MODEL)\n"+
			"  bot                               Run the Telegram bot: persona answers incoming messages (requires TELEGRAM_BOT_TOKEN)\n"+
			"  bot-log [user-id]                 Show bot conversations: summaries, or one user's dialog",
			args[0])
	}
}
