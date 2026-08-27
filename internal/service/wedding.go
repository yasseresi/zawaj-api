package service

import (
	"context"
	"errors"
	"time"

	"zawaj/internal/apperr"
	"zawaj/internal/dto"
	"zawaj/internal/models"
	"zawaj/internal/repository"
	"zawaj/pkg/token"

	"github.com/google/uuid"
)

const dateLayout = "2006-01-02"

// WeddingService holds wedding, membership, and invite-link logic.
type WeddingService struct {
	weddings *repository.WeddingRepo
}

// NewWeddingService builds the service.
func NewWeddingService(weddings *repository.WeddingRepo) *WeddingService {
	return &WeddingService{weddings: weddings}
}

// Create makes a wedding owned by the caller (who becomes owner member).
func (s *WeddingService) Create(ctx context.Context, ownerID uuid.UUID, req dto.CreateWeddingRequest) (*models.Wedding, error) {
	date, err := parseDate(req.EventDate)
	if err != nil {
		return nil, apperr.Validation("invalid event_date")
	}
	w := &models.Wedding{
		Name:        req.Name,
		EventDate:   date,
		Description: req.Description,
		OwnerID:     ownerID,
	}
	if err := s.weddings.CreateWithOwner(ctx, w); err != nil {
		return nil, apperr.Internal("create wedding failed")
	}
	return w, nil
}

// List returns the caller's weddings with role + guest count.
func (s *WeddingService) List(ctx context.Context, userID uuid.UUID) ([]repository.WeddingListItem, error) {
	items, err := s.weddings.ListForUser(ctx, userID)
	if err != nil {
		return nil, apperr.Internal("list weddings failed")
	}
	return items, nil
}

// Get returns a wedding by id (membership already enforced by middleware).
func (s *WeddingService) Get(ctx context.Context, weddingID uuid.UUID) (*models.Wedding, error) {
	w, err := s.weddings.ByID(ctx, weddingID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("wedding not found")
	}
	if err != nil {
		return nil, apperr.Internal("get wedding failed")
	}
	return w, nil
}

// Update edits a wedding's fields.
func (s *WeddingService) Update(ctx context.Context, weddingID uuid.UUID, req dto.UpdateWeddingRequest) (*models.Wedding, error) {
	w, err := s.Get(ctx, weddingID)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Description != nil {
		w.Description = *req.Description
	}
	if req.EventDate != nil {
		date, perr := parseDate(req.EventDate)
		if perr != nil {
			return nil, apperr.Validation("invalid event_date")
		}
		w.EventDate = date
	}
	if err := s.weddings.Update(ctx, w); err != nil {
		return nil, apperr.Internal("update wedding failed")
	}
	return w, nil
}

// Delete removes a wedding and its dependents.
func (s *WeddingService) Delete(ctx context.Context, weddingID uuid.UUID) error {
	if err := s.weddings.Delete(ctx, weddingID); err != nil {
		return apperr.Internal("delete wedding failed")
	}
	return nil
}

// ─── Invite links ───────────────────────────────────────────────────────────

// CreateInviteLink generates a role-scoped share link (editor|viewer only).
func (s *WeddingService) CreateInviteLink(ctx context.Context, weddingID, creatorID uuid.UUID, roleStr string) (*models.InviteLink, error) {
	role := models.Role(roleStr)
	if role != models.RoleEditor && role != models.RoleViewer {
		return nil, apperr.Validation("role must be editor or viewer")
	}
	tok, err := token.New(15)
	if err != nil {
		return nil, apperr.Internal("token generation failed")
	}
	l := &models.InviteLink{WeddingID: weddingID, CreatedBy: creatorID, Token: tok, Role: role}
	if err := s.weddings.CreateLink(ctx, l); err != nil {
		return nil, apperr.Internal("create invite link failed")
	}
	return l, nil
}

// ListInviteLinks returns a wedding's active links.
func (s *WeddingService) ListInviteLinks(ctx context.Context, weddingID uuid.UUID) ([]models.InviteLink, error) {
	links, err := s.weddings.ListLinks(ctx, weddingID)
	if err != nil {
		return nil, apperr.Internal("list invite links failed")
	}
	return links, nil
}

// RevokeInviteLink deactivates a link.
func (s *WeddingService) RevokeInviteLink(ctx context.Context, weddingID, linkID uuid.UUID) error {
	err := s.weddings.RevokeLink(ctx, weddingID, linkID)
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("invite link not found")
	}
	if err != nil {
		return apperr.Internal("revoke invite link failed")
	}
	return nil
}

