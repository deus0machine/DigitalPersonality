package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/domain/entity"
	domrepo "github.com/digital-personality/internal/domain/repository"
)

type episodeRepo struct {
	pool *pgxpool.Pool
}

func NewEpisodeRepository(pool *pgxpool.Pool) domrepo.EpisodeRepository {
	return &episodeRepo{pool: pool}
}

func (r *episodeRepo) Create(ctx context.Context, ep *entity.Episode) (int64, error) {
	const q = `
		INSERT INTO episodes
			(chat_id, started_at, ended_at, episode_type, message_count,
			 participant_ids, segmented_by, confidence, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING id`

	var id int64
	err := r.pool.QueryRow(ctx, q,
		ep.ChatID,
		ep.StartedAt,
		ep.EndedAt,
		string(ep.Type),
		ep.MessageCount,
		ep.ParticipantIDs,
		string(ep.SegmentedBy),
		ep.Confidence,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create episode chat=%d: %w", ep.ChatID, err)
	}
	return id, nil
}

func (r *episodeRepo) LinkMessages(ctx context.Context, links []entity.EpisodeMessage) error {
	if len(links) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	const q = `
		INSERT INTO episode_messages (episode_id, message_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (message_id) DO NOTHING`

	for _, l := range links {
		batch.Queue(q, l.EpisodeID, l.MessageID, l.Position)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := range links {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("link message %d to episode %d: %w", links[i].MessageID, links[i].EpisodeID, err)
		}
	}
	return br.Close()
}

func (r *episodeRepo) GetByID(ctx context.Context, id int64) (*entity.Episode, error) {
	const q = `
		SELECT id, chat_id, started_at, ended_at, episode_type,
		       message_count, participant_ids, segmented_by, confidence,
		       importance, COALESCE(emotional_valence,''),
		       COALESCE(summary,''), COALESCE(summary_model,''),
		       created_at, updated_at
		FROM episodes WHERE id = $1`

	ep, err := scanEpisode(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("episode id=%d: %w", id, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get episode %d: %w", id, err)
	}
	return ep, nil
}

func (r *episodeRepo) ListByChat(ctx context.Context, chatID int64, limit, offset int) ([]*entity.Episode, error) {
	const q = `
		SELECT id, chat_id, started_at, ended_at, episode_type,
		       message_count, participant_ids, segmented_by, confidence,
		       importance, COALESCE(emotional_valence,''),
		       COALESCE(summary,''), COALESCE(summary_model,''),
		       created_at, updated_at
		FROM episodes
		WHERE chat_id = $1
		ORDER BY started_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, q, chatID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list episodes chat=%d: %w", chatID, err)
	}
	defer rows.Close()

	var eps []*entity.Episode
	for rows.Next() {
		ep, err := scanEpisode(rows)
		if err != nil {
			return nil, fmt.Errorf("scan episode row: %w", err)
		}
		eps = append(eps, ep)
	}
	return eps, rows.Err()
}

// ListUnepisodedMessages returns messages in chatID that have not yet been
// assigned to any episode, ordered by sent_at ascending.
// Anti-join on episode_messages ensures idempotency: re-running never re-processes.
func (r *episodeRepo) ListUnepisodedMessages(ctx context.Context, chatID int64, limit int) ([]*entity.Message, error) {
	const q = `
		SELECT m.id, m.telegram_id, m.chat_id,
		       COALESCE(m.sender_id, 0), COALESCE(m.reply_to_id, 0),
		       m.text, m.raw_data, m.entities, m.reactions, m.sticker_meta,
		       m.media_kind, m.sent_at, m.synced_at, m.is_outgoing, m.is_deleted,
		       m.is_forwarded, COALESCE(m.forward_from_id, 0), m.forward_date, m.edit_date
		FROM messages m
		WHERE m.chat_id = $1
		  AND m.is_deleted = FALSE
		  AND m.in_memory_window = TRUE
		  AND NOT EXISTS (
		      SELECT 1 FROM episode_messages em WHERE em.message_id = m.id
		  )
		ORDER BY m.sent_at ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("list unepisoded messages chat=%d: %w", chatID, err)
	}
	defer rows.Close()

	var msgs []*entity.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

func (r *episodeRepo) UpsertSemantic(ctx context.Context, doc *entity.EpisodeSemanticDoc) error {
	const q = `
		INSERT INTO episode_semantic
			(episode_id, semantic_text, token_count, skip_embedding, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (episode_id) DO UPDATE SET
			semantic_text  = EXCLUDED.semantic_text,
			token_count    = EXCLUDED.token_count,
			skip_embedding = EXCLUDED.skip_embedding`

	_, err := r.pool.Exec(ctx, q,
		doc.EpisodeID,
		doc.SemanticText,
		doc.TokenCount,
		doc.SkipEmbedding,
		doc.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert episode semantic ep=%d: %w", doc.EpisodeID, err)
	}
	return nil
}

func (r *episodeRepo) GetSemantic(ctx context.Context, episodeID int64) (*entity.EpisodeSemanticDoc, error) {
	const q = `
		SELECT episode_id, semantic_text, token_count, skip_embedding, created_at
		FROM episode_semantic WHERE episode_id = $1`

	doc := &entity.EpisodeSemanticDoc{}
	err := r.pool.QueryRow(ctx, q, episodeID).Scan(
		&doc.EpisodeID,
		&doc.SemanticText,
		&doc.TokenCount,
		&doc.SkipEmbedding,
		&doc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("episode semantic ep=%d: %w", episodeID, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get episode semantic ep=%d: %w", episodeID, err)
	}
	return doc, nil
}

// ListPendingEmbedding returns episode IDs whose semantic doc has
// skip_embedding = FALSE and no matching entry in episode_embeddings for the given model.
func (r *episodeRepo) ListPendingEmbedding(ctx context.Context, model string, limit int) ([]int64, error) {
	const q = `
		SELECT es.episode_id
		FROM episode_semantic es
		WHERE es.skip_embedding = FALSE
		  AND NOT EXISTS (
		      SELECT 1 FROM episode_embeddings ee
		      WHERE ee.episode_id = es.episode_id AND ee.model = $1
		  )
		ORDER BY es.episode_id ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, model, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending episode embeddings: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pending episode id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func scanEpisode(row pgx.Row) (*entity.Episode, error) {
	ep := &entity.Episode{}
	var (
		epType           string
		segmentedBy      string
		importance       *float32
		emotionalValence string
		summary          string
		summaryModel     string
		createdAt        time.Time
		updatedAt        time.Time
	)

	err := row.Scan(
		&ep.ID,
		&ep.ChatID,
		&ep.StartedAt,
		&ep.EndedAt,
		&epType,
		&ep.MessageCount,
		&ep.ParticipantIDs,
		&segmentedBy,
		&ep.Confidence,
		&importance,
		&emotionalValence,
		&summary,
		&summaryModel,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	ep.Type = entity.EpisodeType(epType)
	ep.SegmentedBy = entity.SegmentMethod(segmentedBy)
	ep.Importance = importance
	ep.EmotionalValence = emotionalValence
	ep.Summary = summary
	ep.SummaryModel = summaryModel
	ep.CreatedAt = createdAt
	ep.UpdatedAt = updatedAt

	return ep, nil
}
