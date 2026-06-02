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
		WHERE m.is_deleted = FALSE
		  AND m.in_memory_window = TRUE %s
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

// ─── WindowStats ─────────────────────────────────────────────────────────────

func (r *retrievalRepo) WindowStats(ctx context.Context, chatID int64) ([]retrieval.WindowStat, error) {
	const q = `
		SELECT
			c.id, c.title, c.personality_surface,
			COUNT(*)                                                              AS total,
			COUNT(*) FILTER (WHERE m.in_memory_window)                           AS in_window,
			COUNT(*) FILTER (WHERE m.is_outgoing)                                AS anchors,
			COUNT(*) FILTER (WHERE m.in_memory_window AND ms.message_id IS NULL) AS pending
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		LEFT JOIN message_semantic ms ON ms.message_id = m.id
		WHERE NOT m.is_deleted
		  AND ($1::bigint = 0 OR c.id = $1)
		GROUP BY c.id, c.title, c.personality_surface
		ORDER BY total DESC`

	rows, err := r.pool.Query(ctx, q, chatID)
	if err != nil {
		return nil, fmt.Errorf("window stats: %w", err)
	}
	defer rows.Close()

	var stats []retrieval.WindowStat
	for rows.Next() {
		var s retrieval.WindowStat
		var surface string
		if err := rows.Scan(
			&s.ChatID, &s.ChatTitle, &surface,
			&s.TotalMessages, &s.InWindowCount, &s.AnchorCount, &s.PendingRebuild,
		); err != nil {
			return nil, fmt.Errorf("scan window stat: %w", err)
		}
		s.Surface = entity.PersonalitySurface(surface)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// ─── WindowAnchors ────────────────────────────────────────────────────────────

func (r *retrievalRepo) WindowAnchors(ctx context.Context, chatID int64, windowBefore, windowAfter, anchorLimit int) ([]retrieval.WindowAnchor, error) {
	const q = `
		WITH ordered AS (
			SELECT
				telegram_id, text, sent_at, is_outgoing, in_memory_window,
				ROW_NUMBER() OVER (ORDER BY sent_at ASC, telegram_id ASC) AS rn
			FROM messages
			WHERE chat_id = $1 AND NOT is_deleted
		),
		anchors AS (
			SELECT rn FROM ordered WHERE is_outgoing ORDER BY rn ASC LIMIT $2
		)
		SELECT o.telegram_id, o.text, o.sent_at, o.is_outgoing, o.in_memory_window, a.rn AS anchor_rn
		FROM ordered o
		JOIN anchors a ON o.rn BETWEEN a.rn - $3 AND a.rn + $4
		ORDER BY a.rn, o.rn`

	rows, err := r.pool.Query(ctx, q, chatID, anchorLimit, windowBefore, windowAfter)
	if err != nil {
		return nil, fmt.Errorf("window anchors chat=%d: %w", chatID, err)
	}
	defer rows.Close()

	var (
		result    []retrieval.WindowAnchor
		current   *retrieval.WindowAnchor
		currentRN int64
	)
	for rows.Next() {
		var msg retrieval.WindowMessage
		var anchorRN int64
		if err := rows.Scan(
			&msg.TelegramID, &msg.Text, &msg.SentAt,
			&msg.IsOutgoing, &msg.InWindow, &anchorRN,
		); err != nil {
			return nil, fmt.Errorf("scan window anchor row: %w", err)
		}
		if current == nil || anchorRN != currentRN {
			result = append(result, retrieval.WindowAnchor{})
			current = &result[len(result)-1]
			currentRN = anchorRN
		}
		current.Messages = append(current.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ─── WindowAnchorsDistributed ─────────────────────────────────────────────────

// WindowAnchorsDistributed returns up to 3 participation windows sampled across
// the chat's full temporal range: ~10th percentile (early), ~50th (middle), last (late).
// For chats with ≤ 3 outgoing messages the result may contain fewer than 3 windows.
func (r *retrievalRepo) WindowAnchorsDistributed(ctx context.Context, chatID int64, windowBefore, windowAfter int) ([]retrieval.WindowAnchor, error) {
	// anchor_pool ranks every outgoing message 1..N by chronological order.
	// sampled picks three row-numbers at ~10th, ~50th, and 100th percentile.
	// DISTINCT collapses duplicates when total anchor count is small (< 3).
	const q = `
		WITH ordered AS (
			SELECT
				telegram_id, text, sent_at, is_outgoing, in_memory_window,
				ROW_NUMBER() OVER (ORDER BY sent_at ASC, telegram_id ASC) AS rn
			FROM messages
			WHERE chat_id = $1 AND NOT is_deleted
		),
		anchor_pool AS (
			SELECT rn, ROW_NUMBER() OVER (ORDER BY rn ASC) AS anchor_rank
			FROM ordered
			WHERE is_outgoing
		),
		sampled AS (
			SELECT DISTINCT ap.rn
			FROM anchor_pool ap
			WHERE ap.anchor_rank IN (
				GREATEST(1, (SELECT (COUNT(*) + 9) / 10 FROM anchor_pool)),
				GREATEST(1, (SELECT (COUNT(*) + 1) / 2 FROM anchor_pool)),
				(SELECT COUNT(*) FROM anchor_pool)
			)
		)
		SELECT o.telegram_id, o.text, o.sent_at, o.is_outgoing, o.in_memory_window, s.rn AS anchor_rn
		FROM ordered o
		JOIN sampled s ON o.rn BETWEEN s.rn - $2 AND s.rn + $3
		ORDER BY s.rn, o.rn`

	rows, err := r.pool.Query(ctx, q, chatID, windowBefore, windowAfter)
	if err != nil {
		return nil, fmt.Errorf("window anchors distributed chat=%d: %w", chatID, err)
	}
	defer rows.Close()

	var (
		result    []retrieval.WindowAnchor
		current   *retrieval.WindowAnchor
		currentRN int64
	)
	for rows.Next() {
		var msg retrieval.WindowMessage
		var anchorRN int64
		if err := rows.Scan(
			&msg.TelegramID, &msg.Text, &msg.SentAt,
			&msg.IsOutgoing, &msg.InWindow, &anchorRN,
		); err != nil {
			return nil, fmt.Errorf("scan distributed anchor row: %w", err)
		}
		if current == nil || anchorRN != currentRN {
			result = append(result, retrieval.WindowAnchor{})
			current = &result[len(result)-1]
			currentRN = anchorRN
		}
		current.Messages = append(current.Messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ─── ValidationStats ──────────────────────────────────────────────────────────

func (r *retrievalRepo) ValidationStats(ctx context.Context) (*retrieval.ValidationStats, error) {
	stats := &retrieval.ValidationStats{
		ChatsBySurface: map[string]int{},
	}

	// Message counts in one pass.
	msgRow := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE in_memory_window),
			COUNT(*) FILTER (WHERE is_outgoing)
		FROM messages WHERE NOT is_deleted`)
	if err := msgRow.Scan(&stats.TotalMessages, &stats.InWindowMessages, &stats.OutgoingMessages); err != nil {
		return nil, fmt.Errorf("message stats: %w", err)
	}

	// Episode count and average size.
	epRow := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(AVG(message_count), 0) FROM episodes`)
	if err := epRow.Scan(&stats.TotalEpisodes, &stats.AvgEpisodeSize); err != nil {
		return nil, fmt.Errorf("episode stats: %w", err)
	}

	// Personality signal count.
	sigRow := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM personality_signals`)
	if err := sigRow.Scan(&stats.TotalSignals); err != nil {
		return nil, fmt.Errorf("signal stats: %w", err)
	}

	// Chat count grouped by personality surface.
	surfRows, err := r.pool.Query(ctx, `
		SELECT personality_surface, COUNT(*)
		FROM chats
		WHERE personality_surface != ''
		GROUP BY personality_surface
		ORDER BY personality_surface`)
	if err != nil {
		return nil, fmt.Errorf("chats by surface: %w", err)
	}
	defer surfRows.Close()
	for surfRows.Next() {
		var surface string
		var cnt int
		if err := surfRows.Scan(&surface, &cnt); err != nil {
			return nil, fmt.Errorf("scan surface row: %w", err)
		}
		stats.ChatsBySurface[surface] = cnt
	}
	if err := surfRows.Err(); err != nil {
		return nil, err
	}

	// High-score chats (>0.8) that have no messages — possible sync gaps.
	emptyRows, err := r.pool.Query(ctx, `
		SELECT c.id, c.title, c.personality_surface, c.relevance_score
		FROM chats c
		LEFT JOIN messages m ON m.chat_id = c.id AND NOT m.is_deleted
		WHERE c.relevance_score > 0.8
		GROUP BY c.id, c.title, c.personality_surface, c.relevance_score
		HAVING COUNT(m.id) = 0
		ORDER BY c.relevance_score DESC`)
	if err != nil {
		return nil, fmt.Errorf("high-score empty chats: %w", err)
	}
	defer emptyRows.Close()
	for emptyRows.Next() {
		var cs retrieval.ChatSummary
		var surface string
		if err := emptyRows.Scan(&cs.ChatID, &cs.Title, &surface, &cs.Score); err != nil {
			return nil, fmt.Errorf("scan high-score empty chat: %w", err)
		}
		cs.Surface = entity.PersonalitySurface(surface)
		stats.HighScoreEmpty = append(stats.HighScoreEmpty, cs)
	}
	if err := emptyRows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// ─── TopChatsByVolume ─────────────────────────────────────────────────────────

func (r *retrievalRepo) TopChatsByVolume(ctx context.Context, limit int) ([]retrieval.TopChatEntry, error) {
	const q = `
		SELECT
			c.id, c.title, c.personality_surface, c.relevance_score,
			COUNT(m.id) AS total,
			COUNT(*) FILTER (WHERE m.in_memory_window) AS in_window,
			COUNT(*) FILTER (WHERE m.is_outgoing) AS outgoing,
			COUNT(DISTINCT em.episode_id) AS episodes
		FROM chats c
		LEFT JOIN messages m ON m.chat_id = c.id AND NOT m.is_deleted
		LEFT JOIN episode_messages em ON em.message_id = m.id
		GROUP BY c.id, c.title, c.personality_surface, c.relevance_score
		ORDER BY total DESC
		LIMIT $1`

	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("top chats by volume: %w", err)
	}
	defer rows.Close()

	var entries []retrieval.TopChatEntry
	for rows.Next() {
		var e retrieval.TopChatEntry
		var surface string
		if err := rows.Scan(
			&e.ChatID, &e.Title, &surface, &e.Score,
			&e.Total, &e.InWindow, &e.Outgoing, &e.EpisodeCount,
		); err != nil {
			return nil, fmt.Errorf("scan top chat entry: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surface)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ─── ChatInspect ──────────────────────────────────────────────────────────────

func (r *retrievalRepo) ChatInspect(ctx context.Context, chatID int64) (*retrieval.ChatInspectReport, error) {
	const mainQ = `
		SELECT
			c.id, c.title, c.personality_surface, c.relevance_score,
			COUNT(m.id)                                                  AS total,
			COUNT(*) FILTER (WHERE m.is_outgoing)                        AS outgoing,
			COUNT(*) FILTER (WHERE m.in_memory_window)                   AS in_window,
			COUNT(*) FILTER (WHERE m.is_outgoing AND m.in_memory_window) AS outgoing_in_window,
			COUNT(*) FILTER (WHERE NOT m.is_outgoing AND m.in_memory_window) AS incoming_in_window,
			COUNT(DISTINCT em.episode_id)                                AS episodes
		FROM chats c
		LEFT JOIN messages m ON m.chat_id = c.id AND NOT m.is_deleted
		LEFT JOIN episode_messages em ON em.message_id = m.id
		WHERE c.id = $1
		GROUP BY c.id, c.title, c.personality_surface, c.relevance_score`

	var rep retrieval.ChatInspectReport
	var surface string
	row := r.pool.QueryRow(ctx, mainQ, chatID)
	if err := row.Scan(
		&rep.ChatID, &rep.Title, &surface, &rep.Score,
		&rep.Total, &rep.Outgoing, &rep.InWindow,
		&rep.OutgoingInWindow, &rep.IncomingInWindow,
		&rep.EpisodeCount,
	); err != nil {
		return nil, fmt.Errorf("chat inspect %d: %w", chatID, err)
	}
	rep.Surface = entity.PersonalitySurface(surface)

	// Count step-3 reply targets that fall outside the row-proximity windows.
	// These are non-outgoing in-window messages whose telegram_id matches reply_to_id
	// of an outgoing message, but are NOT within ±WINDOW rows of any anchor by row number.
	// We approximate using the configurable default (10/10); exact values would require
	// passing window sizes. Non-zero count signals isolated reply-target inclusions.
	isolatedRow := r.pool.QueryRow(ctx, `
		WITH ordered AS (
			SELECT id, is_outgoing, in_memory_window,
			       ROW_NUMBER() OVER (ORDER BY sent_at ASC, telegram_id ASC) AS rn
			FROM messages
			WHERE chat_id = $1 AND NOT is_deleted
		),
		anchor_rns AS (
			SELECT rn FROM ordered WHERE is_outgoing
		)
		SELECT COUNT(*)
		FROM ordered o
		WHERE o.in_memory_window = TRUE
		  AND NOT o.is_outgoing
		  AND NOT EXISTS (
		      SELECT 1 FROM anchor_rns ar
		      WHERE o.rn BETWEEN ar.rn - 10 AND ar.rn + 10
		  )`, chatID)
	if err := isolatedRow.Scan(&rep.IsolatedInWindow); err != nil {
		return nil, fmt.Errorf("isolated in-window count %d: %w", chatID, err)
	}

	// Episode size distribution (min/avg/max).
	epStatsRow := r.pool.QueryRow(ctx, `
		SELECT
			COALESCE(MIN(message_count), 0),
			COALESCE(AVG(message_count), 0),
			COALESCE(MAX(message_count), 0)
		FROM episodes WHERE chat_id = $1`, chatID)
	if err := epStatsRow.Scan(&rep.EpisodeMin, &rep.EpisodeAvg, &rep.EpisodeMax); err != nil {
		return nil, fmt.Errorf("episode stats %d: %w", chatID, err)
	}

	// Top 5 largest episodes.
	epRows, err := r.pool.Query(ctx, `
		SELECT id, started_at, ended_at, message_count
		FROM episodes
		WHERE chat_id = $1
		ORDER BY message_count DESC
		LIMIT 5`, chatID)
	if err != nil {
		return nil, fmt.Errorf("top episodes %d: %w", chatID, err)
	}
	defer epRows.Close()
	for epRows.Next() {
		var e retrieval.EpisodeEntry
		if err := epRows.Scan(&e.EpisodeID, &e.StartedAt, &e.EndedAt, &e.MessageCount); err != nil {
			return nil, fmt.Errorf("scan top episode: %w", err)
		}
		rep.LargestEpisodes = append(rep.LargestEpisodes, e)
	}
	if err := epRows.Err(); err != nil {
		return nil, err
	}

	return &rep, nil
}

// ─── MediaInspect ─────────────────────────────────────────────────────────────

func (r *retrievalRepo) MediaInspect(ctx context.Context) (*retrieval.MediaInspectReport, error) {
	rep := &retrieval.MediaInspectReport{}
	var err error

	// 1. Global kind stats.
	kindRows, err := r.pool.Query(ctx, `
		SELECT
			CASE WHEN media_kind = '' THEN 'text' ELSE media_kind END AS kind,
			COUNT(*)                                     AS total,
			COUNT(*) FILTER (WHERE in_memory_window)     AS in_window,
			COUNT(*) FILTER (WHERE is_outgoing)          AS outgoing
		FROM messages
		WHERE NOT is_deleted
		GROUP BY kind
		ORDER BY total DESC`)
	if err != nil {
		return nil, fmt.Errorf("media kind stats: %w", err)
	}
	defer kindRows.Close()
	for kindRows.Next() {
		var s retrieval.MediaKindStat
		if err := kindRows.Scan(&s.Kind, &s.Total, &s.InWindow, &s.Outgoing); err != nil {
			return nil, fmt.Errorf("scan kind stat: %w", err)
		}
		rep.KindStats = append(rep.KindStats, s)
	}
	if err := kindRows.Err(); err != nil {
		return nil, err
	}

	// 2. Voice detail.
	if rep.Voice, err = r.fetchMediaKindDetail(ctx, "voice"); err != nil {
		return nil, err
	}

	// 3. Round detail.
	if rep.Round, err = r.fetchMediaKindDetail(ctx, "round"); err != nil {
		return nil, err
	}

	// 4. Sticker detail.
	if rep.Sticker, err = r.fetchStickerDetail(ctx); err != nil {
		return nil, err
	}

	// 5. Photo detail.
	if rep.Photo, err = r.fetchPhotoDetail(ctx); err != nil {
		return nil, err
	}

	return rep, nil
}

// fetchMediaKindDetail fetches totals + top-5 chats (all / interpersonal / social)
// for a given media_kind value.
func (r *retrievalRepo) fetchMediaKindDetail(ctx context.Context, kind string) (retrieval.MediaKindDetail, error) {
	var d retrieval.MediaKindDetail

	totRow := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE in_memory_window), COUNT(*) FILTER (WHERE is_outgoing)
		FROM messages WHERE media_kind = $1 AND NOT is_deleted`, kind)
	if err := totRow.Scan(&d.Total, &d.InWindow, &d.Outgoing); err != nil {
		return d, fmt.Errorf("media kind %s totals: %w", kind, err)
	}

	var err error
	if d.TopChats, err = r.fetchMediaTopChats(ctx, kind, "", 5); err != nil {
		return d, err
	}
	if d.TopInterpersonal, err = r.fetchMediaTopChats(ctx, kind, "interpersonal", 5); err != nil {
		return d, err
	}
	if d.TopSocial, err = r.fetchMediaTopChats(ctx, kind, "social", 5); err != nil {
		return d, err
	}
	return d, nil
}

// fetchMediaTopChats returns the top-N chats by message count for a given media_kind,
// optionally filtered to one personality surface (empty string = all surfaces).
func (r *retrievalRepo) fetchMediaTopChats(ctx context.Context, kind, surface string, limit int) ([]retrieval.MediaChatEntry, error) {
	surfaceClause := ""
	args := []any{kind}
	if surface != "" {
		surfaceClause = "AND c.personality_surface = $2"
		args = append(args, surface)
	}
	args = append(args, limit)
	limitParam := len(args)

	query := fmt.Sprintf(`
		SELECT c.id, c.title, c.personality_surface,
		       COUNT(*)                                 AS total,
		       COUNT(*) FILTER (WHERE m.in_memory_window) AS in_window,
		       COUNT(*) FILTER (WHERE m.is_outgoing)    AS outgoing
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		WHERE m.media_kind = $1 AND NOT m.is_deleted %s
		GROUP BY c.id, c.title, c.personality_surface
		ORDER BY total DESC
		LIMIT $%d`, surfaceClause, limitParam)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("media top chats kind=%s surface=%s: %w", kind, surface, err)
	}
	defer rows.Close()

	var entries []retrieval.MediaChatEntry
	for rows.Next() {
		var e retrieval.MediaChatEntry
		var surf string
		if err := rows.Scan(&e.ChatID, &e.Title, &surf, &e.Total, &e.InWindow, &e.Outgoing); err != nil {
			return nil, fmt.Errorf("scan media chat entry: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surf)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *retrievalRepo) fetchStickerDetail(ctx context.Context) (retrieval.StickerDetail, error) {
	var d retrieval.StickerDetail

	totRow := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE in_memory_window), COUNT(*) FILTER (WHERE is_outgoing)
		FROM messages WHERE media_kind = 'sticker' AND NOT is_deleted`)
	if err := totRow.Scan(&d.Total, &d.InWindow, &d.Outgoing); err != nil {
		return d, fmt.Errorf("sticker totals: %w", err)
	}

	// Top 10 emoticons. entity.StickerInfo marshals without json tags → key "Emoticon".
	emoRows, err := r.pool.Query(ctx, `
		SELECT sticker_meta->>'Emoticon' AS emoticon, COUNT(*) AS cnt
		FROM messages
		WHERE media_kind = 'sticker' AND NOT is_deleted
		  AND sticker_meta IS NOT NULL
		  AND sticker_meta->>'Emoticon' != ''
		  AND sticker_meta->>'Emoticon' IS NOT NULL
		GROUP BY emoticon
		ORDER BY cnt DESC
		LIMIT 10`)
	if err != nil {
		return d, fmt.Errorf("sticker emoticons: %w", err)
	}
	defer emoRows.Close()
	for emoRows.Next() {
		var e retrieval.StickerEmoticonEntry
		if err := emoRows.Scan(&e.Emoticon, &e.Count); err != nil {
			return d, fmt.Errorf("scan emoticon: %w", err)
		}
		d.TopEmoticons = append(d.TopEmoticons, e)
	}
	if err := emoRows.Err(); err != nil {
		return d, err
	}

	if d.TopChats, err = r.fetchMediaTopChats(ctx, "sticker", "", 5); err != nil {
		return d, err
	}

	// Surface distribution.
	surfRows, err := r.pool.Query(ctx, `
		SELECT c.personality_surface,
		       COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE m.in_memory_window) AS in_window
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		WHERE m.media_kind = 'sticker' AND NOT m.is_deleted
		  AND c.personality_surface != ''
		GROUP BY c.personality_surface
		ORDER BY total DESC`)
	if err != nil {
		return d, fmt.Errorf("sticker by surface: %w", err)
	}
	defer surfRows.Close()
	for surfRows.Next() {
		var e retrieval.MediaSurfaceEntry
		var surf string
		if err := surfRows.Scan(&surf, &e.Total, &e.InWindow); err != nil {
			return d, fmt.Errorf("scan sticker surface: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surf)
		d.BySurface = append(d.BySurface, e)
	}
	return d, surfRows.Err()
}

func (r *retrievalRepo) fetchPhotoDetail(ctx context.Context) (retrieval.PhotoDetail, error) {
	var d retrieval.PhotoDetail

	totRow := r.pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE in_memory_window), COUNT(*) FILTER (WHERE is_outgoing)
		FROM messages WHERE media_kind = 'photo' AND NOT is_deleted`)
	if err := totRow.Scan(&d.Total, &d.InWindow, &d.Outgoing); err != nil {
		return d, fmt.Errorf("photo totals: %w", err)
	}

	var err error
	if d.TopChats, err = r.fetchMediaTopChats(ctx, "photo", "", 5); err != nil {
		return d, err
	}

	surfRows, err := r.pool.Query(ctx, `
		SELECT c.personality_surface,
		       COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE m.in_memory_window) AS in_window
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		WHERE m.media_kind = 'photo' AND NOT m.is_deleted
		  AND c.personality_surface != ''
		GROUP BY c.personality_surface
		ORDER BY total DESC`)
	if err != nil {
		return d, fmt.Errorf("photo by surface: %w", err)
	}
	defer surfRows.Close()
	for surfRows.Next() {
		var e retrieval.MediaSurfaceEntry
		var surf string
		if err := surfRows.Scan(&surf, &e.Total, &e.InWindow); err != nil {
			return d, fmt.Errorf("scan photo surface: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surf)
		d.BySurface = append(d.BySurface, e)
	}
	return d, surfRows.Err()
}

// ─── VoiceStats ───────────────────────────────────────────────────────────────

func (r *retrievalRepo) VoiceStats(ctx context.Context) (*retrieval.VoiceStats, error) {
	stats := &retrieval.VoiceStats{}

	// Global totals including in_memory_window count.
	totRow := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE is_outgoing),
			COUNT(*) FILTER (WHERE in_memory_window)
		FROM messages
		WHERE media_kind = 'voice' AND NOT is_deleted`)
	if err := totRow.Scan(&stats.TotalVoice, &stats.OutgoingVoice, &stats.VoiceInWindow); err != nil {
		return nil, fmt.Errorf("voice global totals: %w", err)
	}

	// Voice in_window breakdown by personality surface.
	surfRows, err := r.pool.Query(ctx, `
		SELECT c.personality_surface, COUNT(*) FILTER (WHERE m.in_memory_window)
		FROM messages m
		JOIN chats c ON c.id = m.chat_id
		WHERE m.media_kind = 'voice' AND NOT m.is_deleted
		  AND c.personality_surface != ''
		GROUP BY c.personality_surface
		ORDER BY c.personality_surface`)
	if err != nil {
		return nil, fmt.Errorf("voice by surface: %w", err)
	}
	defer surfRows.Close()
	for surfRows.Next() {
		var e retrieval.VoiceSurfaceEntry
		var surface string
		if err := surfRows.Scan(&surface, &e.VoiceInWindow); err != nil {
			return nil, fmt.Errorf("scan voice surface entry: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surface)
		stats.BySurface = append(stats.BySurface, e)
	}
	if err := surfRows.Err(); err != nil {
		return nil, err
	}

	// Top 20 chats by voice message count.
	rows, err := r.pool.Query(ctx, `
		SELECT
			c.id, c.title, c.personality_surface, c.relevance_score,
			COUNT(m.id)                                    AS voice_count,
			COUNT(*) FILTER (WHERE m.is_outgoing)          AS outgoing_count,
			COUNT(*) FILTER (WHERE m.in_memory_window)     AS in_window_count
		FROM chats c
		JOIN messages m ON m.chat_id = c.id
			AND NOT m.is_deleted
			AND m.media_kind = 'voice'
		GROUP BY c.id, c.title, c.personality_surface, c.relevance_score
		ORDER BY voice_count DESC
		LIMIT 20`)
	if err != nil {
		return nil, fmt.Errorf("voice by chat: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e retrieval.VoiceChatEntry
		var surface string
		if err := rows.Scan(
			&e.ChatID, &e.Title, &surface, &e.Score,
			&e.VoiceCount, &e.OutgoingCount, &e.InWindowCount,
		); err != nil {
			return nil, fmt.Errorf("scan voice chat entry: %w", err)
		}
		e.Surface = entity.PersonalitySurface(surface)
		stats.TopChats = append(stats.TopChats, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

// ─── Shared helpers ───────────────────────────────────────────────────────────

// messageWhereClause builds additional AND clauses for metadata filters.
// startN is the next available $N parameter index; returns updated n and args.
func messageWhereClause(q retrieval.Query, startN int, args []any) (string, int, []any) {
	// Always exclude messages outside the participation window.
	// For full-sync surfaces (interpersonal, self_expression, tool_interaction)
	// in_memory_window is always TRUE, so this filter is a no-op on those surfaces.
	clauses := []string{"AND m.in_memory_window = TRUE"}
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
