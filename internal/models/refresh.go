package models

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken is the server-side record of an issued refresh token, keyed by the
// token's jti (Base.ID). Its presence + RevokedAt make refresh tokens revocable
// and rotatable: on each refresh the old row is revoked and a new one created, so
// a leaked-then-rotated token is detected (reuse) and the whole family is killed.
type RefreshToken struct {
	Base
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	ExpiresAt time.Time  `gorm:"not null" json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}
