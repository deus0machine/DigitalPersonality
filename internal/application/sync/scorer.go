package sync

import (
	"fmt"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

// SyncThreshold is the minimum relevance score required to sync a dialog.
// Dialogs below this score are logged but skipped — no messages fetched.
const SyncThreshold = float32(0.35)

// ChatRelevance is the output of ChatRelevanceScorer for one dialog.
// Contains the score, a human-readable explanation, and the personality surface label.
type ChatRelevance struct {
	Score   float32
	Reason  string
	Surface entity.PersonalitySurface
}

// ShouldSync returns true when this dialog exceeds the sync threshold.
func (r ChatRelevance) ShouldSync() bool {
	return r.Score >= SyncThreshold
}

// ChatRelevanceScorer assigns a 0.0–1.0 relevance score to each dialog based on
// Telegram ownership signals and chat type. Pure function — no I/O, no side effects.
//
// Scoring philosophy:
//   - Own channels (creator/admin) are high-value: long-form self-expression, public persona.
//   - Private 1:1 conversations carry the richest interpersonal personality signal.
//   - Groups/supergroups reflect social dynamics and peer communication style.
//   - Bot dialogs reveal tool usage and automation habits (medium value).
//   - Subscribed broadcast channels are passive consumption: no user authorship, excluded.
type ChatRelevanceScorer struct{}

func NewChatRelevanceScorer() *ChatRelevanceScorer {
	return &ChatRelevanceScorer{}
}

// Score returns the relevance assessment for a single dialog.
func (s *ChatRelevanceScorer) Score(d port.DialogInfo) ChatRelevance {
	switch d.Type {

	case entity.ChatTypeSavedMessages:
		return ChatRelevance{
			Score:   1.00,
			Reason:  "saved messages: primary self-expression surface — notes, links, reminders to self",
			Surface: entity.SurfaceSelfExpression,
		}

	case entity.ChatTypePrivate:
		if d.IsBot {
			return ChatRelevance{
				Score:   0.50,
				Reason:  "bot dialog: reveals tool usage, task automation, and service interaction patterns",
				Surface: entity.SurfaceToolInteraction,
			}
		}
		return ChatRelevance{
			Score:   0.85,
			Reason:  "private 1:1 conversation: highest interpersonal signal density",
			Surface: entity.SurfaceInterpersonal,
		}

	case entity.ChatTypeGroup:
		switch {
		case d.IsCreator:
			return ChatRelevance{
				Score:   0.80,
				Reason:  "group created by user: social leadership style and peer dynamics",
				Surface: entity.SurfaceSocial,
			}
		case d.IsAdmin:
			return ChatRelevance{
				Score:   0.70,
				Reason:  "group managed by user: moderation voice and communication authority",
				Surface: entity.SurfaceSocial,
			}
		default:
			return ChatRelevance{
				Score:   0.60,
				Reason:  "group participation: peer communication patterns",
				Surface: entity.SurfaceSocial,
			}
		}

	case entity.ChatTypeSupergroup:
		switch {
		case d.IsCreator:
			return ChatRelevance{
				Score:   0.75,
				Reason:  "supergroup created by user: community leadership and public communication style",
				Surface: entity.SurfaceSocial,
			}
		case d.IsAdmin:
			return ChatRelevance{
				Score:   0.65,
				Reason:  "supergroup admin: moderation style and public-facing communication",
				Surface: entity.SurfaceSocial,
			}
		default:
			return ChatRelevance{
				Score:   0.55,
				Reason:  "supergroup member: community participation patterns",
				Surface: entity.SurfaceSocial,
			}
		}

	case entity.ChatTypeChannel:
		switch {
		case d.IsCreator:
			return ChatRelevance{
				Score:   0.90,
				Reason:  "own channel (creator): long-form self-expression, worldview, public persona — high personality signal",
				Surface: entity.SurfaceSelfExpression,
			}
		case d.IsAdmin:
			return ChatRelevance{
				Score:   0.75,
				Reason:  "administered channel: editorial voice and curated content — reflects worldview",
				Surface: entity.SurfaceSelfExpression,
			}
		default:
			return ChatRelevance{
				Score:   0.10,
				Reason:  "subscribed broadcast channel: read-only passive consumption — no user authorship",
				Surface: entity.SurfacePassiveConsumption,
			}
		}

	default:
		return ChatRelevance{
			Score:   0.00,
			Reason:  fmt.Sprintf("unknown dialog type %q: excluded by default", d.Type),
			Surface: entity.SurfacePassiveConsumption,
		}
	}
}
