package models

import (
	"time"

	"github.com/google/uuid"
)

// DevicePlatform identifies the push transport target for a device token.
type DevicePlatform string

const (
	PlatformAndroid DevicePlatform = "android"
	PlatformIOS     DevicePlatform = "ios"
	PlatformWeb     DevicePlatform = "web"
)

// Valid reports whether p is a known platform.
func (p DevicePlatform) Valid() bool {
	switch p {
	case PlatformAndroid, PlatformIOS, PlatformWeb:
		return true
	default:
		return false
	}
}

// DeviceToken is one FCM registration token for one user's device. A user may
// have many (phone, tablet, web). Tokens are globally unique — when a device
// re-registers, the row is re-pointed to the current owner (upsert on token).
type DeviceToken struct {
	Base
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"user_id"`
	Token      string         `gorm:"uniqueIndex;size:512;not null" json:"token"`
	Platform   DevicePlatform `gorm:"size:10;not null" json:"platform"`
	LastSeenAt time.Time      `json:"last_seen_at"`
}
