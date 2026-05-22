package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/domain/entity"
	domrepo "github.com/digital-personality/internal/domain/repository"
)

type chatRepo struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) domrepo.ChatRepository {
	return &chatRepo{pool: pool}
}

func (r *chatRepo) Upsert(ctx context.Context, c *entity.Chat) error {
	const q = `
		INSERT INTO chats (id, type, title, username, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (id) DO UPDATE SET
			type       = EXCLUDED.type,
			title      = EXCLUDED.title,
			username   = EXCLUDED.username,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, q,
		c.ID, string(c.Type), nullString(c.Title), nullString(c.Username),
	)
	if err != nil {
		return fmt.Errorf("upsert chat %d: %w", c.ID, err)
	}
	return nil
}

func (r *chatRepo) UpdateRelevance(ctx context.Context, chatID int64, score float32, reason string, surface entity.PersonalitySurface) error {
	const q = `
		UPDATE chats
		SET relevance_score     = $2,
		    relevance_reason    = $3,
		    personality_surface = $4,
		    updated_at          = NOW()
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, q, chatID, score, reason, string(surface))
	if err != nil {
		return fmt.Errorf("update relevance chat=%d: %w", chatID, err)
	}
	return nil
}

func (r *chatRepo) GetByID(ctx context.Context, id int64) (*entity.Chat, error) {
	const q = `
		SELECT id, type, COALESCE(title,''), COALESCE(username,''), created_at, updated_at
		FROM chats WHERE id = $1`

	c := &entity.Chat{}
	var chatType string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &chatType, &c.Title, &c.Username, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("chat %d: %w", id, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get chat %d: %w", id, err)
	}
	c.Type = entity.ChatType(chatType)
	return c, nil
}

func (r *chatRepo) ListAll(ctx context.Context) ([]*entity.Chat, error) {
	const q = `
		SELECT id, type, COALESCE(title,''), COALESCE(username,''), created_at, updated_at
		FROM chats ORDER BY updated_at DESC`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()

	var chats []*entity.Chat
	for rows.Next() {
		c := &entity.Chat{}
		var chatType string
		if err := rows.Scan(&c.ID, &chatType, &c.Title, &c.Username, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		c.Type = entity.ChatType(chatType)
		chats = append(chats, c)
	}
	return chats, rows.Err()
}
