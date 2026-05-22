package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/application/retrieval"
	"github.com/digital-personality/internal/domain/entity"
)

type retrievalRepo struct {
	pool *pgxpool.Pool
}

func NewRetrievalRepository(pool *pgxpool.Pool) retrieval.Repository {
	return &retrievalRepo{pool: pool}
}

// ─── SearchMessages ───────────────────────────────────────────────────────────

// SearchMessages tries FTS first. If it returns nothing, falls back to trigram.
func (r *retrievalRepo) SearchMessages(ctx context.Context, q retrieval.Query) ([]retrieval.MessageHit, error) {
	if q.Text != "" {
		hits, err := r.ftsMessages(ctx, q)
		if err != nil {
			return nil, err
		}
		if len(hits) > 0 {
			return hits, nil
		}
		// FTS found nothing — try trigram similarity.
		return r.trigramMessages(ctx, q)
	}
	// No text: return chronologically filtered results.
	return r.filterMessages(ctx, q)
}

// ftsMessages uses websearch_to_tsquery + ts_rank_cd for ranked full-text search.
func (r *retrievalRepo) ftsMessages(ctx context.Context, q retrieval.Query) ([]retrieval.MessageHit, error) {
	args := []any{q.Text}
	n := 2

	where, n, args := messageWhereClause(q, n, args)

	query := fmt.Sprintf(`
		SELECT
			m.id, m.chat_id, c.title, c.personality_surface,
			m.text, m.sent_at, m.is_outgoing, m.is_forwarded, m.media_kind,
			ts_rank_cd(m.text_search, websearch_to_tsquery('simple', $1), 4) AS rank,
			'fts' AS match_type,
			COALESCE(em.episode_id, 0) AS episode_id
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE m.is_deleted = FALSE
		  AND m.text_search @@ websearch_to_tsquery('simple', $1)
		  AND length(m.text) > 0
		  %s
		ORDER BY rank DESC, m.sent_at DESC
		LIMIT %d`, where, q.Limit)

	return r.scanMessageHits(ctx, query, args)
}

// trigramMessages uses pg_trgm similarity() for fuzzy matching.
func (r *retrievalRepo) trigramMessages(ctx context.Context, q retrieval.Query) ([]retrieval.MessageHit, error) {
	args := []any{q.Text, q.SimilarityThreshold}
	n := 3

	where, n, args := messageWhereClause(q, n, args)
	_ = n

	query := fmt.Sprintf(`
		SELECT
			m.id, m.chat_id, c.title, c.personality_surface,
			m.text, m.sent_at, m.is_outgoing, m.is_forwarded, m.media_kind,
			similarity(m.text, $1) AS rank,
			'trigram' AS match_type,
			COALESCE(em.episode_id, 0) AS episode_id
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE m.is_deleted = FALSE
		  AND length(m.text) > 0
		  AND similarity(m.text, $1) >= $2
		  %s
		ORDER BY rank DESC, m.sent_at DESC
		LIMIT %d`, where, q.Limit)

	return r.scanMessageHits(ctx, query, args)
}

// filterMessages returns messages matching metadata filters only (no text search).
func (r *retrievalRepo) filterMessages(ctx context.Context, q retrieval.Query) ([]retrieval.MessageHit, error) {
	args := []any{}
	n := 1
	where, n, args := messageWhereClause(q, n, args)
	_ = n

	query := fmt.Sprintf(`
		SELECT
			m.id, m.chat_id, c.title, c.personality_surface,
			m.text, m.sent_at, m.is_outgoing, m.is_forwarded, m.media_kind,
			0::real AS rank,
			'filter' AS match_type,
			COALESCE(em.episode_id, 0) AS episode_id
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE m.is_deleted = FALSE
		  %s
		ORDER BY m.sent_at DESC
		LIMIT %d`, where, q.Limit)

	return r.scanMessageHits(ctx, query, args)
}

// ─── SearchEpisodes ───────────────────────────────────────────────────────────

