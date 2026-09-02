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

// ActivityFeedRow is an activity entry enriched with the actor's display name
// and (when the entry targets a guest) the guest's name, for the feed screen.
type ActivityFeedRow struct {
	models.ActivityLog
	ActorName string  `json:"actor_name"`
	GuestName *string `json:"guest_name,omitempty"`
}

// ListFeedKeyset returns enriched activity rows newest-first using keyset
// pagination, joining the actor's display name/username and guest name in one
// query. When hasCursor is false it returns the first page.
func (r *ActivityRepo) ListFeedKeyset(ctx context.Context, weddingID uuid.UUID, hasCursor bool, curTime time.Time, curID uuid.UUID, limit int) ([]ActivityFeedRow, error) {
	q := r.db.WithContext(ctx).
		Table("activity_logs AS a").
		Select("a.*, COALESCE(NULLIF(u.display_name, ''), u.username) AS actor_name, g.full_name AS guest_name").
		Joins("LEFT JOIN users u ON u.id = a.actor_id").
		Joins("LEFT JOIN guests g ON g.id = a.guest_id").
		Where("a.wedding_id = ?", weddingID)
	if hasCursor {
		q = q.Where("(a.created_at, a.id) < (?, ?)", curTime, curID)
	}
	var out []ActivityFeedRow
	err := q.Order("a.created_at DESC, a.id DESC").Limit(limit).Find(&out).Error
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
