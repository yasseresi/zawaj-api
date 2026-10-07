package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

const (
	// joinRequestCooldown is how long a declined user must wait before asking
	// to join the same wedding again, so they can't flood the owner.
	joinRequestCooldown = 24 * time.Hour
	// maxPendingListed bounds the owner's review list.
	maxPendingListed = 100

	noteJoinRequested = "join_requested"
	noteJoinApproved  = "join_approved"
	noteJoinDeclined  = "join_declined"
)

// JoinStatus is the outcome of accepting an invite link.
type JoinStatus string

const (
	// JoinStatusMember: the caller already belongs to the wedding (unchanged).
	JoinStatusMember JoinStatus = "member"
	// JoinStatusPending: a join request awaits the owner's decision.
	JoinStatusPending JoinStatus = "pending"
)

// JoinResult is returned by RequestFromLink.
type JoinResult struct {
	WeddingID   uuid.UUID   `json:"wedding_id"`
	WeddingName string      `json:"wedding_name"`
	Role        models.Role `json:"role"`
	Status      JoinStatus  `json:"status"`
	RequestID   *uuid.UUID  `json:"request_id,omitempty"`
}

// JoinService turns invite-link accepts into join requests the owner approves
// or declines. Links stay shareable; nobody becomes a member through one
// without the owner's approval.
type JoinService struct {
	weddings *repository.WeddingRepo
	joins    *repository.JoinRequestRepo
	users    *repository.UserRepo
	activity events.Recorder
	notify   events.UserNotifier
}

// NewJoinService builds the service.
func NewJoinService(weddings *repository.WeddingRepo, joins *repository.JoinRequestRepo, users *repository.UserRepo, activity events.Recorder, notify events.UserNotifier) *JoinService {
	return &JoinService{weddings: weddings, joins: joins, users: users, activity: activity, notify: notify}
}

// RequestFromLink handles a user accepting an invite link:
//   - already a member → their current role, unchanged (no request);
//   - removed by the owner → 403 removed_from_wedding (no request);
//   - declined within the cooldown → 409 join_request_declined;
//   - otherwise → a pending request (existing one if already pending) at the
//     link's role capped by the owner's last decision; the owner is notified
//     when the request is new.
func (s *JoinService) RequestFromLink(ctx context.Context, tok string, userID uuid.UUID) (*JoinResult, error) {
	l, err := validLink(ctx, s.weddings, tok)
	if err != nil {
		return nil, err
	}
	w, err := s.weddings.ByID(ctx, l.WeddingID)
	if err != nil {
		return nil, apperr.Internal("join failed")
	}

	existing, gerr := s.weddings.GetRole(ctx, l.WeddingID, userID)
	if gerr == nil {
		return &JoinResult{WeddingID: w.ID, WeddingName: w.Name, Role: existing, Status: JoinStatusMember}, nil
	}
	if !errors.Is(gerr, repository.ErrNotFound) {
		return nil, apperr.Internal("join failed")
	}

	role, err := s.cappedRole(ctx, l.WeddingID, userID, l.Role)
	if err != nil {
		return nil, err
	}
	declinedAt, derr := s.joins.LastDeclinedAt(ctx, l.WeddingID, userID)
	if derr == nil && time.Since(declinedAt) < joinRequestCooldown {
		return nil, apperr.JoinRequestDeclined("the owner declined your request; try again later")
	}
	if derr != nil && !errors.Is(derr, repository.ErrNotFound) {
		return nil, apperr.Internal("join failed")
	}

	req, created, err := s.joins.CreatePending(ctx, l.WeddingID, userID, l.ID, role)
	if err != nil {
		return nil, apperr.Internal("join failed")
	}
	if created {
		s.notifyOwner(ctx, w, userID, req.ID)
	}
	id := req.ID
	return &JoinResult{WeddingID: w.ID, WeddingName: w.Name, Role: req.Role, Status: JoinStatusPending, RequestID: &id}, nil
}

