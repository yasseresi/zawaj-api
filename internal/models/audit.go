package models

import "github.com/google/uuid"

// AuditLog is the security/compliance trail: authentication events, privilege
// changes, and destructive actions. It is distinct from ActivityLog (the
// user-facing per-wedding feed) — audit entries are for operators/compliance and
// are not surfaced in the app. ActorID is null for actorless events.
type AuditLog struct {
	Base
	ActorID    *uuid.UUID `gorm:"type:uuid;index" json:"actor_id,omitempty"`
	Action     string     `gorm:"size:60;not null;index" json:"action"`
	TargetType string     `gorm:"size:40" json:"target_type,omitempty"`
	TargetID   *uuid.UUID `gorm:"type:uuid" json:"target_id,omitempty"`
	IP         string     `gorm:"size:64" json:"ip,omitempty"`
	Meta       JSON       `gorm:"type:jsonb" json:"meta,omitempty"`
}
