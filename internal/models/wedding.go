package models

import (
	"time"

	"github.com/google/uuid"
)

// Role is a member's permission level on a wedding. owner > editor > viewer.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// Rank orders roles for RequireRole comparisons (higher = more privilege).
func (r Role) Rank() int {
	switch r {
	case RoleOwner:
		return 3
	case RoleEditor:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r.Rank() > 0 }

// Wedding is a guest list for one event, owned by a user and shared with family
// via role-scoped InviteLinks (not a single share code).
type Wedding struct {
	Base
	Name        string     `gorm:"size:120;not null" json:"name"`
	EventDate   *time.Time `gorm:"type:date" json:"event_date,omitempty"`
	Description string     `gorm:"type:text" json:"description,omitempty"`
	OwnerID     uuid.UUID  `gorm:"type:uuid;index;not null" json:"owner_id"`
}

// Membership links a user to a wedding with a role. Unique per (wedding,user).
type Membership struct {
	Base
	WeddingID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_wedding_user" json:"wedding_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_wedding_user;index" json:"user_id"`
	Role      Role      `gorm:"size:10;not null" json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
	// ViaLinkID is the invite link they joined with (nil for owners and
	// username invites), kept for history (migration 000011).
	ViaLinkID *uuid.UUID `gorm:"type:uuid" json:"-"`
}
