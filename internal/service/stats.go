package service

import (
	"context"

	"zawaj/internal/apperr"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// StatsService computes a wedding's analytics figures (screen 8).
type StatsService struct {
	repo *repository.StatsRepo
}

// NewStatsService builds the service.
func NewStatsService(repo *repository.StatsRepo) *StatsService {
	return &StatsService{repo: repo}
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
