package repository

import (
	"context"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// InviteRepo is data access for targeted collaborator invites (screen 10).
type InviteRepo struct{ db *gorm.DB }

// NewInviteRepo builds the repo.
func NewInviteRepo(db *gorm.DB) *InviteRepo { return &InviteRepo{db: db} }

// PendingInviteView is a pending invite enriched with the wedding name/date and
// the inviter's display name, for the invitee's "pending invites" list.
type PendingInviteView struct {
	ID          uuid.UUID  `json:"id"`
	WeddingID   uuid.UUID  `json:"wedding_id"`
	WeddingName string     `json:"wedding_name"`
	EventDate   *time.Time `json:"event_date,omitempty"`
	InviterName string     `json:"inviter_name"`
	Role        models.Role `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Create inserts a new invite.
func (r *InviteRepo) Create(ctx context.Context, inv *models.WeddingInvite) error {
	return r.db.WithContext(ctx).Create(inv).Error
}

// ByID loads an invite by id.
func (r *InviteRepo) ByID(ctx context.Context, id uuid.UUID) (*models.WeddingInvite, error) {
	var inv models.WeddingInvite
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&inv).Error
	if err == gorm.ErrRecordNotFound {
		return nil, ErrNotFound
	}
	return &inv, err
}

// HasPending reports whether the invitee already has a pending invite for the
// wedding.
func (r *InviteRepo) HasPending(ctx context.Context, weddingID, inviteeID uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.WeddingInvite{}).
		Where("wedding_id = ? AND invitee_id = ? AND status = ?", weddingID, inviteeID, models.InvitePending).
		Count(&n).Error
	return n > 0, err
}

// SetStatus updates an invite's status.
func (r *InviteRepo) SetStatus(ctx context.Context, id uuid.UUID, status models.InviteStatus) error {
	return r.db.WithContext(ctx).Model(&models.WeddingInvite{}).
		Where("id = ?", id).Update("status", status).Error
}

// ListPendingForUser returns the invitee's pending invites newest-first,
// enriched with wedding + inviter details.
func (r *InviteRepo) ListPendingForUser(ctx context.Context, inviteeID uuid.UUID) ([]PendingInviteView, error) {
	var out []PendingInviteView
	err := r.db.WithContext(ctx).
		Table("wedding_invites AS i").
		Select(`i.id, i.wedding_id, i.role, i.created_at,
			w.name AS wedding_name, w.event_date AS event_date,
			COALESCE(NULLIF(u.display_name, ''), u.username) AS inviter_name`).
		Joins("LEFT JOIN weddings w ON w.id = i.wedding_id").
		Joins("LEFT JOIN users u ON u.id = i.inviter_id").
		Where("i.invitee_id = ? AND i.status = ?", inviteeID, models.InvitePending).
		Order("i.created_at DESC").
		Find(&out).Error
	return out, err
}
