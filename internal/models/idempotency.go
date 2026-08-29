package models

import "github.com/google/uuid"

// IdempotencyKey stores the outcome of a mutating request so a retry carrying the
// same Idempotency-Key returns the original response instead of executing twice.
// Scoped per user; the request hash guards against reusing a key for a different
// request.
type IdempotencyKey struct {
	Base
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_idem_user_key,priority:2" json:"user_id"`
	Key         string    `gorm:"size:200;not null;uniqueIndex:idx_idem_user_key,priority:1" json:"key"`
	RequestHash string    `gorm:"size:64;not null" json:"request_hash"`
	StatusCode  int       `gorm:"not null" json:"status_code"`
	Response    []byte    `gorm:"type:bytea" json:"-"`
}
