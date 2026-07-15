package sync

import (
	"testing"

	"github.com/digital-personality/internal/application/port"
	"github.com/digital-personality/internal/domain/entity"
)

func TestChatRelevanceScorer(t *testing.T) {
	scorer := NewChatRelevanceScorer()

	tests := []struct {
		name        string
		dialog      port.DialogInfo
		wantScore   float32
		wantSurface entity.PersonalitySurface
		wantSync    bool
	}{
		{
			name:        "saved messages is top self-expression",
			dialog:      port.DialogInfo{Type: entity.ChatTypeSavedMessages},
			wantScore:   1.00,
			wantSurface: entity.SurfaceSelfExpression,
			wantSync:    true,
		},
		{
			name:        "private 1:1 has highest interpersonal signal",
			dialog:      port.DialogInfo{Type: entity.ChatTypePrivate},
			wantScore:   0.85,
			wantSurface: entity.SurfaceInterpersonal,
			wantSync:    true,
		},
		{
			name:        "bot dialog is tool interaction, still synced",
			dialog:      port.DialogInfo{Type: entity.ChatTypePrivate, IsBot: true},
			wantScore:   0.50,
			wantSurface: entity.SurfaceToolInteraction,
			wantSync:    true,
		},
		{
			name:        "own channel is high-value self-expression",
			dialog:      port.DialogInfo{Type: entity.ChatTypeChannel, IsCreator: true},
			wantScore:   0.90,
			wantSurface: entity.SurfaceSelfExpression,
			wantSync:    true,
		},
		{
			name:        "subscribed broadcast channel is excluded",
			dialog:      port.DialogInfo{Type: entity.ChatTypeChannel},
			wantScore:   0.10,
			wantSurface: entity.SurfacePassiveConsumption,
			wantSync:    false,
		},
		{
			name:        "group member",
			dialog:      port.DialogInfo{Type: entity.ChatTypeGroup},
			wantScore:   0.60,
			wantSurface: entity.SurfaceSocial,
			wantSync:    true,
		},
		{
			name:        "group creator outranks member",
			dialog:      port.DialogInfo{Type: entity.ChatTypeGroup, IsCreator: true},
			wantScore:   0.80,
			wantSurface: entity.SurfaceSocial,
			wantSync:    true,
		},
		{
			name:        "supergroup member",
			dialog:      port.DialogInfo{Type: entity.ChatTypeSupergroup},
			wantScore:   0.55,
			wantSurface: entity.SurfaceSocial,
			wantSync:    true,
		},
		{
			name:        "unknown type excluded by default",
			dialog:      port.DialogInfo{Type: entity.ChatType("carrier_pigeon")},
			wantScore:   0.00,
			wantSurface: entity.SurfacePassiveConsumption,
			wantSync:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scorer.Score(tt.dialog)
			if got.Score != tt.wantScore {
				t.Errorf("Score = %v, want %v", got.Score, tt.wantScore)
			}
			if got.Surface != tt.wantSurface {
				t.Errorf("Surface = %q, want %q", got.Surface, tt.wantSurface)
			}
			if got.ShouldSync() != tt.wantSync {
				t.Errorf("ShouldSync = %v, want %v (threshold %v)", got.ShouldSync(), tt.wantSync, SyncThreshold)
			}
			if got.Reason == "" {
				t.Error("Reason must never be empty — inspectability requirement")
			}
		})
	}
}
