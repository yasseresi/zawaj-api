package repository

import (
	"context"
	"errors"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RefreshTokenRepo is data access for server-side refresh-token records.
type RefreshTokenRepo struct{ db *gorm.DB }

// NewRefreshTokenRepo builds the repo.
func NewRefreshTokenRepo(db *gorm.DB) *RefreshTokenRepo { return &RefreshTokenRepo{db: db} }

// Store persists a newly issued refresh token keyed by its jti.
func (r *RefreshTokenRepo) Store(ctx context.Context, jti, userID uuid.UUID, expiresAt time.Time) error {
	row := models.RefreshToken{
		Base:      models.Base{ID: jti},
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
	return r.db.WithContext(ctx).Create(&row).Error
}

// ByID loads a refresh-token record by jti, or ErrNotFound.
func (r *RefreshTokenRepo) ByID(ctx context.Context, jti uuid.UUID) (*models.RefreshToken, error) {
	var row models.RefreshToken
	err := r.db.WithContext(ctx).First(&row, "id = ?", jti).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Revoke marks a single token revoked (idempotent — no error if already revoked
// or absent).
func (r *RefreshTokenRepo) Revoke(ctx context.Context, jti uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", now).Error
}

// RevokeAllForUser revokes every active refresh token of a user (logout-all;
// also used on password change and on refresh-token reuse detection).
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}
