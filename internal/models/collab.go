package models

import (
	"time"

	"github.com/google/uuid"
)

// InviteLink is a role-scoped share link / QR target for a wedding. The owner
// (or an editor) generates one per role; anyone who opens the link and accepts
// joins the wedding with that link's role. Token appears in the URL
// (e.g. zawaj.app/i/{token}) and is encoded into the QR code client-side.
type InviteLink struct {
	Base
	WeddingID uuid.UUID  `gorm:"type:uuid;not null;index" json:"wedding_id"`
	CreatedBy uuid.UUID  `gorm:"type:uuid;not null" json:"created_by"`
	Token     string     `gorm:"uniqueIndex;size:24;not null" json:"token"`
	Role      Role       `gorm:"size:10;not null" json:"role"` // editor | viewer (never owner)
	Revoked   bool       `gorm:"not null;default:false" json:"revoked"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// InviteStatus is the lifecycle of a targeted collaborator invite (screen 10).
type InviteStatus string

const (
	InvitePending  InviteStatus = "pending"
	InviteAccepted InviteStatus = "accepted"
	InviteDeclined InviteStatus = "declined"
	// InviteRevoked: withdrawn because the owner removed the invitee from the
	// wedding; a fresh invite is needed to re-admit them.
	InviteRevoked InviteStatus = "revoked"
)

// WeddingInvite is a targeted invitation for a specific registered user to join
// a wedding with a role. Distinct from InviteLink (shareable token): here the
// recipient sees a "pending invites" list and accepts or declines.
type WeddingInvite struct {
	Base
	WeddingID uuid.UUID    `gorm:"type:uuid;not null" json:"wedding_id"`
	InviterID uuid.UUID    `gorm:"type:uuid;not null" json:"inviter_id"`
	InviteeID uuid.UUID    `gorm:"type:uuid;not null;index" json:"invitee_id"`
	Role      Role         `gorm:"size:10;not null" json:"role"`   // editor | viewer (never owner)
	Status    InviteStatus `gorm:"size:10;not null" json:"status"` // pending | accepted | declined | revoked
}

// JoinRequestStatus is the lifecycle of a request to join through an invite link.
type JoinRequestStatus string

const (
	JoinPending  JoinRequestStatus = "pending"
	JoinApproved JoinRequestStatus = "approved"
	JoinDeclined JoinRequestStatus = "declined"
	// JoinClosed: no longer needs a decision (the user joined another way,
	// was removed, or the link was revoked). Unlike declined, it doesn't
	// start the re-request cooldown.
	JoinClosed JoinRequestStatus = "closed"
)

// JoinRequest is created when someone accepts an invite link: they become a
// member only once the owner approves (schema: migration 000012). Role is what
// the link grants, already capped by any earlier owner decision.
type JoinRequest struct {
	Base
	WeddingID uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_join_requests_pending,where:status = 'pending';index:idx_join_requests_wedding_status,priority:1" json:"wedding_id"`
	UserID    uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:idx_join_requests_pending,where:status = 'pending';index:idx_join_requests_user" json:"user_id"`
	LinkID    uuid.UUID         `gorm:"type:uuid;not null" json:"link_id"`
	Role      Role              `gorm:"size:10;not null" json:"role"`
	Status    JoinRequestStatus `gorm:"size:10;not null;default:pending;index:idx_join_requests_wedding_status,priority:2" json:"status"`
	DecidedAt *time.Time        `json:"decided_at,omitempty"`
	DecidedBy *uuid.UUID        `gorm:"type:uuid" json:"decided_by,omitempty"`
}

// ActivityAction enumerates entries in the activity feed / per-guest history.
type ActivityAction string

const (
	ActGuestAdded        ActivityAction = "guest_added"
	ActGuestUpdated      ActivityAction = "guest_updated"
	ActGuestStatusChange ActivityAction = "guest_status_changed"
	ActNoteAdded         ActivityAction = "note_added"
	ActCollaboratorJoin  ActivityAction = "collaborator_joined"
	ActListExported      ActivityAction = "list_exported"
)

// ActivityLog powers the wedding activity feed (screen 9) and per-guest history
// (screen 7). GuestID is null for wedding-level events.
type ActivityLog struct {
	Base
	WeddingID uuid.UUID      `gorm:"type:uuid;not null;index:idx_activity_wedding,priority:1" json:"wedding_id"`
	ActorID   uuid.UUID      `gorm:"type:uuid;not null" json:"actor_id"`
	GuestID   *uuid.UUID     `gorm:"type:uuid;index:idx_activity_guest,priority:1" json:"guest_id,omitempty"`
	Action    ActivityAction `gorm:"size:30;not null" json:"action"`
	Meta      JSON           `gorm:"type:jsonb" json:"meta,omitempty"`
}

// Notification is a personal update in a user's feed (in-app updates). [P5b]
type Notification struct {
	Base
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_notif_user_read,priority:1" json:"user_id"`
	WeddingID *uuid.UUID `gorm:"type:uuid" json:"wedding_id,omitempty"`
	Type      string     `gorm:"size:40;not null" json:"type"`
	Title     string     `gorm:"size:160;not null" json:"title"`
	Body      string     `gorm:"type:text" json:"body,omitempty"`
	Data      JSON       `gorm:"type:jsonb" json:"data,omitempty"`
	ReadAt    *time.Time `gorm:"index:idx_notif_user_read,priority:2" json:"read_at,omitempty"`
}
