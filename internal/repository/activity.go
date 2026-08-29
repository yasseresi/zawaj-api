package repository

import (
	"context"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ActivityRepo is data access for the activity log.
type ActivityRepo struct{ db *gorm.DB }

// NewActivityRepo builds the repo.
func NewActivityRepo(db *gorm.DB) *ActivityRepo { return &ActivityRepo{db: db} }

// Insert appends an activity entry.
func (r *ActivityRepo) Insert(ctx context.Context, a *models.ActivityLog) error {
	return r.db.WithContext(ctx).Create(a).Error
}

// ListByWedding returns a wedding's activity, newest first (feed).
func (r *ActivityRepo) ListByWedding(ctx context.Context, weddingID uuid.UUID, limit, offset int) ([]models.ActivityLog, error) {
	var out []models.ActivityLog
	err := r.db.WithContext(ctx).
		Where("wedding_id = ?", weddingID).
		Order("created_at DESC").Limit(limit).Offset(offset).
		Find(&out).Error
	return out, err
}

// ListByWeddingKeyset returns a wedding's activity newest-first using keyset
// pagination. When hasCursor is false it returns the first page; otherwise it
// returns rows strictly older than (curTime, curID). Stable under inserts.
func (r *ActivityRepo) ListByWeddingKeyset(ctx context.Context, weddingID uuid.UUID, hasCursor bool, curTime time.Time, curID uuid.UUID, limit int) ([]models.ActivityLog, error) {
	q := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID)
	if hasCursor {
		q = q.Where("(created_at, id) < (?, ?)", curTime, curID)
	}
	var out []models.ActivityLog
	err := q.Order("created_at DESC, id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// ListByGuest returns a guest's history, newest first.
func (r *ActivityRepo) ListByGuest(ctx context.Context, guestID uuid.UUID, limit int) ([]models.ActivityLog, error) {
	var out []models.ActivityLog
	err := r.db.WithContext(ctx).
		Where("guest_id = ?", guestID).
		Order("created_at DESC").Limit(limit).
		Find(&out).Error
	return out, err
}
