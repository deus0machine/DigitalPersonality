package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/application/persona"
)

type styleRepo struct {
	pool *pgxpool.Pool
}

// NewStyleRepository creates the persona style profile aggregator.
func NewStyleRepository(pool *pgxpool.Pool) persona.StyleRepository {
	return &styleRepo{pool: pool}
}

// LoadStyleProfile aggregates the user's outgoing in-window communication style.
// Burst semantics mirror utterance.Build: a burst breaks when the previous
// message in the chat is not outgoing or the pause exceeds burstGapSeconds.
func (r *styleRepo) LoadStyleProfile(ctx context.Context, burstGapSeconds int) (*persona.StyleProfile, error) {
	profile := &persona.StyleProfile{
		LengthDist: map[string]float64{},
	}

	if err := r.fillLengthShares(ctx, profile); err != nil {
		return nil, err
	}
	if err := r.fillTopItems(ctx, "slang_markers", 15, &profile.TopSlang); err != nil {
		return nil, err
	}
	if err := r.fillTopItems(ctx, "emoji_usage", 10, &profile.TopEmoji); err != nil {
		return nil, err
	}
	if err := r.fillBurstStats(ctx, burstGapSeconds, profile); err != nil {
		return nil, err
	}
	return profile, nil
}

func (r *styleRepo) fillLengthShares(ctx context.Context, profile *persona.StyleProfile) error {
	const q = `
		SELECT
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
		  AND in_memory_window = TRUE AND length(text) > 0
		GROUP BY len_class`

	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("style length shares: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	total := 0
	for rows.Next() {
		var class string
		var cnt int
		if err := rows.Scan(&class, &cnt); err != nil {
			return err
		}
		counts[class] = cnt
		total += cnt
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for class, cnt := range counts {
		profile.LengthDist[class] = float64(cnt) / float64(total)
	}
	return nil
}

// fillTopItems aggregates a JSON-array signal (slang_markers, emoji_usage)
// into a global outgoing top-N list, filtering emoji noise tokens.
func (r *styleRepo) fillTopItems(ctx context.Context, signalType string, limit int, dest *[]string) error {
	// Overfetch to survive noise filtering.
	const q = `
		SELECT e.item
		FROM personality_signals ps
		JOIN messages m ON m.id = ps.message_id
		CROSS JOIN LATERAL jsonb_array_elements_text(ps.value_json) AS e(item)
		WHERE ps.signal_type = $1
		  AND m.is_deleted = FALSE AND m.is_outgoing = TRUE
		  AND m.in_memory_window = TRUE
		GROUP BY e.item
		ORDER BY COUNT(*) DESC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, q, signalType, limit*2)
	if err != nil {
		return fmt.Errorf("style top %s: %w", signalType, err)
	}
	defer rows.Close()

	for rows.Next() {
		var item string
		if err := rows.Scan(&item); err != nil {
			return err
		}
		if isEmojiNoise(item) {
			continue
		}
		*dest = append(*dest, item)
		if len(*dest) >= limit {
			break
		}
	}
	return rows.Err()
}

func (r *styleRepo) fillBurstStats(ctx context.Context, burstGapSeconds int, profile *persona.StyleProfile) error {
	// seq walks every in-window message per chat so incoming messages break
	// outgoing bursts — same semantics as utterance.Build author-change rule.
	const q = `
		WITH seq AS (
			SELECT chat_id, id, sent_at, is_outgoing,
				LAG(is_outgoing) OVER w AS prev_outgoing,
				EXTRACT(EPOCH FROM (sent_at - LAG(sent_at) OVER w)) AS gap
			FROM messages
			WHERE is_deleted = FALSE AND in_memory_window = TRUE
			WINDOW w AS (PARTITION BY chat_id ORDER BY sent_at, id)
		),
		outgoing AS (
			SELECT chat_id, id, sent_at, gap,
				CASE WHEN NOT COALESCE(prev_outgoing, FALSE) OR gap > $1 THEN 1 ELSE 0 END AS new_burst
			FROM seq
			WHERE is_outgoing
		),
		bursts AS (
			SELECT chat_id, gap, new_burst,
				SUM(new_burst) OVER (PARTITION BY chat_id ORDER BY sent_at, id) AS burst_id
			FROM outgoing
		),
		sizes AS (
			SELECT COUNT(*) AS size FROM bursts GROUP BY chat_id, burst_id
		),
		size_stats AS (
			SELECT AVG(size)::float8 AS avg_size,
				percentile_cont(0.9) WITHIN GROUP (ORDER BY size)::float8 AS p90_size
			FROM sizes
		),
		gap_stats AS (
			SELECT
				COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY gap), 0)::float8 AS p50_gap,
				COALESCE(percentile_cont(0.9) WITHIN GROUP (ORDER BY gap), 0)::float8 AS p90_gap
			FROM bursts
			WHERE new_burst = 0 AND gap >= 0
		)
		SELECT s.avg_size, s.p90_size, g.p50_gap, g.p90_gap
		FROM size_stats s CROSS JOIN gap_stats g`

	err := r.pool.QueryRow(ctx, q, burstGapSeconds).Scan(
		&profile.AvgBurstSize, &profile.P90BurstSize,
		&profile.GapP50Seconds, &profile.GapP90Seconds,
	)
	if err != nil {
		return fmt.Errorf("style burst stats: %w", err)
	}
	return nil
}
