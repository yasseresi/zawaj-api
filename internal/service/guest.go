package service

import (
	"context"
	"errors"
	"fmt"

	"zawaj/internal/apperr"
	"zawaj/internal/dto"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// GuestService holds guest + note logic. It emits activity and notifications via
// the events seams (Recorder/Notifier) so those consumers stay decoupled.
type GuestService struct {
	guests   *repository.GuestRepo
	activity events.Recorder
	notify   events.Notifier
}

// NewGuestService builds the service.
func NewGuestService(guests *repository.GuestRepo, activity events.Recorder, notify events.Notifier) *GuestService {
	return &GuestService{guests: guests, activity: activity, notify: notify}
}

// GuestListResult is the guest list plus tab counts.
type GuestListResult struct {
	Items  []models.Guest          `json:"items"`
	Counts repository.StatusCounts `json:"counts"`
}

// GuestDetail is a guest with its notes (history is fetched by the caller).
type GuestDetail struct {
	Guest models.Guest       `json:"guest"`
	Notes []models.GuestNote `json:"notes"`
}

// List returns filtered guests plus tab counts.
func (s *GuestService) List(ctx context.Context, weddingID uuid.UUID, f repository.GuestFilter) (*GuestListResult, error) {
	items, err := s.guests.List(ctx, weddingID, f)
	if err != nil {
		return nil, apperr.Internal("list guests failed")
	}
	counts, err := s.guests.Counts(ctx, weddingID)
	if err != nil {
		return nil, apperr.Internal("guest counts failed")
	}
	return &GuestListResult{Items: items, Counts: counts}, nil
}

// Get returns a guest and its notes.
func (s *GuestService) Get(ctx context.Context, weddingID, guestID uuid.UUID) (*GuestDetail, error) {
	g, err := s.load(ctx, weddingID, guestID)
	if err != nil {
		return nil, err
	}
	notes, err := s.guests.ListNotes(ctx, guestID)
	if err != nil {
		return nil, apperr.Internal("load notes failed")
	}
	return &GuestDetail{Guest: *g, Notes: notes}, nil
}

// Create adds a guest, records activity, and notifies collaborators.
func (s *GuestService) Create(ctx context.Context, weddingID, actorID uuid.UUID, req dto.CreateGuestRequest) (*models.Guest, error) {
	status := models.StatusPending
	if req.Status != "" {
		status = models.RSVPStatus(req.Status)
	}
	companions := 0
	if req.Companions != nil {
		companions = *req.Companions
	}
	g := &models.Guest{
		WeddingID:    weddingID,
		AddedBy:      actorID,
		FullName:     req.FullName,
		Contact:      req.Contact,
		Relationship: req.Relationship,
		Status:       status,
		Companions:   companions,
		TableLabel:   req.TableLabel,
		Meal:         req.Meal,
	}
	if err := s.guests.Create(ctx, g); err != nil {
		return nil, apperr.Internal("create guest failed")
	}
	if req.Note != nil && *req.Note != "" {
		_ = s.guests.AddNote(ctx, &models.GuestNote{GuestID: g.ID, AuthorID: actorID, Body: *req.Note})
	}
	s.record(ctx, weddingID, actorID, &g.ID, models.ActGuestAdded)
	s.notifyMembers(ctx, weddingID, actorID, "guest_added", "ضيف جديد", g.FullName)
	return g, nil
}

// Update edits a guest. A status change is recorded as its own activity.
func (s *GuestService) Update(ctx context.Context, weddingID, actorID, guestID uuid.UUID, req dto.UpdateGuestRequest) (*models.Guest, error) {
	g, err := s.load(ctx, weddingID, guestID)
	if err != nil {
		return nil, err
	}
	statusChanged := false
	if req.FullName != nil {
		g.FullName = *req.FullName
	}
	if req.Contact != nil {
		g.Contact = *req.Contact
	}
	if req.Relationship != nil {
		g.Relationship = *req.Relationship
	}
	if req.Companions != nil {
		g.Companions = *req.Companions
	}
	if req.TableLabel != nil {
		g.TableLabel = *req.TableLabel
	}
	if req.Meal != nil {
		g.Meal = *req.Meal
	}
	if req.Status != nil && models.RSVPStatus(*req.Status) != g.Status {
		g.Status = models.RSVPStatus(*req.Status)
		statusChanged = true
	}
	if err := s.guests.Update(ctx, g); err != nil {
		return nil, apperr.Internal("update guest failed")
	}
	if statusChanged {
		s.recordStatusChange(ctx, weddingID, actorID, g.ID, g.Status)
		s.notifyMembers(ctx, weddingID, actorID, "guest_status_changed", "تحديث حالة ضيف", g.FullName)
	} else {
		s.record(ctx, weddingID, actorID, &g.ID, models.ActGuestUpdated)
	}
	return g, nil
}

// SetStatus changes only the RSVP status (screen 7 edit-status sheet).
func (s *GuestService) SetStatus(ctx context.Context, weddingID, actorID, guestID uuid.UUID, statusStr string) (*models.Guest, error) {
	g, err := s.load(ctx, weddingID, guestID)
	if err != nil {
		return nil, err
	}
	newStatus := models.RSVPStatus(statusStr)
	if newStatus == g.Status {
		return g, nil
	}
	g.Status = newStatus
	if err := s.guests.Update(ctx, g); err != nil {
		return nil, apperr.Internal("update status failed")
	}
	s.recordStatusChange(ctx, weddingID, actorID, g.ID, g.Status)
	s.notifyMembers(ctx, weddingID, actorID, "guest_status_changed", "تحديث حالة ضيف", g.FullName)
	return g, nil
}

// Delete removes a guest.
func (s *GuestService) Delete(ctx context.Context, weddingID, guestID uuid.UUID) error {
	err := s.guests.Delete(ctx, weddingID, guestID)
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("guest not found")
	}
	if err != nil {
		return apperr.Internal("delete guest failed")
	}
	return nil
}

