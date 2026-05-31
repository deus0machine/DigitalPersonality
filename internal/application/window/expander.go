// Package window contains the WindowExpander application use case.
// It computes participation windows and retroactively processes Layers 2-3
// for messages that enter the window after a sync pass.
package window

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/repository"
)

const rebuildBatchSize = 100

// Expander computes participation-centered memory windows for group/channel
// dialogs and retroactively runs Layers 2-3 on newly windowed messages.
//
// Lifecycle (per dialog):
//  1. ComputeParticipationWindows — marks in_memory_window per SQL rules.
//  2. rebuildLayers — batched: ListPendingRebuild → normalize → semantic → personality signals.
type Expander struct {
	windowRepo   repository.WindowRepository
	semanticRepo repository.SemanticRepository
	personRepo   repository.PersonalityRepository
	normalizer   port.SemanticNormalizer
	extractor    port.PersonalityExtractor
	before       int
	after        int
	log          *slog.Logger
}

// NewExpander constructs a WindowExpander. All parameters are required.
func NewExpander(
	windowRepo repository.WindowRepository,
	semanticRepo repository.SemanticRepository,
	personRepo repository.PersonalityRepository,
	normalizer port.SemanticNormalizer,
	extractor port.PersonalityExtractor,
	before, after int,
	log *slog.Logger,
) *Expander {
	return &Expander{
		windowRepo:   windowRepo,
		semanticRepo: semanticRepo,
		personRepo:   personRepo,
		normalizer:   normalizer,
		extractor:    extractor,
		before:       before,
		after:        after,
		log:          log,
	}
}

// ComputeAndRebuild runs window computation then retroactive Layer 2-3 rebuild
// for the given chat. Idempotent — safe to call after every sync pass.
func (e *Expander) ComputeAndRebuild(ctx context.Context, chatID int64) error {
	log := e.log.With("chat_id", chatID)

	if err := e.windowRepo.ComputeParticipationWindows(ctx, chatID, e.before, e.after); err != nil {
		return fmt.Errorf("compute windows chat=%d: %w", chatID, err)
	}
	log.Debug("participation windows computed", "window_before", e.before, "window_after", e.after)

	rebuilt, err := e.rebuildLayers(ctx, chatID)
	if err != nil {
		return fmt.Errorf("rebuild layers chat=%d: %w", chatID, err)
	}
	if rebuilt > 0 {
		log.Info("retroactive layer rebuild complete", "messages_rebuilt", rebuilt)
	}
	return nil
}

// rebuildLayers processes all newly windowed messages that lack semantic docs.
// Runs Layer 2 (semantic) and Layer 3 (personality signals) only —
// Layer 1 already exists; Layer 4 (episodes) runs via episode.Builder after this.
// Returns the total number of messages processed.
func (e *Expander) rebuildLayers(ctx context.Context, chatID int64) (int, error) {
	var total int
	for {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}

		msgs, err := e.windowRepo.ListPendingRebuild(ctx, chatID, rebuildBatchSize)
		if err != nil {
			return total, fmt.Errorf("list pending rebuild: %w", err)
		}
		if len(msgs) == 0 {
			break
		}

		for _, msg := range msgs {
			semDoc := e.normalizer.Normalize(msg)
			if semErr := e.semanticRepo.Upsert(ctx, semDoc); semErr != nil {
				e.log.Warn("semantic upsert failed during rebuild",
					"chat_id", chatID, "msg_id", msg.ID, "error", semErr)
			}

			signals := e.extractor.Extract(msg)
			if len(signals) > 0 {
				if sigErr := e.personRepo.SaveSignals(ctx, signals); sigErr != nil {
					e.log.Warn("personality signals save failed during rebuild",
						"chat_id", chatID, "msg_id", msg.ID, "error", sigErr)
				}
			}
		}

		total += len(msgs)
		if len(msgs) < rebuildBatchSize {
			break
		}
	}
	return total, nil
}
