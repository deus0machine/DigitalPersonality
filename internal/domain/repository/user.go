package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

type UserRepository interface {
	Upsert(ctx context.Context, user *entity.User) error
	GetByID(ctx context.Context, id int64) (*entity.User, error)
	GetSelf(ctx context.Context) (*entity.User, error)
}
