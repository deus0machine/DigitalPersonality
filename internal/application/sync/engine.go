// Package sync contains application-layer use cases for Telegram data ingestion.
// It depends only on repository interfaces and application ports —
// no infrastructure types cross this boundary.
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/digital-personality/internal/application/episode"
	"github.com/digital-personality/internal/application/port"
	appwindow "github.com/digital-personality/internal/application/window"
	"github.com/digital-personality/internal/domain/entity"
	"github.com/digital-personality/internal/domain/repository"
)

const (
	defaultBatchSize   = 100
	defaultDialogDelay = 150 * time.Millisecond
	defaultHistoryDelay = 200 * time.Millisecond
)

// Engine orchestrates the Telegram backfill pipeline across all four memory layers:
//
//	Layer 1 (Raw):         messages table — all personality data preserved
//	Layer 2 (Semantic):    message_semantic — normalized text for future embedding
//	Layer 3 (Personality): personality_signals — extracted per-message features
//	Layer 4 (Episodic):    episodes — coherent conversational memory units
type Engine struct {
	gateway        port.TelegramGateway
	normalizer     port.SemanticNormalizer
	extractor      port.PersonalityExtractor
	episodeBuilder *episode.Builder
	windowExpander *appwindow.Expander
	scorer         *ChatRelevanceScorer
	msgRepo        repository.MessageRepository
	chatRepo       repository.ChatRepository
	userRepo       repository.UserRepository
	semanticRepo   repository.SemanticRepository
	personRepo     repository.PersonalityRepository
	log            *slog.Logger
	batchSize      int
	historyDelay   time.Duration
}

// NewEngine constructs a sync Engine. All parameters are required.
func NewEngine(
	gateway port.TelegramGateway,
	normalizer port.SemanticNormalizer,
	extractor port.PersonalityExtractor,
	episodeBuilder *episode.Builder,
	windowExpander *appwindow.Expander,
	scorer *ChatRelevanceScorer,
	msgRepo repository.MessageRepository,
	chatRepo repository.ChatRepository,
	userRepo repository.UserRepository,
	semanticRepo repository.SemanticRepository,
	personRepo repository.PersonalityRepository,
	historyDelay time.Duration,
	log *slog.Logger,
) *Engine {
	if historyDelay <= 0 {
		historyDelay = defaultHistoryDelay
	}
	return &Engine{
		gateway:        gateway,
		normalizer:     normalizer,
		extractor:      extractor,
		episodeBuilder: episodeBuilder,
		windowExpander: windowExpander,
		scorer:         scorer,
		msgRepo:        msgRepo,
		chatRepo:       chatRepo,
		userRepo:       userRepo,
		semanticRepo:   semanticRepo,
		personRepo:     personRepo,
		log:            log,
		batchSize:      defaultBatchSize,
		historyDelay:   historyDelay,
	}
}

