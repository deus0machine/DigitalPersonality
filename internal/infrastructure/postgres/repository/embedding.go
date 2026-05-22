package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/digital-personality/internal/domain/entity"
	domrepo "github.com/digital-personality/internal/domain/repository"
)

type embeddingRepo struct {
	pool *pgxpool.Pool
}

func NewEmbeddingRepository(pool *pgxpool.Pool) domrepo.EmbeddingRepository {
	return &embeddingRepo{pool: pool}
}

func (r *embeddingRepo) Save(ctx context.Context, emb *entity.Embedding) error {
	const q = `
		INSERT INTO embeddings (message_id, model, vector, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (message_id, model) DO UPDATE SET
			vector     = EXCLUDED.vector,
			created_at = NOW()`

	_, err := r.pool.Exec(ctx, q, emb.MessageID, emb.Model, pgvector.NewVector(emb.Vector))
	if err != nil {
		return fmt.Errorf("save embedding msg=%d model=%s: %w", emb.MessageID, emb.Model, err)
	}
	return nil
}

func (r *embeddingRepo) GetByMessageID(ctx context.Context, messageID int64) ([]*entity.Embedding, error) {
	const q = `
		SELECT id, message_id, model, vector, created_at
		FROM embeddings WHERE message_id = $1`

	rows, err := r.pool.Query(ctx, q, messageID)
	if err != nil {
		return nil, fmt.Errorf("get embeddings msg=%d: %w", messageID, err)
	}
	defer rows.Close()

	var result []*entity.Embedding
	for rows.Next() {
		emb := &entity.Embedding{}
		var vec pgvector.Vector
		if err := rows.Scan(&emb.ID, &emb.MessageID, &emb.Model, &vec, &emb.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan embedding: %w", err)
		}
		emb.Vector = vec.Slice()
		result = append(result, emb)
	}
	return result, rows.Err()
}

func (r *embeddingRepo) SearchSimilar(ctx context.Context, vector []float32, model string, topK int) ([]*entity.SearchResult, error) {
	const q = `
		SELECT m.id, m.telegram_id, m.chat_id,
		       COALESCE(m.sender_id,0), COALESCE(m.reply_to_id,0),
		       m.text, m.raw_data, m.sent_at, m.synced_at, m.is_outgoing, m.is_deleted,
		       1 - (e.vector <=> $1) AS similarity
		FROM embeddings e
		JOIN messages m ON m.id = e.message_id
		WHERE e.model = $2 AND m.is_deleted = FALSE
		ORDER BY e.vector <=> $1
		LIMIT $3`

	rows, err := r.pool.Query(ctx, q, pgvector.NewVector(vector), model, topK)
	if err != nil {
		return nil, fmt.Errorf("search similar: %w", err)
	}
	defer rows.Close()

	var results []*entity.SearchResult
	for rows.Next() {
		sr := &entity.SearchResult{}
		if err := rows.Scan(
			&sr.Message.ID, &sr.Message.TelegramID, &sr.Message.ChatID,
			&sr.Message.SenderID, &sr.Message.ReplyToID,
			&sr.Message.Text, &sr.Message.RawData, &sr.Message.SentAt,
			&sr.Message.SyncedAt, &sr.Message.IsOutgoing, &sr.Message.IsDeleted,
			&sr.Similarity,
		); err != nil {
			return nil, fmt.Errorf("scan search result: %w", err)
		}
		results = append(results, sr)
	}
	return results, rows.Err()
}

func (r *embeddingRepo) ListUnembedded(ctx context.Context, model string, limit int) ([]int64, error) {
	const q = `
		SELECT m.id FROM messages m
		WHERE m.is_deleted = FALSE
		  AND m.text != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM embeddings e
		    WHERE e.message_id = m.id AND e.model = $1
		  )
		ORDER BY m.sent_at DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, model, limit)
	if err != nil {
		return nil, fmt.Errorf("list unembedded: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
