package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	App           AppConfig
	Postgres      PostgresConfig
	Telegram      TelegramConfig
	OpenAI        OpenAIConfig
	Embedding     EmbeddingConfig
	Window        WindowConfig
	Sync          SyncConfig
	Transcription TranscriptionConfig
	Utterance     UtteranceConfig
	Rerank        RerankConfig
}

// SyncConfig controls Telegram history pagination rate limiting and FLOOD_WAIT handling.
type SyncConfig struct {
	// HistoryRequestDelay is the pause between consecutive GetHistory page requests
	// within a single dialog. Acts as a proactive throttle to reduce FLOOD_WAIT frequency.
	HistoryRequestDelay time.Duration `env:"SYNC_HISTORY_REQUEST_DELAY" envDefault:"200ms"`

	// FloodMaxRetries is the maximum number of times a GetHistory call is retried
	// after receiving a FLOOD_WAIT error before the dialog is marked as failed.
	FloodMaxRetries int `env:"SYNC_FLOOD_MAX_RETRIES" envDefault:"5"`

	// FloodJitter is added to the FLOOD_WAIT duration on every retry to prevent
	// synchronized retry storms across multiple concurrent callers.
	FloodJitter time.Duration `env:"SYNC_FLOOD_JITTER" envDefault:"1s"`

	// FloodBackoffMultiplier scales the sleep duration on each successive retry.
	// A value of 1.5 means each retry waits 1.5× longer than the previous one.
	FloodBackoffMultiplier float64 `env:"SYNC_FLOOD_BACKOFF_MULT" envDefault:"1.5"`
}

// TranscriptionConfig controls the voice transcription backfill worker.
type TranscriptionConfig struct {
	// BatchSize is the number of voice messages fetched per worker iteration.
	BatchSize int `env:"TRANSCRIPTION_BATCH_SIZE" envDefault:"10"`

	// RequestDelay is the pause inserted between consecutive transcribeAudio calls
	// to avoid hitting undocumented rate limits.
	RequestDelay time.Duration `env:"TRANSCRIPTION_REQUEST_DELAY" envDefault:"3s"`

	// PollDelay is the wait time between the first and subsequent calls when
	// Telegram returns Pending=true (transcription in progress).
	PollDelay time.Duration `env:"TRANSCRIPTION_POLL_DELAY" envDefault:"30s"`

	// PollAttempts is the maximum number of MessagesTranscribeAudio calls per message.
	// If all attempts return Pending=true, the message is skipped without marking
	// transcribed_at — it will be retried on the next worker run.
	PollAttempts int `env:"TRANSCRIPTION_POLL_ATTEMPTS" envDefault:"2"`
}

// RerankConfig controls the length-sigmoid bonus applied after BM25 scoring.
// Formula: final_score = bm25_score × n_eff / (n_eff + K), n_eff = min(tokens, Cap).
type RerankConfig struct {
	// K is the inflection point: at token_count=K the utterance keeps 50% of its BM25 score.
	// Lower K = more aggressive penalty on short utterances.
	K float64 `env:"RERANK_LENGTH_K"   envDefault:"10"`
	// Cap is the maximum token count used in the formula.
	// Prevents very long utterances from dominating purely through length.
	Cap int `env:"RERANK_LENGTH_CAP" envDefault:"100"`
}

// UtteranceConfig controls how consecutive messages are grouped into semantic utterances.
type UtteranceConfig struct {
	// GapSeconds is the maximum silence between consecutive messages from the same author
	// before a new utterance begins. Messages within this gap are treated as one thought.
	GapSeconds int `env:"UTTERANCE_GAP_SECONDS" envDefault:"120"`
}

// WindowConfig controls participation-window size for group/channel dialogs.
// WindowBefore and WindowAfter define how many messages on each side of an
// outgoing anchor are included in the semantic/personality/episodic pipelines.
type WindowConfig struct {
	Before int `env:"WINDOW_BEFORE" envDefault:"10"`
	After  int `env:"WINDOW_AFTER"  envDefault:"10"`
}

type AppConfig struct {
	Env      string `env:"APP_ENV"       envDefault:"development"`
	Name     string `env:"APP_NAME"      envDefault:"digital-personality"`
	LogLevel string `env:"APP_LOG_LEVEL" envDefault:"info"`
}

