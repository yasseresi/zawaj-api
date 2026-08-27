package repository

import (
	"context"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StatsRepo is read-only aggregate access over a wedding's guest list.
type StatsRepo struct{ db *gorm.DB }

// NewStatsRepo builds the repo.
func NewStatsRepo(db *gorm.DB) *StatsRepo { return &StatsRepo{db: db} }

// GuestStats are the aggregate figures for a wedding's analytics screen.
// A guest row covers 1 + Companions people (see models.Guest).
type GuestStats struct {
	TotalGuests    int64 `json:"total_guests"`
	Confirmed      int64 `json:"confirmed"`
	Pending        int64 `json:"pending"`
	Declined       int64 `json:"declined"`
	TotalPeople    int64 `json:"total_people"`
	ConfirmedSeats int64 `json:"confirmed_seats"`
}

// Guests computes every stat in a single aggregate query over the wedding's
// guests, using conditional aggregates so the DB does the counting.
func (r *StatsRepo) Guests(ctx context.Context, weddingID uuid.UUID) (GuestStats, error) {
	var out GuestStats
	err := r.db.WithContext(ctx).Model(&models.Guest{}).
		Select(
			"count(*) AS total_guests, "+
				"count(*) FILTER (WHERE status = ?) AS confirmed, "+
				"count(*) FILTER (WHERE status = ?) AS pending, "+
				"count(*) FILTER (WHERE status = ?) AS declined, "+
				"coalesce(sum(1 + companions), 0) AS total_people, "+
				"coalesce(sum(1 + companions) FILTER (WHERE status = ?), 0) AS confirmed_seats",
			models.StatusConfirmed, models.StatusPending, models.StatusDeclined, models.StatusConfirmed).
		Where("wedding_id = ?", weddingID).
		Scan(&out).Error
	return out, err
}
