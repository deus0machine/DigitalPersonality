package repository

import "errors"

var (
	ErrNotFound    = errors.New("not found")
	ErrDuplicate   = errors.New("duplicate")
	ErrInvalidData = errors.New("invalid data")
)