// RunBackfill executes a full incremental backfill and returns when complete.
// Idempotent: re-running resumes from saved cursors.
func (e *Engine) RunBackfill(ctx context.Context) error {
	return e.gateway.Run(ctx, func(ctx context.Context) error {
		start := time.Now()
		e.log.Info("backfill started")

		selfInfo, err := e.gateway.Self(ctx)
		if err != nil {
			return fmt.Errorf("get self: %w", err)
		}
		if err := e.upsertSelf(ctx, selfInfo); err != nil {
			return fmt.Errorf("upsert self: %w", err)
		}
		e.log.Info("authenticated", "user_id", selfInfo.ID, "username", selfInfo.Username)

		dialogs, err := e.gateway.ListDialogs(ctx)
		if err != nil {
			return fmt.Errorf("list dialogs: %w", err)
		}

		// Score every dialog, persist relevance metadata, decide what to sync.
		type scored struct {
			dialog    port.DialogInfo
			relevance ChatRelevance
		}
		all := make([]scored, 0, len(dialogs))
		for _, d := range dialogs {
			all = append(all, scored{d, e.scorer.Score(d)})
		}

		// Upsert ALL chats (even excluded ones) so SQL inspection shows the full picture.
		// Then update relevance scores so the reason is always persisted.
		for _, s := range all {
			if err := e.chatRepo.Upsert(ctx, &entity.Chat{
				ID: s.dialog.ID, Type: s.dialog.Type,
				Title: s.dialog.Title, Username: s.dialog.Username,
			}); err != nil {
				e.log.Warn("upsert chat failed", "chat_id", s.dialog.ID, "error", err)
			}
			if err := e.chatRepo.UpdateRelevance(ctx,
				s.dialog.ID, s.relevance.Score,
				s.relevance.Reason, s.relevance.Surface,
			); err != nil {
				e.log.Warn("update relevance failed", "chat_id", s.dialog.ID, "error", err)
			}
		}

		// Log scoring summary grouped by surface.
		var toSync []scored
		surfaceCount := map[entity.PersonalitySurface]int{}
		for _, s := range all {
			surfaceCount[s.relevance.Surface]++
			if s.relevance.ShouldSync() {
				toSync = append(toSync, s)
			} else {
				e.log.Debug("dialog excluded by scorer",
					"chat_id", s.dialog.ID,
					"title", s.dialog.Title,
					"score", s.relevance.Score,
					"reason", s.relevance.Reason,
				)
			}
		}
		e.log.Info("dialog scoring complete",
			"total", len(all),
			"to_sync", len(toSync),
			"excluded", len(all)-len(toSync),
			"self_expression", surfaceCount[entity.SurfaceSelfExpression],
			"interpersonal", surfaceCount[entity.SurfaceInterpersonal],
			"social", surfaceCount[entity.SurfaceSocial],
			"tool_interaction", surfaceCount[entity.SurfaceToolInteraction],
			"passive_consumption", surfaceCount[entity.SurfacePassiveConsumption],
		)

		var synced, failed int
		for _, s := range toSync {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := e.syncDialogMessages(ctx, s.dialog); err != nil {
				e.log.Error("dialog sync failed",
					"chat_id", s.dialog.ID, "title", s.dialog.Title, "error", err)
				failed++
				continue
			}

			// Window computation: runs before episode building so that retroactive
			// Layer 2-3 rebuild populates data that the episode builder will segment.
			if needsWindowing(s.relevance.Surface) {
				if err := e.windowExpander.ComputeAndRebuild(ctx, s.dialog.ID); err != nil {
					e.log.Warn("window compute+rebuild failed",
						"chat_id", s.dialog.ID, "error", err)
					// Non-fatal: window flags can be recomputed on next sync pass.
				}
			}

			// Layer 4: segment new messages into episodes.
			if err := e.episodeBuilder.BuildForDialog(ctx, s.dialog.ID); err != nil {
				e.log.Warn("episode building failed",
					"chat_id", s.dialog.ID, "error", err)
				// Non-fatal: episodes can be rebuilt; don't abort the dialog.
			}

			synced++
			time.Sleep(defaultDialogDelay)
		}

		e.log.Info("backfill complete",
			"dialogs_synced", synced,
			"dialogs_failed", failed,
			"duration", time.Since(start).Round(time.Second),
		)
		return nil
	})
}

// upsertSelf persists the authenticated Telegram account.
func (e *Engine) upsertSelf(ctx context.Context, info *port.UserInfo) error {
	return e.userRepo.Upsert(ctx, &entity.User{
		ID: info.ID, Username: info.Username,
		FirstName: info.FirstName, LastName: info.LastName,
		Phone: info.Phone, IsSelf: true,
	})
}

// upsertChats persists all fetched dialogs as Chat entities.
// syncDialogMessages fetches and persists all three memory layers for one dialog.
func (e *Engine) syncDialogMessages(ctx context.Context, dialog port.DialogInfo) error {
	log := e.log.With("chat_id", dialog.ID, "title", dialog.Title)

	cursor, err := e.msgRepo.GetCursor(ctx, dialog.ID)
	if err != nil {
		return fmt.Errorf("get cursor: %w", err)
	}

	var (
		offsetID  int64
		maxIDSeen int64
		totalSaved int
	)

	log.Info("syncing dialog", "cursor", cursor.LastMessageID)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		page, err := e.gateway.GetHistory(ctx, port.HistoryRequest{
			Dialog: dialog, OffsetID: offsetID, Limit: e.batchSize,
		})
		if err != nil {
			return fmt.Errorf("get history offset=%d: %w", offsetID, err)
		}
		if len(page.Messages) == 0 {
			break
		}

		// Upsert all participants from this page BEFORE processing messages.
		// This prevents FK violations on messages.sender_id for group members,
		// private chat peers, and signed channel authors.
		e.upsertParticipants(ctx, page.Participants)

		reachedCursor := false
		for i := range page.Messages {
			incoming := &page.Messages[i]
			if incoming.TelegramID <= cursor.LastMessageID {
				reachedCursor = true
				break
			}
			if maxIDSeen == 0 {
				maxIDSeen = incoming.TelegramID
			}

			if err := e.ingestMessage(ctx, incoming); err != nil {
				// Log and continue — one bad message shouldn't abort the dialog.
				log.Error("ingest message failed",
					"telegram_id", incoming.TelegramID, "error", err)
				continue
			}
			totalSaved++
		}

		if reachedCursor || !page.HasMore {
			break
		}
		offsetID = page.MinID

		// Proactive throttle between page requests to reduce FLOOD_WAIT probability.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(e.historyDelay):
		}
	}

	if maxIDSeen > 0 {
		if err := e.msgRepo.SaveCursor(ctx, &entity.SyncCursor{
			ChatID: dialog.ID, LastMessageID: maxIDSeen,
		}); err != nil {
			log.Warn("save cursor failed", "error", err)
		}
	}

	log.Info("dialog synced", "messages_saved", totalSaved, "new_cursor", maxIDSeen)
	return nil
}