// InvitePreview is the public-safe view of an invite link.
type InvitePreview struct {
	WeddingID   uuid.UUID   `json:"wedding_id"`
	WeddingName string      `json:"wedding_name"`
	Role        models.Role `json:"role"`
}

// PreviewInvite resolves a token to the wedding + role it grants.
func (s *WeddingService) PreviewInvite(ctx context.Context, tok string) (*InvitePreview, error) {
	l, err := s.validLink(ctx, tok)
	if err != nil {
		return nil, err
	}
	w, err := s.weddings.ByID(ctx, l.WeddingID)
	if err != nil {
		return nil, apperr.NotFound("invite not found")
	}
	return &InvitePreview{WeddingID: w.ID, WeddingName: w.Name, Role: l.Role}, nil
}

// AcceptInvite joins the caller to the wedding with the link's role. Idempotent;
// never downgrades an existing owner.
func (s *WeddingService) AcceptInvite(ctx context.Context, tok string, userID uuid.UUID) (*InvitePreview, error) {
	l, err := s.validLink(ctx, tok)
	if err != nil {
		return nil, err
	}
	// Never downgrade an existing membership: if the caller already holds a role
	// at least as high as the link's, keep it (owner accepting a viewer link, an
	// editor re-opening a viewer link, etc.).
	if existing, gerr := s.weddings.GetRole(ctx, l.WeddingID, userID); gerr == nil && existing.Rank() >= l.Role.Rank() {
		w, _ := s.weddings.ByID(ctx, l.WeddingID)
		return &InvitePreview{WeddingID: l.WeddingID, WeddingName: name(w), Role: existing}, nil
	}
	if err := s.weddings.UpsertMembership(ctx, l.WeddingID, userID, l.Role); err != nil {
		return nil, apperr.Internal("join failed")
	}
	w, err := s.weddings.ByID(ctx, l.WeddingID)
	if err != nil {
		return nil, apperr.Internal("join failed")
	}
	return &InvitePreview{WeddingID: w.ID, WeddingName: w.Name, Role: l.Role}, nil
}

func (s *WeddingService) validLink(ctx context.Context, tok string) (*models.InviteLink, error) {
	l, err := s.weddings.LinkByToken(ctx, tok)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, apperr.NotFound("invite not found")
	}
	if err != nil {
		return nil, apperr.Internal("invite lookup failed")
	}
	if l.Revoked {
		return nil, apperr.NotFound("invite is no longer active")
	}
	if l.ExpiresAt != nil && l.ExpiresAt.Before(time.Now()) {
		return nil, apperr.NotFound("invite has expired")
	}
	return l, nil
}

// ─── Members ────────────────────────────────────────────────────────────────

// Members lists a wedding's collaborators.
func (s *WeddingService) Members(ctx context.Context, weddingID uuid.UUID) ([]repository.MembershipView, error) {
	members, err := s.weddings.ListMembers(ctx, weddingID)
	if err != nil {
		return nil, apperr.Internal("list members failed")
	}
	return members, nil
}

// SetMemberRole changes a collaborator's role (owner action). Cannot target the
// owner or set someone to owner.
func (s *WeddingService) SetMemberRole(ctx context.Context, weddingID, targetID uuid.UUID, roleStr string) error {
	role := models.Role(roleStr)
	if role != models.RoleEditor && role != models.RoleViewer {
		return apperr.Validation("role must be editor or viewer")
	}
	cur, err := s.weddings.GetRole(ctx, weddingID, targetID)
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("member not found")
	}
	if err != nil {
		return apperr.Internal("set role failed")
	}
	if cur == models.RoleOwner {
		return apperr.Forbidden("cannot change the owner's role")
	}
	if err := s.weddings.SetMemberRole(ctx, weddingID, targetID, role); err != nil {
		return apperr.Internal("set role failed")
	}
	return nil
}

// RemoveMember removes a collaborator (owner action) or lets a member leave.
// The owner cannot be removed or leave (must transfer ownership first — later).
func (s *WeddingService) RemoveMember(ctx context.Context, weddingID, targetID uuid.UUID) error {
	cur, err := s.weddings.GetRole(ctx, weddingID, targetID)
	if errors.Is(err, repository.ErrNotFound) {
		return apperr.NotFound("member not found")
	}
	if err != nil {
		return apperr.Internal("remove member failed")
	}
	if cur == models.RoleOwner {
		return apperr.Forbidden("the owner cannot be removed")
	}
	if err := s.weddings.RemoveMember(ctx, weddingID, targetID); err != nil {
		return apperr.Internal("remove member failed")
	}
	return nil
}

func parseDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func name(w *models.Wedding) string {
	if w == nil {
		return ""
	}
	return w.Name
}
