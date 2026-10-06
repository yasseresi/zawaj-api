package repository

import (
	"context"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeviceTokenRepo is data access for FCM push registrations.
type DeviceTokenRepo struct{ db *gorm.DB }

// NewDeviceTokenRepo builds the repo.
func NewDeviceTokenRepo(db *gorm.DB) *DeviceTokenRepo { return &DeviceTokenRepo{db: db} }

// Upsert registers (or re-points) a device token to userID. Tokens are globally
// unique: if the same token was previously registered to another user (device
// handed over, reinstall), the row is updated to the current owner.
func (r *DeviceTokenRepo) Upsert(ctx context.Context, userID uuid.UUID, token string, platform models.DevicePlatform) error {
	now := time.Now()
	row := models.DeviceToken{
		Base:       models.Base{ID: uuid.New()},
		UserID:     userID,
		Token:      token,
		Platform:   platform,
		LastSeenAt: now,
	}
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "token"}},
			DoUpdates: clause.AssignmentColumns([]string{"user_id", "platform", "last_seen_at", "updated_at"}),
		}).
		Create(&row).Error
}

// Delete unregisters a token for a user, or ErrNotFound if the pair is absent.
func (r *DeviceTokenRepo) Delete(ctx context.Context, userID uuid.UUID, token string) error {
	res := r.db.WithContext(ctx).
		Where("user_id = ? AND token = ?", userID, token).
		Delete(&models.DeviceToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// TokensForUsers returns the FCM tokens of the given users. When pushEnabledOnly
// is set, users who have disabled push (users.notif_push = false) are excluded;
// when rsvpEnabledOnly is set, users who turned off RSVP-change alerts
// (users.notif_rsvp = false) are excluded too.
func (r *DeviceTokenRepo) TokensForUsers(ctx context.Context, userIDs []uuid.UUID, pushEnabledOnly, rsvpEnabledOnly bool) ([]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	q := r.db.WithContext(ctx).
		Table("device_tokens dt").
		Select("dt.token").
		Where("dt.user_id IN ?", userIDs)
	if pushEnabledOnly || rsvpEnabledOnly {
		q = q.Joins("JOIN users u ON u.id = dt.user_id")
	}
	if pushEnabledOnly {
		q = q.Where("u.notif_push = ?", true)
	}
	if rsvpEnabledOnly {
		q = q.Where("u.notif_rsvp = ?", true)
	}
	var tokens []string
	err := q.Scan(&tokens).Error
	return tokens, err
}

// DeleteTokens removes the given tokens outright (used to prune tokens FCM has
// reported as unregistered/invalid). No error if some are already gone.
func (r *DeviceTokenRepo) DeleteTokens(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("token IN ?", tokens).
		Delete(&models.DeviceToken{}).Error
}
