package models

import "time"

// User is an account identified by a unique username, authenticated with a
// password. Recovery is via a one-time code (ADR-003). Phone is an optional,
// unverified contact field, never an auth factor.
type User struct {
	Base
	Username     string  `gorm:"uniqueIndex;size:120;not null" json:"username"` // may be an email
	DisplayName  string  `gorm:"size:80" json:"display_name"`
	Email        *string `gorm:"size:120" json:"-"`              // legacy; no longer exposed (phone replaces it)
	Phone        *string `gorm:"size:10" json:"phone,omitempty"` // Algerian national format, unverified
	PasswordHash string  `gorm:"not null" json:"-"`
	RecoveryHash string  `gorm:"not null" json:"-"`

	// Preferences (settings screen).
	DarkMode   bool `gorm:"not null;default:false" json:"dark_mode"`
	NotifPush  bool `gorm:"not null;default:true" json:"notif_push"`
	NotifEmail bool `gorm:"not null;default:true" json:"notif_email"`
	NotifRSVP  bool `gorm:"not null;default:true" json:"notif_rsvp"`
	IsPremium  bool `gorm:"not null;default:false" json:"is_premium"` // monetization [later]

	// Brute-force protection (enforced in P6; fields live here from the start).
	FailedAttempts int        `gorm:"not null;default:0" json:"-"`
	LockedUntil    *time.Time `json:"-"`
}
