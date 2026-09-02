package service

import (
	"context"
	"log/slog"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/pagination"

	"github.com/google/uuid"
)

// ActivityService records and reads activity. It implements events.Recorder so
// producer services (guests, members) can log without importing this package.
type ActivityService struct {
	repo *repository.ActivityRepo
	log  *slog.Logger
}

// NewActivityService builds the service.
func NewActivityService(repo *repository.ActivityRepo, log *slog.Logger) *ActivityService {
	return &ActivityService{repo: repo, log: log}
}

// Record persists an activity entry. Fire-and-forget: on error it logs and
// returns, never failing the caller's request.
func (s *ActivityService) Record(ctx context.Context, a events.Activity) {
	entry := &models.ActivityLog{
		WeddingID: a.WeddingID,
		ActorID:   a.ActorID,
		GuestID:   a.GuestID,
		Action:    a.Action,
		Meta:      a.Meta,
	}
	if err := s.repo.Insert(ctx, entry); err != nil {
		s.log.Error("activity record failed", "error", err, "action", a.Action)
	}
}

// Feed returns a wedding's activity newest-first (screen 9).
func (s *ActivityService) Feed(ctx context.Context, weddingID uuid.UUID, limit, offset int) ([]models.ActivityLog, error) {
	items, err := s.repo.ListByWedding(ctx, weddingID, limit, offset)
	if err != nil {
		return nil, apperr.Internal("activity feed failed")
	}
	return items, nil
}

// FeedCursor returns a wedding's activity newest-first with keyset pagination.
// It returns the page and an opaque next cursor ("" when there are no more rows).
func (s *ActivityService) FeedCursor(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]repository.ActivityFeedRow, string, error) {
	cur, has, err := pagination.Decode(cursor)
	if err != nil {
		return nil, "", apperr.Validation("invalid cursor")
	}
	items, err := s.repo.ListFeedKeyset(ctx, weddingID, has, cur.CreatedAt, cur.ID, limit)
	if err != nil {
		return nil, "", apperr.Internal("activity feed failed")
	}
	next := ""
	if limit > 0 && len(items) == limit {
		last := items[len(items)-1]
		next = pagination.Encode(last.CreatedAt, last.ID)
	}
	return items, next, nil
}

// GuestHistory returns a guest's change history (screen 7 timeline).
func (s *ActivityService) GuestHistory(ctx context.Context, guestID uuid.UUID, limit int) ([]models.ActivityLog, error) {
	items, err := s.repo.ListByGuest(ctx, guestID, limit)
	if err != nil {
		return nil, apperr.Internal("guest history failed")
	}
	return items, nil
}
