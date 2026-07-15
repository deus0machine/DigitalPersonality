package entity

import "time"

// BotMessage is one message in a conversation between the digital persona
// (Telegram bot) and an external interlocutor.
// UserID is the interlocutor's Telegram id on both directions, so a whole
// dialog can be fetched by one user id.
type BotMessage struct {
	ID          int64
	ChatID      int64
	UserID      int64
	Username    string
	FromPersona bool
	Text        string
	CreatedAt   time.Time
}
