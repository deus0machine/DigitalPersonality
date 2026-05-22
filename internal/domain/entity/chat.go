package entity

import "time"

type ChatType string

const (
	ChatTypePrivate       ChatType = "private"
	ChatTypeGroup         ChatType = "group"
	ChatTypeSupergroup    ChatType = "supergroup"
	ChatTypeChannel       ChatType = "channel"
	ChatTypeSavedMessages ChatType = "saved_messages" // self-dialog: notes, links, reminders
)

// PersonalitySurface classifies what kind of personality signal a chat carries.
// Determines ingestion priority and downstream use in the personality engine.
type PersonalitySurface string

const (
	// SurfaceSelfExpression: own channels, Saved Messages — long-form thoughts, public persona.
	SurfaceSelfExpression PersonalitySurface = "self_expression"

	// SurfaceInterpersonal: 1:1 private conversations — richest raw personality signal.
	SurfaceInterpersonal PersonalitySurface = "interpersonal"

	// SurfaceSocial: groups and supergroups — peer communication and social dynamics.
	SurfaceSocial PersonalitySurface = "social"

	// SurfaceToolInteraction: bot dialogs — reveals task automation and tool usage patterns.
	SurfaceToolInteraction PersonalitySurface = "tool_interaction"

	// SurfacePassiveConsumption: subscribed broadcast channels — no user authorship, zero personality signal.
	SurfacePassiveConsumption PersonalitySurface = "passive_consumption"
)

type Chat struct {
	ID        int64
	Type      ChatType
	Title     string
	Username  string

	// Relevance metadata — scored during sync, persisted for inspectability.
	RelevanceScore    float32
	RelevanceReason   string
	PersonalitySurface PersonalitySurface

	CreatedAt time.Time
	UpdatedAt time.Time
}
