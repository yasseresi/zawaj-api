package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/push"
	"zawaj/internal/repository"
	"zawaj/pkg/pagination"

	"github.com/google/uuid"
)

// pushTimeout bounds a fan-out's push delivery, which runs detached from the
// originating request so a slow FCM call never delays the API response.
const pushTimeout = 10 * time.Second

// NotificationService fans out and reads personal notifications. It implements
// events.Notifier so producer services (guests, members) can emit notifications
// without importing this package.
type NotificationService struct {
	notifs   *repository.NotificationRepo
	weddings *repository.WeddingRepo
	devices  *repository.DeviceTokenRepo
	pusher   push.Sender
	log      *slog.Logger
}

// NewNotificationService builds the service. devices and pusher power FCM push;
// pass a push.Noop when push is disabled.
func NewNotificationService(notifs *repository.NotificationRepo, weddings *repository.WeddingRepo, devices *repository.DeviceTokenRepo, pusher push.Sender, log *slog.Logger) *NotificationService {
	return &NotificationService{notifs: notifs, weddings: weddings, devices: devices, pusher: pusher, log: log}
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
	recipients := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		if m.UserID == n.ActorID {
			continue
		}
		recipients = append(recipients, m.UserID)
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
		return
	}

	s.pushToDevices(recipients, n)
}

// pushToDevices delivers a fan-out as FCM push to recipients who have push
// enabled. It runs detached from the request (its own timeout) and prunes tokens
// FCM reports as invalid. Best-effort: failures are logged, never surfaced.
func (s *NotificationService) pushToDevices(recipients []uuid.UUID, n events.Note) {
	if s.pusher == nil || s.devices == nil || len(recipients) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), pushTimeout)
		defer cancel()

		tokens, err := s.devices.TokensForUsers(ctx, recipients, true)
		if err != nil {
			s.log.Error("notify: load device tokens failed", "error", err, "type", n.Type)
			return
		}
		if len(tokens) == 0 {
			return
		}

		data := map[string]string{"type": n.Type, "wedding_id": n.WeddingID.String()}
		invalid := s.pusher.Send(ctx, tokens, push.Message{Title: n.Title, Body: n.Body, Data: data})
		if len(invalid) > 0 {
			if err := s.devices.DeleteTokens(ctx, invalid); err != nil {
				s.log.Error("notify: prune invalid tokens failed", "error", err, "count", len(invalid))
			}
		}
	}()
}

// List returns a user's notifications, newest first (optionally unread only).
func (s *NotificationService) List(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]models.Notification, error) {
	items, err := s.notifs.ListForUser(ctx, userID, unreadOnly, limit, offset)
	if err != nil {
		return nil, apperr.Internal("list notifications failed")
	}
	return items, nil
}

// ListCursor returns a user's notifications newest-first with keyset pagination,
// plus an opaque next cursor ("" when there are no more rows).
func (s *NotificationService) ListCursor(ctx context.Context, userID uuid.UUID, unreadOnly bool, cursor string, limit int) ([]models.Notification, string, error) {
	cur, has, err := pagination.Decode(cursor)
	if err != nil {
		return nil, "", apperr.Validation("invalid cursor")
	}
	items, err := s.notifs.ListForUserKeyset(ctx, userID, unreadOnly, has, cur.CreatedAt, cur.ID, limit)
	if err != nil {
		return nil, "", apperr.Internal("list notifications failed")
	}
	next := ""
	if limit > 0 && len(items) == limit {
		last := items[len(items)-1]
		next = pagination.Encode(last.CreatedAt, last.ID)
	}
	return items, next, nil
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
