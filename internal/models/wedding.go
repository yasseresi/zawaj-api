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

// MembershipCeiling is the owner's last decision about a (former) member,
// kept after the membership is gone: MaxRole is the highest role invite links
// may grant ("editor"/"viewer"), or "none" once the owner removed them.
// Schema: migration 000010 (this model mirrors it for the test AutoMigrate).
type MembershipCeiling struct {
	WeddingID uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	MaxRole   string    `gorm:"size:10;not null"`
	SetAt     time.Time `gorm:"not null"`
}

// Membership links a user to a wedding with a role. Unique per (wedding,user).
type Membership struct {
	Base
	WeddingID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_wedding_user" json:"wedding_id"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_membership_wedding_user;index" json:"user_id"`
	Role      Role      `gorm:"size:10;not null" json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}
