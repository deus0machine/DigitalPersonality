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

type userRepo struct {
	pool *pgxpool.Pool
}

// NewUserRepository constructs a Postgres-backed UserRepository.
func NewUserRepository(pool *pgxpool.Pool) domrepo.UserRepository {
	return &userRepo{pool: pool}
}

func (r *userRepo) Upsert(ctx context.Context, u *entity.User) error {
	const q = `
		INSERT INTO users (id, username, first_name, last_name, phone, is_self, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (id) DO UPDATE SET
			username   = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name  = EXCLUDED.last_name,
			phone      = EXCLUDED.phone,
			is_self    = EXCLUDED.is_self,
			updated_at = NOW()`

	_, err := r.pool.Exec(ctx, q,
		u.ID, nullString(u.Username), u.FirstName, u.LastName,
		nullString(u.Phone), u.IsSelf,
	)
	if err != nil {
		return fmt.Errorf("upsert user %d: %w", u.ID, err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, id int64) (*entity.User, error) {
	const q = `
		SELECT id, COALESCE(username,''), first_name, last_name,
		       COALESCE(phone,''), is_self, created_at, updated_at
		FROM users WHERE id = $1`

	row := r.pool.QueryRow(ctx, q, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %d: %w", id, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get user %d: %w", id, err)
	}
	return u, nil
}

func (r *userRepo) EnsureExists(ctx context.Context, id int64) error {
	// INSERT ... DO NOTHING guarantees the row exists without overwriting real data.
	const q = `INSERT INTO users (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("ensure user exists %d: %w", id, err)
	}
	return nil
}

func (r *userRepo) GetSelf(ctx context.Context) (*entity.User, error) {
	const q = `
		SELECT id, COALESCE(username,''), first_name, last_name,
		       COALESCE(phone,''), is_self, created_at, updated_at
		FROM users WHERE is_self = TRUE LIMIT 1`

	row := r.pool.QueryRow(ctx, q)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("self user: %w", domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get self: %w", err)
	}
	return u, nil
}

func scanUser(row pgx.Row) (*entity.User, error) {
	u := &entity.User{}
	return u, row.Scan(
		&u.ID, &u.Username, &u.FirstName, &u.LastName,
		&u.Phone, &u.IsSelf, &u.CreatedAt, &u.UpdatedAt,
	)
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
