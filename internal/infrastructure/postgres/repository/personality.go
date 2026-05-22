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

type personalityRepo struct {
	pool *pgxpool.Pool
}

func NewPersonalityRepository(pool *pgxpool.Pool) domrepo.PersonalityRepository {
	return &personalityRepo{pool: pool}
}

// SaveSignals persists a batch of personality signals in a single transaction.
// Uses UPSERT on (message_id, signal_type) so re-extraction is safe.
func (r *personalityRepo) SaveSignals(ctx context.Context, signals []entity.PersonalitySignal) error {
	if len(signals) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	const q = `
		INSERT INTO personality_signals (message_id, signal_type, value_json, extracted_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (message_id, signal_type) DO UPDATE SET
			value_json   = EXCLUDED.value_json,
			extracted_at = NOW()`

	for i := range signals {
		s := &signals[i]
		if _, err := tx.Exec(ctx, q, s.MessageID, string(s.Type), s.Value); err != nil {
			return fmt.Errorf("save signal type=%s msg=%d: %w", s.Type, s.MessageID, err)
		}
	}

	return tx.Commit(ctx)
}

func (r *personalityRepo) GetSignals(ctx context.Context, messageID int64) ([]entity.PersonalitySignal, error) {
	const q = `
		SELECT id, message_id, signal_type, value_json, extracted_at
		FROM personality_signals WHERE message_id = $1`

	rows, err := r.pool.Query(ctx, q, messageID)
	if err != nil {
		return nil, fmt.Errorf("get signals msg=%d: %w", messageID, err)
	}
	defer rows.Close()

	var signals []entity.PersonalitySignal
	for rows.Next() {
		var s entity.PersonalitySignal
		var sigType string
		if err := rows.Scan(&s.ID, &s.MessageID, &sigType, &s.Value, &s.ExtractedAt); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		s.Type = entity.SignalType(sigType)
		signals = append(signals, s)
	}
	return signals, rows.Err()
}

func (r *personalityRepo) GetSignalsByType(ctx context.Context, signalType entity.SignalType, limit int) ([]entity.PersonalitySignal, error) {
	const q = `
		SELECT id, message_id, signal_type, value_json, extracted_at
		FROM personality_signals WHERE signal_type = $1
		ORDER BY extracted_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, q, string(signalType), limit)
	if err != nil {
		return nil, fmt.Errorf("get signals type=%s: %w", signalType, err)
	}
	defer rows.Close()

	var signals []entity.PersonalitySignal
	for rows.Next() {
		var s entity.PersonalitySignal
		var sigType string
		if err := rows.Scan(&s.ID, &s.MessageID, &sigType, &s.Value, &s.ExtractedAt); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		s.Type = entity.SignalType(sigType)
		signals = append(signals, s)
	}
	return signals, rows.Err()
}

func (r *personalityRepo) UpsertProfile(ctx context.Context, profile *entity.PersonalityProfile) error {
	const q = `
		INSERT INTO personality_profiles (user_id, features, signal_count, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			features     = EXCLUDED.features,
			signal_count = EXCLUDED.signal_count,
			updated_at   = NOW()`

	_, err := r.pool.Exec(ctx, q, profile.UserID, profile.Features, profile.SignalCount)
	if err != nil {
		return fmt.Errorf("upsert profile user=%d: %w", profile.UserID, err)
	}
	return nil
}

func (r *personalityRepo) GetProfile(ctx context.Context, userID int64) (*entity.PersonalityProfile, error) {
	const q = `
		SELECT user_id, features, signal_count, updated_at
		FROM personality_profiles WHERE user_id = $1`

	p := &entity.PersonalityProfile{}
	err := r.pool.QueryRow(ctx, q, userID).Scan(
		&p.UserID, &p.Features, &p.SignalCount, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("profile user=%d: %w", userID, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get profile user=%d: %w", userID, err)
	}
	return p, nil
}
