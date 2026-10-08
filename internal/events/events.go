// Package events defines the seams between producers (guest/member services)
// and consumers (activity log, notifications). Producers depend on these
// interfaces, never on the concrete consumer — so the consumers can be built and
// swapped independently (e.g. by parallel agents). Noop impls let producers run
// before a consumer exists.
package events

import (
	"context"

	"zawaj/internal/models"

	"github.com/google/uuid"
)

// Activity is a single audit/feed entry to record.
type Activity struct {
	WeddingID uuid.UUID
	ActorID   uuid.UUID
	GuestID   *uuid.UUID
	Action    models.ActivityAction
	Meta      models.JSON
}

// Recorder persists activity entries (feed + per-guest history). Fire-and-forget:
// a failure must not fail the originating request (impl logs it).
type Recorder interface {
	Record(ctx context.Context, a Activity)
}

// NoopRecorder discards activity. Used until the activity module is wired.
type NoopRecorder struct{}

// Record does nothing.
func (NoopRecorder) Record(context.Context, Activity) {}

// Note is a personal notification to fan out to a wedding's other members.
type Note struct {
	WeddingID uuid.UUID
	ActorID   uuid.UUID // excluded from recipients
	Type      string
	Title     string
	Body      string
	Data      models.JSON
	// Silent stores the in-app notification without a push (e.g. batching:
	// the recipient was already pushed about the same thing).
	Silent bool
}

// Notifier fans a notification out to the relevant recipients.
type Notifier interface {
	Notify(ctx context.Context, n Note)
}

// NoopNotifier discards notifications. Used until the notifications module is wired.
type NoopNotifier struct{}

// Notify does nothing.
func (NoopNotifier) Notify(context.Context, Note) {}

// UserNotifier delivers a notification to a single named user (not a wedding
// fan-out). Used for invites, where the recipient is not yet a member.
type UserNotifier interface {
	NotifyUser(ctx context.Context, userID uuid.UUID, n Note)
}

// NoopUserNotifier discards user notifications.
type NoopUserNotifier struct{}

// NotifyUser does nothing.
func (NoopUserNotifier) NotifyUser(context.Context, uuid.UUID, Note) {}