// AddNote appends a note to a guest and records activity.
func (s *GuestService) AddNote(ctx context.Context, weddingID, actorID, guestID uuid.UUID, body string) (*models.GuestNote, error) {
	g, err := s.load(ctx, weddingID, guestID)
	if err != nil {
		return nil, err
	}
	n := &models.GuestNote{GuestID: g.ID, AuthorID: actorID, Body: body}
	if err := s.guests.AddNote(ctx, n); err != nil {
		return nil, apperr.Internal("add note failed")
	}
	s.record(ctx, weddingID, actorID, &g.ID, models.ActNoteAdded)
	return n, nil
}

func (s *GuestService) load(ctx context.Context, weddingID, guestID uuid.UUID) (*models.Guest, error) {
	g, err := s.guests.ByID(ctx, weddingID, guestID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("guest not found")
	}
	if err != nil {
		return nil, apperr.Internal("load guest failed")
	}
	return g, nil
}

// recordStatusChange records a status change carrying the new status in meta so
// the activity feed can distinguish confirmed / declined / pending entries.
func (s *GuestService) recordStatusChange(ctx context.Context, weddingID, actorID uuid.UUID, guestID uuid.UUID, status models.RSVPStatus) {
	s.activity.Record(ctx, events.Activity{
		WeddingID: weddingID, ActorID: actorID, GuestID: &guestID,
		Action: models.ActGuestStatusChange,
		Meta:   models.JSON(fmt.Sprintf(`{"status":%q}`, string(status))),
	})
}

func (s *GuestService) record(ctx context.Context, weddingID, actorID uuid.UUID, guestID *uuid.UUID, action models.ActivityAction) {
	s.activity.Record(ctx, events.Activity{
		WeddingID: weddingID, ActorID: actorID, GuestID: guestID, Action: action,
	})
}

func (s *GuestService) notifyMembers(ctx context.Context, weddingID, actorID uuid.UUID, typ, title, body string) {
	s.notify.Notify(ctx, events.Note{
		WeddingID: weddingID, ActorID: actorID, Type: typ, Title: title, Body: body,
	})
}
