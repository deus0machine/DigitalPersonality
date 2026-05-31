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

type semanticRepo struct {
	pool *pgxpool.Pool
}

func NewSemanticRepository(pool *pgxpool.Pool) domrepo.SemanticRepository {
	return &semanticRepo{pool: pool}
}

func (r *semanticRepo) Upsert(ctx context.Context, doc *entity.SemanticDocument) error {
	const q = `
		INSERT INTO message_semantic
			(message_id, normalized_text, language, token_count, skip_embedding, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (message_id) DO UPDATE SET
			normalized_text = EXCLUDED.normalized_text,
			language        = EXCLUDED.language,
			token_count     = EXCLUDED.token_count,
			skip_embedding  = EXCLUDED.skip_embedding`

	_, err := r.pool.Exec(ctx, q,
		doc.MessageID, doc.NormalizedText, nullString(doc.Language),
		doc.TokenCount, doc.SkipEmbedding,
	)
	if err != nil {
		return fmt.Errorf("upsert semantic doc msg=%d: %w", doc.MessageID, err)
	}
	return nil
}

func (r *semanticRepo) GetByMessageID(ctx context.Context, messageID int64) (*entity.SemanticDocument, error) {
	const q = `
		SELECT message_id, normalized_text, COALESCE(language,''),
		       token_count, skip_embedding, created_at
		FROM message_semantic WHERE message_id = $1`

	doc := &entity.SemanticDocument{}
	err := r.pool.QueryRow(ctx, q, messageID).Scan(
		&doc.MessageID, &doc.NormalizedText, &doc.Language,
		&doc.TokenCount, &doc.SkipEmbedding, &doc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("semantic doc msg=%d: %w", messageID, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get semantic doc msg=%d: %w", messageID, err)
	}
	return doc, nil
}

// ListPendingEmbedding returns message IDs with a semantic document that:
// - has skip_embedding = FALSE
// - is in the memory window (in_memory_window = TRUE)
// - has no matching entry in the embeddings table for the given model
// The in_memory_window filter prevents embedding orphan docs for messages that
// were windowed out after window computation (social/passive_consumption surfaces).
func (r *semanticRepo) ListPendingEmbedding(ctx context.Context, model string, limit int) ([]int64, error) {
	const q = `
		SELECT ms.message_id
		FROM message_semantic ms
		JOIN messages m ON m.id = ms.message_id
		WHERE ms.skip_embedding = FALSE
		  AND m.in_memory_window = TRUE
		  AND NOT m.is_deleted
		  AND NOT EXISTS (
		      SELECT 1 FROM embeddings e
		      WHERE e.message_id = ms.message_id AND e.model = $1
		  )
		ORDER BY ms.message_id ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, model, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending embedding: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pending id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
