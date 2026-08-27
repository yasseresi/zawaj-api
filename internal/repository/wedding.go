package repository

import (
	"context"
	"errors"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WeddingRepo is data access for weddings, memberships, and invite links.
type WeddingRepo struct{ db *gorm.DB }

// NewWeddingRepo builds the repo.
func NewWeddingRepo(db *gorm.DB) *WeddingRepo { return &WeddingRepo{db: db} }

// WeddingListItem is a wedding plus the caller's role and its guest count.
type WeddingListItem struct {
	models.Wedding
	Role       models.Role `json:"my_role"`
	GuestCount int64       `json:"guest_count"`
}

// CreateWithOwner inserts a wedding and the owner's membership in one transaction.
func (r *WeddingRepo) CreateWithOwner(ctx context.Context, w *models.Wedding) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(w).Error; err != nil {
			return err
		}
		m := &models.Membership{
			WeddingID: w.ID,
			UserID:    w.OwnerID,
			Role:      models.RoleOwner,
			JoinedAt:  time.Now(),
		}
		return tx.Create(m).Error
	})
}

// ListForUser returns weddings the user belongs to, with role and guest count.
func (r *WeddingRepo) ListForUser(ctx context.Context, userID uuid.UUID) ([]WeddingListItem, error) {
	var items []WeddingListItem
	err := r.db.WithContext(ctx).
		Table("weddings w").
		Select("w.*, m.role AS role, "+
			"(SELECT count(*) FROM guests g WHERE g.wedding_id = w.id) AS guest_count").
		Joins("JOIN memberships m ON m.wedding_id = w.id AND m.user_id = ?", userID).
		Order("w.created_at DESC").
		Scan(&items).Error
	return items, err
}

// ByID loads a wedding, or ErrNotFound.
func (r *WeddingRepo) ByID(ctx context.Context, id uuid.UUID) (*models.Wedding, error) {
	var w models.Wedding
	err := r.db.WithContext(ctx).First(&w, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// Update persists changes to a wedding.
func (r *WeddingRepo) Update(ctx context.Context, w *models.Wedding) error {
	return r.db.WithContext(ctx).Save(w).Error
}

// Delete removes a wedding and its dependent rows (memberships, guests, notes,
// invite links, activity) in one transaction.
func (r *WeddingRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM guest_notes WHERE guest_id IN (SELECT id FROM guests WHERE wedding_id = ?)", id).Error; err != nil {
			return err
		}
		for _, q := range []string{
			"DELETE FROM guests WHERE wedding_id = ?",
			"DELETE FROM activity_logs WHERE wedding_id = ?",
			"DELETE FROM invite_links WHERE wedding_id = ?",
			"DELETE FROM memberships WHERE wedding_id = ?",
		} {
			if err := tx.Exec(q, id).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&models.Wedding{}, "id = ?", id).Error
	})
}

// ─── Memberships ────────────────────────────────────────────────────────────

// GetRole returns the caller's role on a wedding, or ErrNotFound if not a member.
func (r *WeddingRepo) GetRole(ctx context.Context, weddingID, userID uuid.UUID) (models.Role, error) {
	var m models.Membership
	err := r.db.WithContext(ctx).
		Where("wedding_id = ? AND user_id = ?", weddingID, userID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return m.Role, nil
}

// MembershipView is a member row joined with the user's public fields.
type MembershipView struct {
	UserID      uuid.UUID   `json:"user_id"`
	Username    string      `json:"username"`
	DisplayName string      `json:"display_name"`
	Role        models.Role `json:"role"`
	JoinedAt    time.Time   `json:"joined_at"`
}

// ListMembers returns all members of a wedding with their user info.
func (r *WeddingRepo) ListMembers(ctx context.Context, weddingID uuid.UUID) ([]MembershipView, error) {
	var out []MembershipView
	err := r.db.WithContext(ctx).
		Table("memberships m").
		Select("m.user_id, u.username, u.display_name, m.role, m.joined_at").
		Joins("JOIN users u ON u.id = m.user_id").
		Where("m.wedding_id = ?", weddingID).
		Order("m.joined_at ASC").
		Scan(&out).Error
	return out, err
}

// UpsertMembership creates or updates a user's membership+role on a wedding.
func (r *WeddingRepo) UpsertMembership(ctx context.Context, weddingID, userID uuid.UUID, role models.Role) error {
	var m models.Membership
	err := r.db.WithContext(ctx).Where("wedding_id = ? AND user_id = ?", weddingID, userID).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(&models.Membership{
			WeddingID: weddingID, UserID: userID, Role: role, JoinedAt: time.Now(),
		}).Error
	}
	if err != nil {
		return err
	}
	m.Role = role
	return r.db.WithContext(ctx).Save(&m).Error
}

// SetMemberRole changes an existing member's role, or ErrNotFound.
func (r *WeddingRepo) SetMemberRole(ctx context.Context, weddingID, userID uuid.UUID, role models.Role) error {
	res := r.db.WithContext(ctx).Model(&models.Membership{}).
		Where("wedding_id = ? AND user_id = ?", weddingID, userID).
		Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveMember deletes a membership, or ErrNotFound.
func (r *WeddingRepo) RemoveMember(ctx context.Context, weddingID, userID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Where("wedding_id = ? AND user_id = ?", weddingID, userID).
		Delete(&models.Membership{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByRole counts members of a wedding holding a given role.
func (r *WeddingRepo) CountByRole(ctx context.Context, weddingID uuid.UUID, role models.Role) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.Membership{}).
		Where("wedding_id = ? AND role = ?", weddingID, role).Count(&n).Error
	return n, err
}

// ─── Invite links ───────────────────────────────────────────────────────────

// CreateLink inserts an invite link.
func (r *WeddingRepo) CreateLink(ctx context.Context, l *models.InviteLink) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// ListLinks returns a wedding's non-revoked invite links.
func (r *WeddingRepo) ListLinks(ctx context.Context, weddingID uuid.UUID) ([]models.InviteLink, error) {
	var links []models.InviteLink
	err := r.db.WithContext(ctx).
		Where("wedding_id = ? AND revoked = false", weddingID).
		Order("created_at DESC").Find(&links).Error
	return links, err
}

// LinkByToken loads a link by its token, or ErrNotFound.
func (r *WeddingRepo) LinkByToken(ctx context.Context, tok string) (*models.InviteLink, error) {
	var l models.InviteLink
	err := r.db.WithContext(ctx).First(&l, "token = ?", tok).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// RevokeLink marks a link revoked, scoped to its wedding, or ErrNotFound.
func (r *WeddingRepo) RevokeLink(ctx context.Context, weddingID, linkID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&models.InviteLink{}).
		Where("id = ? AND wedding_id = ?", linkID, weddingID).
		Update("revoked", true)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