func (r *retrievalRepo) SearchEpisodes(ctx context.Context, q retrieval.Query) ([]retrieval.EpisodeHit, error) {
	args := []any{q.Text}
	n := 2

	var clauses []string
	if q.ChatID != 0 {
		clauses = append(clauses, fmt.Sprintf("AND e.chat_id = $%d", n))
		args = append(args, q.ChatID)
		n++
	}
	if q.Surface != "" {
		clauses = append(clauses, fmt.Sprintf("AND c.personality_surface = $%d", n))
		args = append(args, string(q.Surface))
		n++
	}
	if !q.Since.IsZero() {
		clauses = append(clauses, fmt.Sprintf("AND e.started_at >= $%d", n))
		args = append(args, q.Since)
		n++
	}
	if !q.Until.IsZero() {
		clauses = append(clauses, fmt.Sprintf("AND e.ended_at <= $%d", n))
		args = append(args, q.Until)
		n++
	}
	_ = n
	extra := strings.Join(clauses, " ")

	query := fmt.Sprintf(`
		SELECT
			e.id, e.chat_id, c.title, c.personality_surface,
			e.type, es.semantic_text,
			e.message_count, e.started_at, e.ended_at,
			ts_rank_cd(es.text_search, websearch_to_tsquery('simple', $1), 4) AS rank
		FROM episodes e
		JOIN episode_semantic es ON es.episode_id = e.id
		JOIN chats c ON c.id = e.chat_id
		WHERE es.text_search @@ websearch_to_tsquery('simple', $1)
		  %s
		ORDER BY rank DESC, e.started_at DESC
		LIMIT %d`, extra, q.Limit)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	defer rows.Close()

	var hits []retrieval.EpisodeHit
	for rows.Next() {
		var h retrieval.EpisodeHit
		var surface string
		var epType string
		if err := rows.Scan(
			&h.EpisodeID, &h.ChatID, &h.ChatTitle, &surface,
			&epType, &h.SemanticText,
			&h.MessageCount, &h.StartedAt, &h.EndedAt,
			&h.Rank,
		); err != nil {
			return nil, fmt.Errorf("scan episode hit: %w", err)
		}
		h.Surface = entity.PersonalitySurface(surface)
		h.Type = entity.EpisodeType(epType)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// ─── FindSimilar ──────────────────────────────────────────────────────────────

func (r *retrievalRepo) FindSimilar(ctx context.Context, sample string, q retrieval.Query) ([]retrieval.MessageHit, error) {
	args := []any{sample, q.SimilarityThreshold}
	n := 3
	where, n, args := messageWhereClause(q, n, args)
	_ = n

	query := fmt.Sprintf(`
		SELECT
			m.id, m.chat_id, c.title, c.personality_surface,
			m.text, m.sent_at, m.is_outgoing, m.is_forwarded, m.media_kind,
			similarity(m.text, $1) AS rank,
			'trigram' AS match_type,
			COALESCE(em.episode_id, 0) AS episode_id
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE m.is_deleted = FALSE
		  AND length(m.text) > 0
		  AND similarity(m.text, $1) >= $2
		  AND m.text != $1
		  %s
		ORDER BY rank DESC
		LIMIT %d`, where, q.Limit)

	return r.scanMessageHits(ctx, query, args)
}

// ─── PersonalityReport ────────────────────────────────────────────────────────

func (r *retrievalRepo) PersonalityReport(ctx context.Context, chatID int64) (*retrieval.PersonalityReport, error) {
	reports, err := r.fetchReports(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, fmt.Errorf("chat %d not found", chatID)
	}
	return &reports[0], nil
}

func (r *retrievalRepo) AllPersonalityReports(ctx context.Context) ([]retrieval.PersonalityReport, error) {
	return r.fetchReports(ctx, 0)
}

func (r *retrievalRepo) fetchReports(ctx context.Context, chatID int64) ([]retrieval.PersonalityReport, error) {
	args := []any{}
	chatFilter := ""
	if chatID != 0 {
		chatFilter = "AND m.chat_id = $1"
		args = append(args, chatID)
	}

	// Aggregate per-chat counts in one pass.
	countQuery := fmt.Sprintf(`
		SELECT
			c.id,
			c.title,
			c.personality_surface,
			c.relevance_score,
			COUNT(*)                                        AS total,
			COUNT(*) FILTER (WHERE m.is_outgoing)          AS outgoing,
			COUNT(*) FILTER (WHERE m.is_forwarded)         AS forwarded,
			COUNT(*) FILTER (WHERE m.edit_date IS NOT NULL) AS edited,
			COUNT(DISTINCT em.episode_id)                  AS episodes
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE m.is_deleted = FALSE %s
		GROUP BY c.id, c.title, c.personality_surface, c.relevance_score
		ORDER BY c.relevance_score DESC`, chatFilter)

	rows, err := r.pool.Query(ctx, countQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("personality report counts: %w", err)
	}
	defer rows.Close()

	reports := map[int64]*retrieval.PersonalityReport{}
	var order []int64
	for rows.Next() {
		var rep retrieval.PersonalityReport
		var surface string
		if err := rows.Scan(
			&rep.ChatID, &rep.Title, &surface, &rep.Score,
			&rep.TotalMessages, &rep.OutgoingCount, &rep.ForwardedCount,
			&rep.EditedCount, &rep.EpisodeCount,
		); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		rep.Surface = entity.PersonalitySurface(surface)
		rep.HourDistribution = map[int]int{}
		rep.TopEmoji = map[string]int{}
		rep.LengthClassDist = map[string]int{}
		rep.TopSlang = map[string]int{}
		reports[rep.ChatID] = &rep
		order = append(order, rep.ChatID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, nil
	}

	// Hour distribution for outgoing messages.
	if err := r.fillHourDist(ctx, chatID, reports); err != nil {
		return nil, err
	}

	// Writing length distribution.
	if err := r.fillLengthDist(ctx, chatID, reports); err != nil {
		return nil, err
	}

	result := make([]retrieval.PersonalityReport, 0, len(order))
	for _, id := range order {
		result = append(result, *reports[id])
	}
	return result, nil
}

func (r *retrievalRepo) fillHourDist(ctx context.Context, chatID int64, reports map[int64]*retrieval.PersonalityReport) error {
	args := []any{}
	chatFilter := ""
	if chatID != 0 {
		chatFilter = "AND chat_id = $1"
		args = append(args, chatID)
	}
	q := fmt.Sprintf(`
		SELECT chat_id, EXTRACT(HOUR FROM sent_at)::int AS hour, COUNT(*)
		FROM messages
		WHERE is_deleted = FALSE AND is_outgoing = TRUE %s
		GROUP BY chat_id, hour`, chatFilter)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("hour distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int64
		var hour, cnt int
		if err := rows.Scan(&cid, &hour, &cnt); err != nil {
			return err
		}
		if rep, ok := reports[cid]; ok {
			rep.HourDistribution[hour] = cnt
		}
	}
	return rows.Err()
}

func (r *retrievalRepo) fillLengthDist(ctx context.Context, chatID int64, reports map[int64]*retrieval.PersonalityReport) error {
	args := []any{}
	chatFilter := ""
	if chatID != 0 {
		chatFilter = "AND chat_id = $1"
		args = append(args, chatID)
	}
	// Classify outgoing messages by character length.
	q := fmt.Sprintf(`
		SELECT chat_id,
			CASE
				WHEN length(text) <= 10   THEN 'tiny'
				WHEN length(text) <= 50   THEN 'short'
				WHEN length(text) <= 200  THEN 'medium'
				WHEN length(text) <= 500  THEN 'long'
				ELSE 'very_long'
			END AS len_class,
			COUNT(*)
		FROM messages
		WHERE is_deleted = FALSE AND is_outgoing = TRUE
		  AND length(text) > 0 %s
		GROUP BY chat_id, len_class`, chatFilter)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("length distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int64
		var class string
		var cnt int
		if err := rows.Scan(&cid, &class, &cnt); err != nil {
			return err
		}
		if rep, ok := reports[cid]; ok {
			rep.LengthClassDist[class] = cnt
		}
	}
	return rows.Err()
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// messageWhereClause builds additional AND clauses for metadata filters.
// startN is the next available $N parameter index; returns updated n and args.
func messageWhereClause(q retrieval.Query, startN int, args []any) (string, int, []any) {
	var clauses []string
	n := startN

	if q.ChatID != 0 {
		clauses = append(clauses, fmt.Sprintf("AND m.chat_id = $%d", n))
		args = append(args, q.ChatID)
		n++
	}
	if q.Surface != "" {
		clauses = append(clauses, fmt.Sprintf("AND c.personality_surface = $%d", n))
		args = append(args, string(q.Surface))
		n++
	}
	if q.IsOutgoing != nil {
		clauses = append(clauses, fmt.Sprintf("AND m.is_outgoing = $%d", n))
		args = append(args, *q.IsOutgoing)
		n++
	}
	if q.MediaKind != "" {
		clauses = append(clauses, fmt.Sprintf("AND m.media_kind = $%d", n))
		args = append(args, q.MediaKind)
		n++
	}
	if !q.Since.IsZero() {
		clauses = append(clauses, fmt.Sprintf("AND m.sent_at >= $%d", n))
		args = append(args, q.Since)
		n++
	}
	if !q.Until.IsZero() {
		clauses = append(clauses, fmt.Sprintf("AND m.sent_at <= $%d", n))
		args = append(args, q.Until)
		n++
	}

	return strings.Join(clauses, " "), n, args
}

func (r *retrievalRepo) scanMessageHits(ctx context.Context, query string, args []any) ([]retrieval.MessageHit, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var hits []retrieval.MessageHit
	for rows.Next() {
		var h retrieval.MessageHit
		var surface string
		if err := rows.Scan(
			&h.MessageID, &h.ChatID, &h.ChatTitle, &surface,
			&h.Text, &h.SentAt, &h.IsOutgoing, &h.IsForwarded, &h.MediaKind,
			&h.Rank, &h.MatchType, &h.EpisodeID,
		); err != nil {
			return nil, fmt.Errorf("scan message hit: %w", err)
		}
		h.Surface = entity.PersonalitySurface(surface)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
