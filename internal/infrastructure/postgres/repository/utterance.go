package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/application/utterance"
)

type utteranceRepo struct {
	pool *pgxpool.Pool
}

// NewUtteranceRepository returns an utterance.Repository backed by PostgreSQL.
func NewUtteranceRepository(pool *pgxpool.Pool) utterance.Repository {
	return &utteranceRepo{pool: pool}
}

const utteranceSelectCols = `
	m.id,
	m.chat_id,
	COALESCE(c.title, ''),
	COALESCE(m.sender_id, 0),
	m.sent_at,
	COALESCE(ms.normalized_text, ''),
	COALESCE(ms.token_count, 0),
	m.is_outgoing,
	COALESCE(m.media_kind, '')`

const utteranceJoins = `
	LEFT JOIN message_semantic ms ON ms.message_id = m.id
	LEFT JOIN chats c             ON c.id = m.chat_id`

const utteranceOrder = `ORDER BY m.sent_at ASC, m.id ASC`

// FetchInWindowMessages returns all in-window, non-deleted messages for a single chat.
func (r *utteranceRepo) FetchInWindowMessages(ctx context.Context, chatID int64) ([]utterance.MessageInput, error) {
	q := `SELECT` + utteranceSelectCols + `
		FROM messages m` + utteranceJoins + `
		WHERE m.chat_id          = $1
		  AND m.in_memory_window = TRUE
		  AND NOT m.is_deleted
		` + utteranceOrder

	rows, err := r.pool.Query(ctx, q, chatID)
	if err != nil {
		return nil, fmt.Errorf("fetch in-window messages chat=%d: %w", chatID, err)
	}
	defer rows.Close()
	return scanMessageInputs(rows)
}

// FetchAllInWindowMessages returns in-window, non-deleted messages across all chats.
func (r *utteranceRepo) FetchAllInWindowMessages(ctx context.Context) ([]utterance.MessageInput, error) {
	q := `SELECT` + utteranceSelectCols + `
		FROM messages m` + utteranceJoins + `
		WHERE m.in_memory_window = TRUE
		  AND NOT m.is_deleted
		` + utteranceOrder

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("fetch all in-window messages: %w", err)
	}
	defer rows.Close()
	return scanMessageInputs(rows)
}

type scannable interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

func scanMessageInputs(rows scannable) ([]utterance.MessageInput, error) {
	defer rows.Close()
	var msgs []utterance.MessageInput
	for rows.Next() {
		var m utterance.MessageInput
		if err := rows.Scan(
			&m.ID, &m.ChatID, &m.ChatTitle, &m.AuthorID, &m.SentAt,
			&m.NormalizedText, &m.TokenCount, &m.IsOutgoing, &m.MediaKind,
		); err != nil {
			return nil, fmt.Errorf("scan message input: %w", err)
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
