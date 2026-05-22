package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/digital-personality/internal/domain/entity"
	domrepo "github.com/digital-personality/internal/domain/repository"
)

type messageRepo struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) domrepo.MessageRepository {
	return &messageRepo{pool: pool}
}

func (r *messageRepo) Upsert(ctx context.Context, msg *entity.Message) (*entity.Message, error) {
	entitiesJSON, err := json.Marshal(msg.Entities)
	if err != nil {
		return nil, fmt.Errorf("marshal entities: %w", err)
	}
	reactionsJSON, err := json.Marshal(msg.Reactions)
	if err != nil {
		return nil, fmt.Errorf("marshal reactions: %w", err)
	}
	var stickerJSON []byte
	if msg.StickerMeta != nil {
		stickerJSON, err = json.Marshal(msg.StickerMeta)
		if err != nil {
			return nil, fmt.Errorf("marshal sticker: %w", err)
		}
	}

	const q = `
		INSERT INTO messages
			(telegram_id, chat_id, sender_id, reply_to_id, text, raw_data,
			 entities, reactions, sticker_meta, media_kind, sent_at, is_outgoing,
			 is_forwarded, forward_from_id, forward_date, edit_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (telegram_id, chat_id) DO UPDATE SET
			text           = EXCLUDED.text,
			raw_data       = EXCLUDED.raw_data,
			entities       = EXCLUDED.entities,
			reactions      = EXCLUDED.reactions,
			sticker_meta   = EXCLUDED.sticker_meta,
			media_kind     = EXCLUDED.media_kind,
			is_outgoing    = EXCLUDED.is_outgoing,
			is_forwarded   = EXCLUDED.is_forwarded,
			forward_from_id = EXCLUDED.forward_from_id,
			forward_date   = EXCLUDED.forward_date,
			edit_date      = EXCLUDED.edit_date,
			synced_at      = NOW()
		RETURNING id, telegram_id, chat_id,
		          COALESCE(sender_id,0), COALESCE(reply_to_id,0),
		          text, raw_data, entities, reactions, sticker_meta,
		          media_kind, sent_at, synced_at, is_outgoing, is_deleted,
		          is_forwarded, COALESCE(forward_from_id,0), forward_date, edit_date`

	row := r.pool.QueryRow(ctx, q,
		msg.TelegramID, msg.ChatID,
		nullInt64(msg.SenderID), nullInt64(msg.ReplyToID),
		msg.Text, msg.RawData,
		entitiesJSON, reactionsJSON, stickerJSON,
		string(msg.MediaKind), msg.SentAt, msg.IsOutgoing,
		msg.IsForwarded, nullInt64(msg.ForwardFromID),
		nullTime(msg.ForwardDate), nullTime(msg.EditDate),
	)
	return scanMessage(row)
}

