package repository

import (
	"context"

	"github.com/digital-personality/internal/domain/entity"
)

// WindowRepository manages participation-centered memory window computation.
// Window computation determines which messages from group/channel dialogs
// are included in semantic, personality, and episodic memory layers.
//
// Invariants preserved:
//   - Layer 1 (raw messages) is never modified — in_memory_window is additive metadata.
//   - Window computation is idempotent — safe to run on every sync pass.
//   - Full-sync surfaces (interpersonal, self_expression, tool_interaction)
//     never have in_memory_window set to FALSE by this repository.
type WindowRepository interface {
	// ComputeParticipationWindows marks in_memory_window for all non-deleted
	// messages in chatID based on proximity to outgoing anchors:
	//   - outgoing messages and their ±windowBefore/After neighbours → TRUE
	//   - direct reply targets of outgoing messages → TRUE
	//   - all other messages → FALSE
	// Runs atomically in a single transaction.
	ComputeParticipationWindows(ctx context.Context, chatID int64, windowBefore, windowAfter int) error

	// ListPendingRebuild returns messages with in_memory_window = TRUE
	// that have no corresponding entry in message_semantic (Layer 2 not processed).
	// Used by WindowExpander to retroactively run Layers 2-3 on newly windowed messages.
	// Results are ordered by sent_at ASC for chronological processing.
	ListPendingRebuild(ctx context.Context, chatID int64, limit int) ([]*entity.Message, error)
}
