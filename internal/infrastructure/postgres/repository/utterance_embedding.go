package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/digital-personality/internal/application/utterance"
)

type utteranceEmbeddingRepo struct {
	pool *pgxpool.Pool
}

// NewUtteranceEmbeddingRepository returns an utterance.UtteranceEmbeddingRepository backed by PostgreSQL.
func NewUtteranceEmbeddingRepository(pool *pgxpool.Pool) utterance.UtteranceEmbeddingRepository {
	return &utteranceEmbeddingRepo{pool: pool}
}

// FilterUnembedded returns the subset of ids not yet in utterance_embeddings.
func (r *utteranceEmbeddingRepo) FilterUnembedded(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	const q = `
		SELECT first_message_id
		FROM utterance_embeddings
		WHERE first_message_id = ANY($1)`

	rows, err := r.pool.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("filter unembedded: %w", err)
	}
	defer rows.Close()

	embedded := make(map[int64]struct{}, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan embedded id: %w", err)
		}
		embedded[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("filter unembedded rows: %w", err)
	}

	pending := make([]int64, 0, len(ids)-len(embedded))
	for _, id := range ids {
		if _, ok := embedded[id]; !ok {
			pending = append(pending, id)
		}
	}
	return pending, nil
}

// SaveBatch inserts a batch of embeddings inside a single transaction.
// Idempotent: ON CONFLICT (first_message_id) DO NOTHING.
func (r *utteranceEmbeddingRepo) SaveBatch(
	ctx context.Context,
	batch []utterance.EmbeddingCandidate,
	vectors [][]float32,
	modelName string,
) error {
	if len(batch) == 0 {
		return nil
	}

	const q = `
		INSERT INTO utterance_embeddings (first_message_id, model_name, gap_seconds, vector)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (first_message_id) DO NOTHING`

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, c := range batch {
		if i >= len(vectors) || vectors[i] == nil {
			continue
		}
		if _, err := tx.Exec(ctx, q,
			c.FirstMessageID,
			modelName,
			c.GapSeconds,
			pgvector.NewVector(vectors[i]),
		); err != nil {
			return fmt.Errorf("insert utterance embedding id=%d: %w", c.FirstMessageID, err)
		}
	}

	return tx.Commit(ctx)
}

// SearchByVector returns the top-limit utterances nearest to vector by cosine distance.
func (r *utteranceEmbeddingRepo) SearchByVector(ctx context.Context, vector []float32, limit int) ([]utterance.VectorHit, error) {
	const q = `
		SELECT first_message_id, vector <=> $1 AS distance
		FROM utterance_embeddings
		ORDER BY vector <=> $1
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, pgvector.NewVector(vector), limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var hits []utterance.VectorHit
	for rows.Next() {
		var h utterance.VectorHit
		if err := rows.Scan(&h.FirstMessageID, &h.Distance); err != nil {
			return nil, fmt.Errorf("scan vector hit: %w", err)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// StoredGapSeconds returns the gap_seconds from any existing row, or 0 if the table is empty.
func (r *utteranceEmbeddingRepo) StoredGapSeconds(ctx context.Context) (int, error) {
	const q = `SELECT gap_seconds FROM utterance_embeddings LIMIT 1`
	var gap int
	err := r.pool.QueryRow(ctx, q).Scan(&gap)
	if err == pgx.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stored gap seconds: %w", err)
	}
	return gap, nil
}
