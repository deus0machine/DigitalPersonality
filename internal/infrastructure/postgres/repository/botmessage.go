package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/domain/entity"
	domrepo "github.com/digital-personality/internal/domain/repository"
)

type botMessageRepo struct {
	pool *pgxpool.Pool
}

// NewBotMessageRepository creates the bot conversation log repository.
func NewBotMessageRepository(pool *pgxpool.Pool) domrepo.BotMessageRepository {
	return &botMessageRepo{pool: pool}
}

func (r *botMessageRepo) Save(ctx context.Context, msg *entity.BotMessage) error {
	const q = `
		INSERT INTO bot_messages (chat_id, user_id, username, from_persona, text)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.pool.Exec(ctx, q, msg.ChatID, msg.UserID, msg.Username, msg.FromPersona, msg.Text)
	if err != nil {
		return fmt.Errorf("save bot message chat=%d: %w", msg.ChatID, err)
	}
	return nil
}

func (r *botMessageRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]entity.BotMessage, error) {
	const q = `
		SELECT id, chat_id, user_id, username, from_persona, text, created_at
		FROM bot_messages
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list bot messages user=%d: %w", userID, err)
	}
	defer rows.Close()

	var msgs []entity.BotMessage
	for rows.Next() {
		var m entity.BotMessage
		if err := rows.Scan(&m.ID, &m.ChatID, &m.UserID, &m.Username, &m.FromPersona, &m.Text, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan bot message: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to chronological order (query fetched the newest window).
	for lo, hi := 0, len(msgs)-1; lo < hi; lo, hi = lo+1, hi-1 {
		msgs[lo], msgs[hi] = msgs[hi], msgs[lo]
	}
	return msgs, nil
}

func (r *botMessageRepo) ListDialogs(ctx context.Context) ([]domrepo.BotDialogSummary, error) {
	const q = `
		SELECT user_id,
			MAX(username) FILTER (WHERE username <> '') AS username,
			COUNT(*) AS messages,
			MAX(created_at) AS last_at
		FROM bot_messages
		GROUP BY user_id
		ORDER BY last_at DESC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list bot dialogs: %w", err)
	}
	defer rows.Close()

	var dialogs []domrepo.BotDialogSummary
	for rows.Next() {
		var d domrepo.BotDialogSummary
		var username *string
		if err := rows.Scan(&d.UserID, &username, &d.Messages, &d.LastAt); err != nil {
			return nil, fmt.Errorf("scan bot dialog: %w", err)
		}
		if username != nil {
			d.Username = *username
		}
		dialogs = append(dialogs, d)
	}
	return dialogs, rows.Err()
}
