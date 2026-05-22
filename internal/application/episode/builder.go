// Package episode contains the EpisodeBuilder application use case.
// It depends only on ports and repository interfaces — no infrastructure types.
package episode

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
	"github.com/digital-personality/internal/domain/repository"
)

const (
	// maxBatchMessages limits how many messages are loaded at once per dialog.
	// Large chats are processed in rolling batches to cap memory usage.
	maxBatchMessages = 1000

	// minEpisodeSemanticTokens: episodes with fewer tokens skip embedding.
	minEpisodeSemanticTokens = 5
)

// Builder is the application use case for building episodic memories.
//
// Lifecycle (per dialog):
//  1. Load messages not yet assigned to any episode.
//  2. Run segmenter → get EpisodeSegments.
//  3. For each segment: create Episode, link messages, build semantic doc.
//
// The three steps are transactionally independent; a failure in step 3 (semantic)
// does not roll back episode creation — the semantic doc can be rebuilt.
type Builder struct {
	episodeRepo repository.EpisodeRepository
	segmenter   port.EpisodeSegmenter
	normalizer  port.SemanticNormalizer
	log         *slog.Logger
}

// NewBuilder constructs an EpisodeBuilder. All parameters are required.
func NewBuilder(
	episodeRepo repository.EpisodeRepository,
	segmenter port.EpisodeSegmenter,
	normalizer port.SemanticNormalizer,
	log *slog.Logger,
) *Builder {
	return &Builder{
		episodeRepo: episodeRepo,
		segmenter:   segmenter,
		normalizer:  normalizer,
		log:         log,
	}
}

// BuildForDialog segments all unprocessed messages in chatID into episodes.
// Idempotent: messages already assigned to episodes are skipped.
func (b *Builder) BuildForDialog(ctx context.Context, chatID int64) error {
	log := b.log.With("chat_id", chatID)

	messages, err := b.episodeRepo.ListUnepisodedMessages(ctx, chatID, maxBatchMessages)
	if err != nil {
		return fmt.Errorf("list unepisoded messages chat=%d: %w", chatID, err)
	}
	if len(messages) == 0 {
		log.Debug("no unepisoded messages")
		return nil
	}

	log.Info("building episodes", "unepisoded_messages", len(messages))

	segments := b.segmenter.Segment(messages)

	var (
		episodesCreated int
		skipped         int
	)

	for _, seg := range segments {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if len(seg.Messages) == 0 {
			continue
		}

		if err := b.persistSegment(ctx, chatID, seg); err != nil {
			log.Error("persist segment failed",
				"messages", len(seg.Messages),
				"type", seg.Type,
				"error", err,
			)
			skipped++
			continue
		}
		episodesCreated++
	}

	log.Info("episode building complete",
		"episodes_created", episodesCreated,
		"segments_skipped", skipped,
		"total_segments", len(segments),
	)
	return nil
}

// persistSegment writes one EpisodeSegment as an Episode + semantic doc to the DB.
func (b *Builder) persistSegment(ctx context.Context, chatID int64, seg port.EpisodeSegment) error {
	first := seg.Messages[0]
	last := seg.Messages[len(seg.Messages)-1]

	// Collect unique participant IDs.
	participantSet := make(map[int64]struct{}, 4)
	for _, m := range seg.Messages {
		if m.SenderID != 0 {
			participantSet[m.SenderID] = struct{}{}
		}
	}
	participants := make([]int64, 0, len(participantSet))
	for id := range participantSet {
		participants = append(participants, id)
	}

	episode := &entity.Episode{
		ChatID:         chatID,
		StartedAt:      first.SentAt,
		EndedAt:        last.SentAt,
		Type:           seg.Type,
		MessageCount:   len(seg.Messages),
		ParticipantIDs: participants,
		SegmentedBy:    seg.Method,
		Confidence:     seg.Confidence,
	}

	episodeID, err := b.episodeRepo.Create(ctx, episode)
	if err != nil {
		return fmt.Errorf("create episode: %w", err)
	}

	// Link messages to episode.
	links := make([]entity.EpisodeMessage, len(seg.Messages))
	for i, m := range seg.Messages {
		links[i] = entity.EpisodeMessage{
			EpisodeID: episodeID,
			MessageID: m.ID,
			Position:  i,
		}
	}
	if err := b.episodeRepo.LinkMessages(ctx, links); err != nil {
		return fmt.Errorf("link messages to episode=%d: %w", episodeID, err)
	}

	// Build and store semantic document.
	semDoc := b.buildSemanticDoc(episodeID, seg.Messages)
	if semErr := b.episodeRepo.UpsertSemantic(ctx, semDoc); semErr != nil {
		// Non-fatal: semantic can be rebuilt; don't abort episode creation.
		b.log.Warn("episode semantic upsert failed",
			"episode_id", episodeID, "error", semErr)
	}

	return nil
}

// buildSemanticDoc composes the direction-annotated semantic text for an episode.
//
// Format:
//
//	→ normalized outgoing message text
//	← normalized incoming message text
//
// Only messages with non-empty, non-skip normalized text contribute.
// Stickers and voice messages are represented as brief placeholders to preserve
// the conversational rhythm in the semantic text.
func (b *Builder) buildSemanticDoc(episodeID int64, messages []*entity.Message) *entity.EpisodeSemanticDoc {
	var parts []string
	for _, m := range messages {
		line := b.formatMessageLine(m)
		if line != "" {
			parts = append(parts, line)
		}
	}

	text := strings.Join(parts, "\n")
	tokenCount := len(strings.Fields(text))
	skip := utf8.RuneCountInString(strings.TrimSpace(text)) < 10 || tokenCount < minEpisodeSemanticTokens

	return &entity.EpisodeSemanticDoc{
		EpisodeID:     episodeID,
		SemanticText:  text,
		TokenCount:    tokenCount,
		SkipEmbedding: skip,
		CreatedAt:     time.Now().UTC(),
	}
}

// formatMessageLine normalizes a single message into a direction-annotated line.
func (b *Builder) formatMessageLine(m *entity.Message) string {
	direction := "←"
	if m.IsOutgoing {
		direction = "→"
	}

	// For stickers: use emoticon as semantic placeholder.
	if m.MediaKind == entity.MediaKindSticker {
		if m.StickerMeta != nil && m.StickerMeta.Emoticon != "" {
			return direction + " [" + m.StickerMeta.Emoticon + "]"
		}
		return direction + " [sticker]"
	}

	// For voice/round: mark presence without content.
	if m.MediaKind == entity.MediaKindVoice || m.MediaKind == entity.MediaKindRound {
		return direction + " [voice]"
	}

	// For text messages: normalize (strip emoji, lowercase).
	semDoc := b.normalizer.Normalize(m)
	normalized := strings.TrimSpace(semDoc.NormalizedText)
	if normalized == "" {
		return ""
	}
	return direction + " " + normalized
}
