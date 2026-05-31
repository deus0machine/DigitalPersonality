package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type UserRepository interface {
	Upsert(ctx context.Context, user *entity.User) error
	GetByID(ctx context.Context, id int64) (*entity.User, error)
	GetSelf(ctx context.Context) (*entity.User, error)

	// EnsureExists inserts a minimal stub for id if no record exists.
	// Existing records are never overwritten — use this only as a FK-safety
	// guarantee before inserting messages whose sender may be unknown
	// (deleted accounts, anonymous admins, channel members not in participant list).
	EnsureExists(ctx context.Context, id int64) error
}
