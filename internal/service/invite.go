package service

import (
	"context"
	"errors"

	"zawaj/internal/apperr"
	"zawaj/internal/events"
	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// InviteService handles targeted collaborator invites (screen 10): an owner
// invites a specific registered user to a wedding; the invitee accepts (which
// grants membership) or declines.
type InviteService struct {
	invites  *repository.InviteRepo
	weddings *repository.WeddingRepo
	users    *repository.UserRepo
	activity events.Recorder
	notify   events.UserNotifier
}

// NewInviteService builds the service.
func NewInviteService(invites *repository.InviteRepo, weddings *repository.WeddingRepo, users *repository.UserRepo, activity events.Recorder, notify events.UserNotifier) *InviteService {
	return &InviteService{invites: invites, weddings: weddings, users: users, activity: activity, notify: notify}
}

// Invite creates a pending invite for [username] to join [weddingID] with [role]
// (editor|viewer). Caller must be the wedding owner (enforced by middleware).
func (s *InviteService) Invite(ctx context.Context, weddingID, inviterID uuid.UUID, username string, role models.Role) (*models.WeddingInvite, error) {
	if role != models.RoleEditor && role != models.RoleViewer {
		return nil, apperr.Validation("role must be editor or viewer")
	}
	invitee, err := s.users.ByUsername(ctx, username)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("user not found")
	}
	if err != nil {
		return nil, apperr.Internal("user lookup failed")
	}
	if invitee.ID == inviterID {
		return nil, apperr.Validation("you cannot invite yourself")
	}
	// Already a member? Nothing to invite.
	if existing, gerr := s.weddings.GetRole(ctx, weddingID, invitee.ID); gerr == nil && existing != "" {
		return nil, apperr.Conflict("user is already a collaborator")
	}
	pending, err := s.invites.HasPending(ctx, weddingID, invitee.ID)
	if err != nil {
		return nil, apperr.Internal("invite check failed")
	}
	if pending {
		return nil, apperr.Conflict("an invite is already pending for this user")
	}
	inv := &models.WeddingInvite{
		WeddingID: weddingID,
		InviterID: inviterID,
		InviteeID: invitee.ID,
		Role:      role,
		Status:    models.InvitePending,
	}
	if err := s.invites.Create(ctx, inv); err != nil {
		return nil, apperr.Internal("create invite failed")
	}
	// Notify the invitee (not yet a member, so the member fan-out would miss
	// them). Best-effort: a wedding-title lookup failure must not fail the invite.
	body := ""
	if w, werr := s.weddings.ByID(ctx, weddingID); werr == nil {
		body = w.Name
	}
	s.notify.NotifyUser(ctx, invitee.ID, events.Note{
		WeddingID: weddingID,
		ActorID:   inviterID,
		Type:      "invite_received",
		Title:     "دعوة جديدة",
		Body:      body,
	})
	return inv, nil
}

// ListMine returns the caller's pending invites, enriched for display.
func (s *InviteService) ListMine(ctx context.Context, userID uuid.UUID) ([]repository.PendingInviteView, error) {
	items, err := s.invites.ListPendingForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal("list invites failed")
	}
	return items, nil
}

// Accept grants the caller membership per the invite's role and marks it
// accepted. The invite must belong to the caller and still be pending; otherwise
// 404 (existence hidden).
func (s *InviteService) Accept(ctx context.Context, inviteID, userID uuid.UUID) (*models.Wedding, error) {
	inv, err := s.load(ctx, inviteID, userID)
	if err != nil {
		return nil, err
	}
	// Never downgrade an existing higher role.
	if existing, gerr := s.weddings.GetRole(ctx, inv.WeddingID, userID); gerr != nil || existing.Rank() < inv.Role.Rank() {
		if uerr := s.weddings.UpsertMembership(ctx, inv.WeddingID, userID, inv.Role); uerr != nil {
			return nil, apperr.Internal("join failed")
		}
	}
	if err := s.invites.SetStatus(ctx, inv.ID, models.InviteAccepted); err != nil {
		return nil, apperr.Internal("accept invite failed")
	}
	s.activity.Record(ctx, events.Activity{
		WeddingID: inv.WeddingID, ActorID: userID, Action: models.ActCollaboratorJoin,
	})
	w, err := s.weddings.ByID(ctx, inv.WeddingID)
	if err != nil {
		return nil, apperr.Internal("load wedding failed")
	}
	return w, nil
}

// Decline marks the caller's pending invite declined.
func (s *InviteService) Decline(ctx context.Context, inviteID, userID uuid.UUID) error {
	inv, err := s.load(ctx, inviteID, userID)
	if err != nil {
		return err
	}
	if err := s.invites.SetStatus(ctx, inv.ID, models.InviteDeclined); err != nil {
		return apperr.Internal("decline invite failed")
	}
	return nil
}

// load fetches a pending invite that belongs to userID, hiding existence
// (404) for invites that are missing, already resolved, or someone else's.
func (s *InviteService) load(ctx context.Context, inviteID, userID uuid.UUID) (*models.WeddingInvite, error) {
	inv, err := s.invites.ByID(ctx, inviteID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("invite not found")
	}
	if err != nil {
		return nil, apperr.Internal("invite lookup failed")
	}
	if inv.InviteeID != userID || inv.Status != models.InvitePending {
		return nil, apperr.NotFound("invite not found")
	}
	return inv, nil
}