// ListPending returns the wedding's pending join requests (owner).
func (s *JoinService) ListPending(ctx context.Context, weddingID uuid.UUID) ([]repository.JoinRequestView, error) {
	items, err := s.joins.ListPending(ctx, weddingID, maxPendingListed)
	if err != nil {
		return nil, apperr.Internal("list join requests failed")
	}
	return items, nil
}

// Approve admits the requester (owner). roleStr may lower the requested role
// but never raise it; empty keeps it.
func (s *JoinService) Approve(ctx context.Context, weddingID, requestID, ownerID uuid.UUID, roleStr string) (*models.JoinRequest, error) {
	req, err := s.loadPending(ctx, weddingID, requestID)
	if err != nil {
		return nil, err
	}
	role := req.Role
	if roleStr != "" {
		r := models.Role(roleStr)
		if r != models.RoleEditor && r != models.RoleViewer {
			return nil, apperr.Validation("role must be editor or viewer")
		}
		if r.Rank() > req.Role.Rank() {
			return nil, apperr.Validation("cannot approve above the role the invite link grants")
		}
		role = r
	}
	decided, created, err := s.joins.Approve(ctx, weddingID, requestID, ownerID, role)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("join request not found")
	}
	if err != nil {
		return nil, apperr.Internal("approve failed")
	}
	if created {
		s.activity.Record(ctx, events.Activity{WeddingID: weddingID, ActorID: decided.UserID, Action: models.ActCollaboratorJoin})
	}
	s.notifyRequester(ctx, weddingID, ownerID, decided.UserID, noteJoinApproved, "تمت الموافقة على طلب الانضمام")
	return decided, nil
}

// Decline refuses the request (owner). The requester may ask again after the
// cooldown.
func (s *JoinService) Decline(ctx context.Context, weddingID, requestID, ownerID uuid.UUID) (*models.JoinRequest, error) {
	decided, err := s.joins.Decline(ctx, weddingID, requestID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("join request not found")
	}
	if err != nil {
		return nil, apperr.Internal("decline failed")
	}
	s.notifyRequester(ctx, weddingID, ownerID, decided.UserID, noteJoinDeclined, "تم رفض طلب الانضمام")
	return decided, nil
}

// loadPending returns the request if it is pending in this wedding, before
// role checks (the repository re-checks under a row lock when deciding).
func (s *JoinService) loadPending(ctx context.Context, weddingID, requestID uuid.UUID) (*models.JoinRequest, error) {
	req, err := s.joins.PendingByID(ctx, weddingID, requestID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("join request not found")
	}
	if err != nil {
		return nil, apperr.Internal("join request lookup failed")
	}
	return req, nil
}

// cappedRole applies the user's membership ceiling (if any) to role: a
// removed user is refused, a demoted one gets at most the owner's role.
func (s *JoinService) cappedRole(ctx context.Context, weddingID, userID uuid.UUID, role models.Role) (models.Role, error) {
	c, err := s.weddings.GetCeiling(ctx, weddingID, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return role, nil
	}
	if err != nil {
		return "", apperr.Internal("join failed")
	}
	return applyCeiling(c, role)
}

func (s *JoinService) notifyOwner(ctx context.Context, w *models.Wedding, requesterID, requestID uuid.UUID) {
	body := ""
	if u, err := s.users.ByID(ctx, requesterID); err == nil {
		body = fmt.Sprintf("%s (@%s)", u.DisplayName, u.Username)
	}
	data, _ := json.Marshal(map[string]string{"request_id": requestID.String()})
	s.notify.NotifyUser(ctx, w.OwnerID, events.Note{
		WeddingID: w.ID,
		ActorID:   requesterID,
		Type:      noteJoinRequested,
		Title:     "طلب انضمام جديد",
		Body:      body,
		Data:      data,
	})
}

func (s *JoinService) notifyRequester(ctx context.Context, weddingID, ownerID, requesterID uuid.UUID, typ, title string) {
	body := ""
	if w, err := s.weddings.ByID(ctx, weddingID); err == nil {
		body = w.Name
	}
	s.notify.NotifyUser(ctx, requesterID, events.Note{
		WeddingID: weddingID, ActorID: ownerID, Type: typ, Title: title, Body: body,
	})
}
