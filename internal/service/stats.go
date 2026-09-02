package service

import (
	"context"
	"fmt"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// StatsService computes a wedding's analytics figures (screen 8).
type StatsService struct {
	repo     *repository.StatsRepo
	activity events.Recorder
}

// NewStatsService builds the service.
func NewStatsService(repo *repository.StatsRepo, activity events.Recorder) *StatsService {
	return &StatsService{repo: repo, activity: activity}
}

// StatusBreakdown is the per-status guest count.
type StatusBreakdown struct {
	Confirmed int64 `json:"confirmed"`
	Pending   int64 `json:"pending"`
	Declined  int64 `json:"declined"`
}

// StatsResult is the analytics payload for a wedding.
type StatsResult struct {
	TotalGuests    int64           `json:"total_guests"`
	ByStatus       StatusBreakdown `json:"by_status"`
	TotalPeople    int64           `json:"total_people"`
	ConfirmedSeats int64           `json:"confirmed_seats"`
}

// Guests returns the aggregate stats for a wedding.
func (s *StatsService) Guests(ctx context.Context, weddingID uuid.UUID) (*StatsResult, error) {
	stats, err := s.repo.Guests(ctx, weddingID)
	if err != nil {
		return nil, apperr.Internal("guest stats failed")
	}
	return &StatsResult{
		TotalGuests: stats.TotalGuests,
		ByStatus: StatusBreakdown{
			Confirmed: stats.Confirmed,
			Pending:   stats.Pending,
			Declined:  stats.Declined,
		},
		TotalPeople:    stats.TotalPeople,
		ConfirmedSeats: stats.ConfirmedSeats,
	}, nil
}

// SendReminders records a broadcast-reminder action for every pending guest and
// returns how many guests it targeted (screen 8 "Send Reminders"). Delivery to
// guests (SMS/email) is out of scope here; this establishes the count and the
// audited activity entry the feed and analytics screen rely on.
func (s *StatsService) SendReminders(ctx context.Context, weddingID, actorID uuid.UUID) (int64, error) {
	stats, err := s.repo.Guests(ctx, weddingID)
	if err != nil {
		return 0, apperr.Internal("guest stats failed")
	}
	count := stats.Pending
	if count > 0 {
		s.activity.Record(ctx, events.Activity{
			WeddingID: weddingID,
			ActorID:   actorID,
			Action:    models.ActRemindersSent,
			Meta:      models.JSON(fmt.Sprintf(`{"count":%d}`, count)),
		})
	}
	return count, nil
}