// upsertParticipants persists all users from a history page response.
// Called once per page, before messages, to satisfy the messages.sender_id FK.
// Individual failures are warnings only — the EnsureExists fallback in ingestMessage
// covers the rare cases that slip through (deleted accounts, partial responses).
func (e *Engine) upsertParticipants(ctx context.Context, participants []port.UserInfo) {
	for _, p := range participants {
		if p.ID == 0 {
			continue
		}
		if err := e.userRepo.Upsert(ctx, &entity.User{
			ID:        p.ID,
			Username:  p.Username,
			FirstName: p.FirstName,
			LastName:  p.LastName,
		}); err != nil {
			e.log.Warn("upsert participant failed", "user_id", p.ID, "error", err)
		}
	}
}

// ingestMessage writes one message across all three memory layers.
// The three operations are independent: semantic/personality failures are logged
// but don't block message persistence (raw layer is authoritative).
func (e *Engine) ingestMessage(ctx context.Context, incoming *port.IncomingMessage) error {
	// FK safety: sender must exist in users before message insert.
	// Participants are upserted in bulk before each page; this is a fallback
	// for deleted accounts and edge cases not included in the participants list.
	if incoming.SenderID != 0 {
		if err := e.userRepo.EnsureExists(ctx, incoming.SenderID); err != nil {
			e.log.Debug("ensure sender exists failed",
				"sender_id", incoming.SenderID, "error", err)
		}
	}

	// ── Layer 1: Raw ─────────────────────────────────────────────────────────
	msg := portToEntity(incoming)
	saved, err := e.msgRepo.Upsert(ctx, msg)
	if err != nil {
		return fmt.Errorf("upsert raw: %w", err)
	}

	// ── Layer 2: Semantic normalization ──────────────────────────────────────
	semDoc := e.normalizer.Normalize(saved)
	if semErr := e.semanticRepo.Upsert(ctx, semDoc); semErr != nil {
		e.log.Warn("semantic upsert failed",
			"msg_id", saved.ID, "error", semErr)
	}

	// ── Layer 3: Personality signals ──────────────────────────────────────────
	signals := e.extractor.Extract(saved)
	if len(signals) > 0 {
		if sigErr := e.personRepo.SaveSignals(ctx, signals); sigErr != nil {
			e.log.Warn("personality signals save failed",
				"msg_id", saved.ID, "error", sigErr)
		}
	}

	return nil
}

// needsWindowing reports whether a dialog surface uses participation-window
// filtering rather than full-sync. Social and PassiveConsumption surfaces
// contain messages from many participants; only outgoing-anchor neighbourhoods
// are relevant to personality/semantic pipelines.
func needsWindowing(surface entity.PersonalitySurface) bool {
	return surface == entity.SurfaceSocial || surface == entity.SurfacePassiveConsumption
}

// portToEntity converts an IncomingMessage port DTO to a domain entity.
// This mapping is the application layer's responsibility — neither infrastructure
// nor domain should know about the other's types.
func portToEntity(m *port.IncomingMessage) *entity.Message {
	msg := &entity.Message{
		TelegramID:    m.TelegramID,
		ChatID:        m.ChatID,
		SenderID:      m.SenderID,
		ReplyToID:     m.ReplyToID,
		Text:          m.Text,
		RawData:       m.RawData,
		SentAt:        m.SentAt,
		IsOutgoing:    m.IsOutgoing,
		MediaKind:     entity.MediaKind(m.MediaKind),
		IsForwarded:   m.IsForwarded,
		ForwardFromID: m.ForwardFromID,
		ForwardDate:   m.ForwardDate,
		EditDate:      m.EditDate,
	}

	if len(m.Entities) > 0 {
		msg.Entities = make([]entity.MessageEntity, len(m.Entities))
		for i, e := range m.Entities {
			msg.Entities[i] = entity.MessageEntity{
				Type: e.Type, Offset: e.Offset, Length: e.Length, URL: e.URL,
			}
		}
	}

	if len(m.Reactions) > 0 {
		msg.Reactions = make([]entity.ReactionCount, len(m.Reactions))
		for i, r := range m.Reactions {
			msg.Reactions[i] = entity.ReactionCount{Emoji: r.Emoji, Count: r.Count}
		}
	}

	if m.Sticker != nil {
		msg.StickerMeta = &entity.StickerInfo{
			SetName: m.Sticker.SetName, Emoticon: m.Sticker.Emoticon,
		}
	}

	return msg
}
