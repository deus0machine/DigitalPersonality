package entity

import "time"

type User struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	Phone     string
	IsSelf    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
