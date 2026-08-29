package repository

import (
	"context"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// NotificationRepo is data access for personal notifications, always user-scoped.
type NotificationRepo struct{ db *gorm.DB }

// NewNotificationRepo builds the repo.
func NewNotificationRepo(db *gorm.DB) *NotificationRepo { return &NotificationRepo{db: db} }

// Create inserts a single notification.
func (r *NotificationRepo) Create(ctx context.Context, n *models.Notification) error {
	return r.db.WithContext(ctx).Create(n).Error
}

// CreateBatch inserts many notifications in one statement. A nil/empty slice is a no-op.
func (r *NotificationRepo) CreateBatch(ctx context.Context, ns []models.Notification) error {
	if len(ns) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&ns).Error
}

// ListForUser returns a user's notifications, newest first. When unreadOnly is
// true only unread (read_at IS NULL) rows are returned.
func (r *NotificationRepo) ListForUser(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, error) {
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if limit <= 0 {
		limit = 50
	}
	var out []models.Notification
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&out).Error
	return out, err
}

// ListForUserKeyset returns a user's notifications newest-first using keyset
// pagination. hasCursor=false returns the first page; otherwise rows strictly
// older than (curTime, curID). unreadOnly filters to read_at IS NULL.
func (r *NotificationRepo) ListForUserKeyset(ctx context.Context, userID uuid.UUID, unreadOnly, hasCursor bool, curTime time.Time, curID uuid.UUID, limit int) ([]models.Notification, error) {
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if hasCursor {
		q = q.Where("(created_at, id) < (?, ?)", curTime, curID)
	}
	if limit <= 0 {
		limit = 50
	}
	var out []models.Notification
	err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// UnreadCount returns how many unread notifications a user has.
func (r *NotificationRepo) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).Count(&n).Error
	return n, err
}

// MarkRead marks one notification read, scoped to its owner, or ErrNotFound.
// Already-read rows are left untouched (idempotent) but still resolve to success.
func (r *NotificationRepo) MarkRead(ctx context.Context, userID, notifID uuid.UUID) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", notifID, userID).
		Update("read_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// Either the notification does not belong to this user or it is already
		// read. Distinguish the two so a repeat call stays idempotent.
		var count int64
		if err := r.db.WithContext(ctx).Model(&models.Notification{}).
			Where("id = ? AND user_id = ?", notifID, userID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return ErrNotFound
		}
	}
	return nil
}

// MarkAllRead marks every unread notification of a user read.
func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now()).Error
}