func (r *messageRepo) GetByID(ctx context.Context, id int64) (*entity.Message, error) {
	const q = `
		SELECT id, telegram_id, chat_id, COALESCE(sender_id,0), COALESCE(reply_to_id,0),
		       text, raw_data, entities, reactions, sticker_meta,
		       media_kind, sent_at, synced_at, is_outgoing, is_deleted,
		       is_forwarded, COALESCE(forward_from_id,0), forward_date, edit_date
		FROM messages WHERE id = $1`

	msg, err := scanMessage(r.pool.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("message id=%d: %w", id, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get message %d: %w", id, err)
	}
	return msg, nil
}

func (r *messageRepo) GetByTelegramID(ctx context.Context, telegramID, chatID int64) (*entity.Message, error) {
	const q = `
		SELECT id, telegram_id, chat_id, COALESCE(sender_id,0), COALESCE(reply_to_id,0),
		       text, raw_data, entities, reactions, sticker_meta,
		       media_kind, sent_at, synced_at, is_outgoing, is_deleted,
		       is_forwarded, COALESCE(forward_from_id,0), forward_date, edit_date
		FROM messages WHERE telegram_id = $1 AND chat_id = $2`

	msg, err := scanMessage(r.pool.QueryRow(ctx, q, telegramID, chatID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("message tg=%d chat=%d: %w", telegramID, chatID, domrepo.ErrNotFound)
		}
		return nil, fmt.Errorf("get message by telegram id: %w", err)
	}
	return msg, nil
}

func (r *messageRepo) List(ctx context.Context, f domrepo.MessageFilter) ([]*entity.Message, error) {
	args := []any{}
	where := "WHERE is_deleted = FALSE"
	n := 1

	if f.ChatID != 0 {
		where += fmt.Sprintf(" AND chat_id = $%d", n)
		args = append(args, f.ChatID)
		n++
	}
	if f.SenderID != 0 {
		where += fmt.Sprintf(" AND sender_id = $%d", n)
		args = append(args, f.SenderID)
		n++
	}
	if !f.Since.IsZero() {
		where += fmt.Sprintf(" AND sent_at >= $%d", n)
		args = append(args, f.Since)
		n++
	}
	if !f.Until.IsZero() {
		where += fmt.Sprintf(" AND sent_at <= $%d", n)
		args = append(args, f.Until)
		n++
	}
	if f.IsOutgoing != nil {
		where += fmt.Sprintf(" AND is_outgoing = $%d", n)
		args = append(args, *f.IsOutgoing)
		n++
	}

	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	q := fmt.Sprintf(`
		SELECT id, telegram_id, chat_id, COALESCE(sender_id,0), COALESCE(reply_to_id,0),
		       text, raw_data, entities, reactions, sticker_meta,
		       media_kind, sent_at, synced_at, is_outgoing, is_deleted,
		       is_forwarded, COALESCE(forward_from_id,0), forward_date, edit_date
		FROM messages %s ORDER BY sent_at DESC LIMIT $%d OFFSET $%d`,
		where, n, n+1)
	args = append(args, limit, f.Offset)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var msgs []*entity.Message
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, rows.Err()
}

func (r *messageRepo) GetCursor(ctx context.Context, chatID int64) (*entity.SyncCursor, error) {
	const q = `SELECT chat_id, last_message_id, synced_at FROM sync_cursors WHERE chat_id = $1`
	c := &entity.SyncCursor{}
	err := r.pool.QueryRow(ctx, q, chatID).Scan(&c.ChatID, &c.LastMessageID, &c.SyncedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &entity.SyncCursor{ChatID: chatID}, nil
		}
		return nil, fmt.Errorf("get cursor chat=%d: %w", chatID, err)
	}
	return c, nil
}

func (r *messageRepo) SaveCursor(ctx context.Context, cursor *entity.SyncCursor) error {
	const q = `
		INSERT INTO sync_cursors (chat_id, last_message_id, synced_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (chat_id) DO UPDATE SET
			last_message_id = EXCLUDED.last_message_id,
			synced_at       = NOW()`
	_, err := r.pool.Exec(ctx, q, cursor.ChatID, cursor.LastMessageID)
	if err != nil {
		return fmt.Errorf("save cursor chat=%d: %w", cursor.ChatID, err)
	}
	return nil
}

func (r *messageRepo) MarkDeleted(ctx context.Context, telegramID, chatID int64) error {
	const q = `UPDATE messages SET is_deleted = TRUE WHERE telegram_id = $1 AND chat_id = $2`
	_, err := r.pool.Exec(ctx, q, telegramID, chatID)
	return err
}

func nullInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func nullTime(v time.Time) *time.Time {
	if v.IsZero() {
		return nil
	}
	return &v
}

// scanMessage reads a full message row including personality-relevant columns.
func scanMessage(row pgx.Row) (*entity.Message, error) {
	m := &entity.Message{}
	var (
		entitiesJSON  []byte
		reactionsJSON []byte
		stickerJSON   []byte
		mediaKind     string
		forwardDate   *time.Time
		editDate      *time.Time
	)

	err := row.Scan(
		&m.ID, &m.TelegramID, &m.ChatID, &m.SenderID, &m.ReplyToID,
		&m.Text, &m.RawData,
		&entitiesJSON, &reactionsJSON, &stickerJSON,
		&mediaKind, &m.SentAt, &m.SyncedAt, &m.IsOutgoing, &m.IsDeleted,
		&m.IsForwarded, &m.ForwardFromID, &forwardDate, &editDate,
	)
	if err != nil {
		return nil, err
	}
	m.MediaKind = entity.MediaKind(mediaKind)
	if forwardDate != nil {
		m.ForwardDate = *forwardDate
	}
	if editDate != nil {
		m.EditDate = *editDate
	}

	if len(entitiesJSON) > 0 && string(entitiesJSON) != "null" {
		_ = json.Unmarshal(entitiesJSON, &m.Entities)
	}
	if len(reactionsJSON) > 0 && string(reactionsJSON) != "null" {
		_ = json.Unmarshal(reactionsJSON, &m.Reactions)
	}
	if len(stickerJSON) > 0 && string(stickerJSON) != "null" {
		m.StickerMeta = &entity.StickerInfo{}
		_ = json.Unmarshal(stickerJSON, m.StickerMeta)
	}
	return m, nil
}
