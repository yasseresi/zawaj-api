package models

import "time"

// User is an account identified by a unique username, authenticated with a PIN.
// No email/phone by design (see ADR-003). Recovery is via a one-time code.
type User struct {
	Base
	Username     string `gorm:"uniqueIndex;size:30;not null" json:"username"`
	DisplayName  string `gorm:"size:80" json:"display_name"`
	PINHash      string `gorm:"not null" json:"-"`
	RecoveryHash string `gorm:"not null" json:"-"`

	// Brute-force protection (enforced in P6; fields live here from the start).
	FailedAttempts int        `gorm:"not null;default:0" json:"-"`
	LockedUntil    *time.Time `json:"-"`
}
