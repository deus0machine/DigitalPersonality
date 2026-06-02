// Package transcription contains the voice transcription backfill use case.
// It fetches in-window voice messages that have not yet been transcribed,
// calls Telegram Premium STT via VoiceTranscriber, and writes results to
// message_semantic so they enter the embedding pipeline automatically.
package transcription

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/config"
	"github.com/digital-personality/internal/domain/entity"
	"github.com/digital-personality/internal/domain/repository"
)

// Worker transcribes in-window voice messages via Telegram Premium STT.
// Safe to interrupt and re-run: transcribed_at IS NULL is the idempotent guard.
type Worker struct {
	transcriber  port.VoiceTranscriber
	semanticRepo repository.SemanticRepository
	pool         *pgxpool.Pool
	cfg          config.TranscriptionConfig
	log          *slog.Logger
}

// New constructs a Worker. pool is used for the queue query (joins messages + chats).
func New(
	transcriber  port.VoiceTranscriber,
	semanticRepo repository.SemanticRepository,
	pool         *pgxpool.Pool,
	cfg          config.TranscriptionConfig,
	log          *slog.Logger,
) *Worker {
	return &Worker{
		transcriber:  transcriber,
		semanticRepo: semanticRepo,
		pool:         pool,
		cfg:          cfg,
		log:          log,
	}
}

// voiceCandidate holds the data needed for one TranscribeVoice call.
type voiceCandidate struct {
	messageID     int64
	telegramID    int64
	chatID        int64
	chatType      entity.ChatType
	accessHash    int64
}

// Run processes all pending voice messages in batches until the queue is empty.
// Returns a non-nil error only for systemic failures (e.g. ErrPremiumRequired).
func (w *Worker) Run(ctx context.Context) error {
	var (
		processed int
		succeeded int
		permanent int
		pending   int
		transient int
	)

	start := time.Now()
	w.log.Info("transcription worker started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		batch, err := w.fetchBatch(ctx)
		if err != nil {
			return fmt.Errorf("fetch transcription batch: %w", err)
		}
		if len(batch) == 0 {
			break
		}

		for _, c := range batch {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			transcript, err := w.transcriber.TranscribeVoice(
				ctx, c.chatType, c.chatID, c.accessHash, int(c.telegramID),
			)

			switch {
			case err == nil:
				tc := tokenCount(transcript)
				if mErr := w.semanticRepo.MarkTranscribed(ctx, c.messageID, transcript, tc); mErr != nil {
					w.log.Warn("mark transcribed failed",
						"message_id", c.messageID, "error", mErr)
				} else {
					succeeded++
					w.log.Info("voice transcribed",
						"message_id", c.messageID,
						"chat_id", c.chatID,
						"tokens", tc,
					)
				}

			case errors.Is(err, port.ErrTranscriptionPending):
				pending++
				w.log.Warn("transcription still pending after polls, will retry in next run",
					"message_id", c.messageID,
					"chat_id", c.chatID,
					"telegram_id", c.telegramID,
				)
				// transcribed_at stays NULL — message reappears in next run

			case errors.Is(err, port.ErrPremiumRequired):
				w.log.Error("Telegram Premium required, aborting transcription run", "error", err)
				w.logSummary(processed, succeeded, permanent, pending, transient, start)
				return fmt.Errorf("transcription aborted: %w", err)

			case errors.Is(err, port.ErrTranscriptionPermanent):
				permanent++
				w.log.Warn("permanent transcription failure, marking as done",
					"message_id", c.messageID,
					"chat_id", c.chatID,
					"error", err,
				)
				if mErr := w.semanticRepo.MarkTranscribed(ctx, c.messageID, "", 0); mErr != nil {
					w.log.Warn("mark permanent failure failed", "message_id", c.messageID, "error", mErr)
				}

			default:
				transient++
				w.log.Warn("transient transcription error, will retry in next run",
					"message_id", c.messageID,
					"chat_id", c.chatID,
					"error", err,
				)
				// transcribed_at stays NULL
			}

			processed++
			time.Sleep(w.cfg.RequestDelay)
		}
	}

	w.logSummary(processed, succeeded, permanent, pending, transient, start)
	return nil
}

// fetchBatch retrieves the next batch of voice messages awaiting transcription.
// Only in-window, non-deleted voice messages with no transcribed_at are included.
func (w *Worker) fetchBatch(ctx context.Context) ([]voiceCandidate, error) {
	const q = `
		SELECT m.id, m.telegram_id, m.chat_id, c.type, c.access_hash
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		JOIN message_semantic ms ON ms.message_id = m.id
		WHERE m.media_kind = 'voice'
		  AND m.in_memory_window = TRUE
		  AND NOT m.is_deleted
		  AND ms.transcribed_at IS NULL
		ORDER BY m.sent_at ASC
		LIMIT $1`

	rows, err := w.pool.Query(ctx, q, w.cfg.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("voice transcription queue: %w", err)
	}
	defer rows.Close()

	var batch []voiceCandidate
	for rows.Next() {
		var c voiceCandidate
		var chatType string
		if err := rows.Scan(&c.messageID, &c.telegramID, &c.chatID, &chatType, &c.accessHash); err != nil {
			return nil, fmt.Errorf("scan voice candidate: %w", err)
		}
		c.chatType = entity.ChatType(chatType)
		batch = append(batch, c)
	}
	return batch, rows.Err()
}

func (w *Worker) logSummary(processed, succeeded, permanent, pending, transient int, start time.Time) {
	w.log.Info("transcription worker finished",
		"processed", processed,
		"succeeded", succeeded,
		"permanent_failures", permanent,
		"pending_skipped", pending,
		"transient_skipped", transient,
		"duration", time.Since(start).Round(time.Second),
	)
}

// tokenCount splits on whitespace — consistent with normalizer.countTokens.
func tokenCount(s string) int {
	return len(strings.Fields(s))
}