type PostgresConfig struct {
	Host            string        `env:"POSTGRES_HOST"              envDefault:"localhost"`
	Port            int           `env:"POSTGRES_PORT"              envDefault:"5432"`
	DB              string        `env:"POSTGRES_DB"                envDefault:"digital_personality"`
	User            string        `env:"POSTGRES_USER"              envDefault:"dp_user"`
	Password        string        `env:"POSTGRES_PASSWORD,required"`
	MaxConns        int32         `env:"POSTGRES_MAX_CONNS"         envDefault:"25"`
	MinConns        int32         `env:"POSTGRES_MIN_CONNS"         envDefault:"5"`
	MaxConnLifetime time.Duration `env:"POSTGRES_MAX_CONN_LIFETIME" envDefault:"1h"`
	MaxConnIdleTime time.Duration `env:"POSTGRES_MAX_CONN_IDLE_TIME" envDefault:"30m"`
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.DB,
	)
}

type TelegramConfig struct {
	AppID         int    `env:"TELEGRAM_APP_ID,required"`
	AppHash       string `env:"TELEGRAM_APP_HASH,required"`
	SessionFile   string `env:"TELEGRAM_SESSION_FILE"   envDefault:"/data/sessions/telegram.session"`
	Phone         string `env:"TELEGRAM_PHONE,required"`
	TwoFAPassword string `env:"TELEGRAM_2FA_PASSWORD"` // optional — leave empty if account has no 2FA
}

type OpenAIConfig struct {
	APIKey         string `env:"OPENAI_API_KEY"` // reserved for Phase 6 LLM persona
	EmbeddingModel string `env:"OPENAI_EMBEDDING_MODEL" envDefault:"text-embedding-3-small"`
	BatchSize      int    `env:"OPENAI_EMBEDDING_BATCH_SIZE" envDefault:"100"`
}

// OllamaConfig configures the local Ollama server.
// Model names must match `ollama list`.
// Set EmbeddingModel to "" to disable vector/embed commands.
// Set ChatModel to "" to disable the persona `ask` command.
type OllamaConfig struct {
	Host           string `env:"OLLAMA_HOST"            envDefault:"http://localhost:11434"`
	EmbeddingModel string `env:"OLLAMA_EMBEDDING_MODEL" envDefault:"bge-m3"`
	EmbedBatchSize int    `env:"OLLAMA_EMBED_BATCH_SIZE" envDefault:"10"`
	ChatModel      string `env:"OLLAMA_CHAT_MODEL"      envDefault:""`
}

// BotConfig configures the Telegram bot delivery layer (Bot API, not MTProto).
// AllowedUserIDs is a comma-separated allowlist of Telegram user IDs;
// empty = the bot replies to everyone (logged as a warning on start).
type BotConfig struct {
	Token          string  `env:"TELEGRAM_BOT_TOKEN"`
	AllowedUserIDs []int64 `env:"TELEGRAM_BOT_ALLOWED_USER_IDS" envSeparator:","`
}

type EmbeddingConfig struct {
	WorkerCount int           `env:"EMBEDDING_WORKER_COUNT" envDefault:"4"`
	QueueSize   int           `env:"EMBEDDING_QUEUE_SIZE"   envDefault:"1000"`
	RetryMax    int           `env:"EMBEDDING_RETRY_MAX"    envDefault:"3"`
	RetryDelay  time.Duration `env:"EMBEDDING_RETRY_DELAY"  envDefault:"5s"`
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// CLIConfig is a minimal config for read-only CLI inspection commands.
// It does not require Telegram credentials.
// Ollama is optional: if EmbeddingModel is empty, vector/embed commands are disabled.
type CLIConfig struct {
	App       AppConfig
	Postgres  PostgresConfig
	Utterance UtteranceConfig
	Rerank    RerankConfig
	Ollama    OllamaConfig
	Bot       BotConfig
}

// LoadCLI parses only the application and database configuration.
// Use this instead of Load() when running CLI commands that do not need Telegram.
func LoadCLI() (*CLIConfig, error) {
	cfg := &CLIConfig{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parse cli config: %w", err)
	}
	return cfg, nil
}
