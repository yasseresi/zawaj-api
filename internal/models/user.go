package models

import "time"

// User is an account identified by a unique username, authenticated with a
// password. No email/phone by design (see ADR-003). Recovery is via a one-time code.
type User struct {
	Base
	Username     string  `gorm:"uniqueIndex;size:120;not null" json:"username"` // may be an email
	DisplayName  string  `gorm:"size:80" json:"display_name"`
	Email        *string `gorm:"size:120" json:"email,omitempty"` // optional (ADR-003; no email required)
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
