package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	domrepo "github.com/digital-personality/internal/domain/repository"
	"github.com/digital-personality/internal/domain/entity"
)

type windowRepo struct {
	pool *pgxpool.Pool
}

func NewWindowRepository(pool *pgxpool.Pool) domrepo.WindowRepository {
	return &windowRepo{pool: pool}
}

// ComputeParticipationWindows runs three SQL steps in a single transaction:
//  1. Reset all non-outgoing messages to outside-window.
//  2. Expand windows ±N rows around each outgoing anchor.
//  3. Mark direct reply targets of outgoing messages as in-window.
func (r *windowRepo) ComputeParticipationWindows(
	ctx context.Context,
	chatID int64,
	windowBefore, windowAfter int,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Step 1: Reset non-outgoing messages to outside window.
	// Outgoing messages are always in-window (they are the anchors).
	const reset = `
		UPDATE messages
		SET in_memory_window = FALSE
		WHERE chat_id = $1
		  AND NOT is_outgoing
		  AND NOT is_deleted`
	if _, err := tx.Exec(ctx, reset, chatID); err != nil {
		return fmt.Errorf("reset window flags chat=%d: %w", chatID, err)
	}

	// Step 2: Mark messages within ±windowBefore/After rows of any outgoing anchor.
	// Uses ROW_NUMBER() ordered by sent_at to define "message count" proximity.
	const expand = `
		WITH ordered AS (
			SELECT id,
			       ROW_NUMBER() OVER (ORDER BY sent_at ASC, telegram_id ASC) AS rn
			FROM messages
			WHERE chat_id = $1 AND NOT is_deleted
		),
		anchor_rns AS (
			SELECT o.rn
			FROM ordered o
			JOIN messages m ON m.id = o.id
			WHERE m.is_outgoing = TRUE
		),
		in_window AS (
			SELECT o.id
			FROM ordered o
			WHERE EXISTS (
				SELECT 1 FROM anchor_rns ar
				WHERE o.rn BETWEEN ar.rn - $2 AND ar.rn + $3
			)
		)
		UPDATE messages
		SET in_memory_window = TRUE
		WHERE id IN (SELECT id FROM in_window)`
	if _, err := tx.Exec(ctx, expand, chatID, windowBefore, windowAfter); err != nil {
		return fmt.Errorf("expand anchor windows chat=%d: %w", chatID, err)
	}

	// Step 3: Mark direct reply targets of outgoing messages.
	// reply_to_id stores the telegram_id of the parent message (not the internal id).
	const replyTargets = `
		UPDATE messages
		SET in_memory_window = TRUE
		WHERE chat_id = $1
		  AND NOT is_deleted
		  AND telegram_id IN (
		      SELECT DISTINCT reply_to_id
		      FROM messages
		      WHERE chat_id = $1
		        AND is_outgoing = TRUE
		        AND reply_to_id IS NOT NULL
		        AND reply_to_id != 0
		        AND NOT is_deleted
		  )`
	if _, err := tx.Exec(ctx, replyTargets, chatID); err != nil {
		return fmt.Errorf("mark reply targets chat=%d: %w", chatID, err)
	}

	return tx.Commit(ctx)
}

// ListPendingRebuild returns messages that are in_memory_window = TRUE
// but have no entry in message_semantic — i.e., Layer 2 has not been run yet.
// Ordered chronologically so the expander processes them in conversation order.
func (r *windowRepo) ListPendingRebuild(ctx context.Context, chatID int64, limit int) ([]*entity.Message, error) {
	const q = `
		SELECT m.id, m.telegram_id, m.chat_id,
		       COALESCE(m.sender_id, 0), COALESCE(m.reply_to_id, 0),
		       m.text, m.raw_data, m.entities, m.reactions, m.sticker_meta,
		       m.media_kind, m.sent_at, m.synced_at, m.is_outgoing, m.is_deleted,
		       m.is_forwarded, COALESCE(m.forward_from_id, 0), m.forward_date, m.edit_date
		FROM messages m
		LEFT JOIN message_semantic ms ON ms.message_id = m.id
		WHERE m.chat_id = $1
		  AND m.in_memory_window = TRUE
		  AND NOT m.is_deleted
		  AND ms.message_id IS NULL
		ORDER BY m.sent_at ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending rebuild chat=%d: %w", chatID, err)
	}
	defer rows.Close()

	var msgs []*entity.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending rebuild row: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}
