package service

import (
	"context"
	"errors"
	"log/slog"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// NotificationService fans out and reads personal notifications. It implements
// events.Notifier so producer services (guests, members) can emit notifications
// without importing this package.
type NotificationService struct {
	notifs   *repository.NotificationRepo
	weddings *repository.WeddingRepo
	log      *slog.Logger
}

// NewNotificationService builds the service.
func NewNotificationService(notifs *repository.NotificationRepo, weddings *repository.WeddingRepo, log *slog.Logger) *NotificationService {
	return &NotificationService{notifs: notifs, weddings: weddings, log: log}
}

// Notify fans a note out to every member of its wedding except the actor,
// inserting one notification row per recipient. Fire-and-forget: on error it
// logs and returns, never failing the caller's request.
func (s *NotificationService) Notify(ctx context.Context, n events.Note) {
	members, err := s.weddings.ListMembers(ctx, n.WeddingID)
	if err != nil {
		s.log.Error("notify: list members failed", "error", err, "wedding_id", n.WeddingID, "type", n.Type)
		return
	}
	weddingID := n.WeddingID
	rows := make([]models.Notification, 0, len(members))
	for _, m := range members {
		if m.UserID == n.ActorID {
			continue
		}
		rows = append(rows, models.Notification{
			UserID:    m.UserID,
			WeddingID: &weddingID,
			Type:      n.Type,
			Title:     n.Title,
			Body:      n.Body,
			Data:      n.Data,
		})
	}
	if len(rows) == 0 {
		return
	}
	if err := s.notifs.CreateBatch(ctx, rows); err != nil {
		s.log.Error("notify: create notifications failed", "error", err, "wedding_id", n.WeddingID, "type", n.Type)
	}
}

// List returns a user's notifications, newest first (optionally unread only).
func (s *NotificationService) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, error) {
	items, err := s.notifs.ListForUser(ctx, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, apperr.Internal("list notifications failed")
	}
	return items, nil
}

// UnreadCount returns how many unread notifications a user has.
func (s *NotificationService) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	n, err := s.notifs.UnreadCount(ctx, userID)
	if err != nil {
		return 0, apperr.Internal("unread count failed")
	}
	return n, nil
}

// MarkRead marks one of the user's notifications read, or NotFound.
func (s *NotificationService) MarkRead(ctx context.Context, userID, notifID uuid.UUID) error {
	err := s.notifs.MarkRead(ctx, userID, notifID)
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("notification not found")
	}
	if err != nil {
		return apperr.Internal("mark read failed")
	}
	return nil
}

// MarkAllRead marks every unread notification of the user read.
func (s *NotificationService) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := s.notifs.MarkAllRead(ctx, userID); err != nil {
		return apperr.Internal("mark all read failed")
	}
	return nil
}
