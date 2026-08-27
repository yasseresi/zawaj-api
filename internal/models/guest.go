package models

import "github.com/google/uuid"

// RSVPStatus is a guest's response state. Default pending (UI "مدعو/invited").
type RSVPStatus string

const (
	StatusPending   RSVPStatus = "pending"
	StatusConfirmed RSVPStatus = "confirmed"
	StatusDeclined  RSVPStatus = "declined"
)

// Valid reports whether s is a known status.
func (s RSVPStatus) Valid() bool {
	switch s {
	case StatusPending, StatusConfirmed, StatusDeclined:
		return true
	}
	return false
}

// Guest is an invitee on a wedding's list. One row = one invite covering the
// guest plus `Companions` accompanying people (total people = 1 + Companions).
type Guest struct {
	Base
	WeddingID    uuid.UUID  `gorm:"type:uuid;not null;index:idx_guest_wedding;index:idx_guest_wedding_status,priority:1" json:"wedding_id"`
	AddedBy      uuid.UUID  `gorm:"type:uuid;not null" json:"added_by"`
	FullName     string     `gorm:"size:120;not null" json:"full_name"`
	Contact      string     `gorm:"size:120" json:"contact,omitempty"`      // phone OR email
	Relationship string     `gorm:"size:80" json:"relationship,omitempty"`  // "صديقة العروس"
	Status       RSVPStatus `gorm:"size:12;not null;default:pending;index:idx_guest_wedding_status,priority:2" json:"status"`
	Companions   int        `gorm:"not null;default:0" json:"companions"`
	TableLabel   string     `gorm:"size:40" json:"table_label,omitempty"` // "طاولة رقم 4"
	Meal         string     `gorm:"size:40" json:"meal,omitempty"`        // "نباتي"
}

// GuestNote is one authored note on a guest (screen 7 shows a thread of them).
type GuestNote struct {
	Base
	GuestID  uuid.UUID `gorm:"type:uuid;not null;index" json:"guest_id"`
	AuthorID uuid.UUID `gorm:"type:uuid;not null" json:"author_id"`
	Body     string    `gorm:"type:text;not null" json:"body"`
}
