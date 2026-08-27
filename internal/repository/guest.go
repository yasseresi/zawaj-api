package repository

import (
	"context"
	"errors"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GuestRepo is data access for guests and their notes, always wedding-scoped.
type GuestRepo struct{ db *gorm.DB }

// NewGuestRepo builds the repo.
func NewGuestRepo(db *gorm.DB) *GuestRepo { return &GuestRepo{db: db} }

// GuestFilter narrows a guest listing.
type GuestFilter struct {
	Status string // "" = all
	Query  string // ILIKE on full_name
	Limit  int
	Offset int
}

// StatusCounts are the tab counts on the guest list screen.
type StatusCounts struct {
	All       int64 `json:"all"`
	Confirmed int64 `json:"confirmed"`
	Pending   int64 `json:"pending"`
	Declined  int64 `json:"declined"`
}

// List returns filtered guests for a wedding, newest first.
func (r *GuestRepo) List(ctx context.Context, weddingID uuid.UUID, f GuestFilter) ([]models.Guest, error) {
	q := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Query != "" {
		q = q.Where("full_name ILIKE ?", "%"+f.Query+"%")
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	var out []models.Guest
	err := q.Order("created_at DESC").Limit(f.Limit).Offset(f.Offset).Find(&out).Error
	return out, err
}

// Counts returns the per-status tab counts for a wedding.
func (r *GuestRepo) Counts(ctx context.Context, weddingID uuid.UUID) (StatusCounts, error) {
	type row struct {
		Status string
		N      int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&models.Guest{}).
		Select("status, count(*) AS n").
		Where("wedding_id = ?", weddingID).
		Group("status").Scan(&rows).Error
	if err != nil {
		return StatusCounts{}, err
	}
	c := StatusCounts{}
	for _, r := range rows {
		switch models.RSVPStatus(r.Status) {
		case models.StatusConfirmed:
			c.Confirmed = r.N
		case models.StatusPending:
			c.Pending = r.N
		case models.StatusDeclined:
			c.Declined = r.N
		}
		c.All += r.N
	}
	return c, nil
}

// Create inserts a guest.
func (r *GuestRepo) Create(ctx context.Context, g *models.Guest) error {
	return r.db.WithContext(ctx).Create(g).Error
}

// ByID loads a guest scoped to its wedding, or ErrNotFound.
func (r *GuestRepo) ByID(ctx context.Context, weddingID, guestID uuid.UUID) (*models.Guest, error) {
	var g models.Guest
	err := r.db.WithContext(ctx).
		Where("id = ? AND wedding_id = ?", guestID, weddingID).First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// Update persists changes to a guest.
func (r *GuestRepo) Update(ctx context.Context, g *models.Guest) error {
	return r.db.WithContext(ctx).Save(g).Error
}

// Delete removes a guest and its notes, scoped to its wedding, or ErrNotFound.
func (r *GuestRepo) Delete(ctx context.Context, weddingID, guestID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? AND wedding_id = ?", guestID, weddingID).Delete(&models.Guest{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return tx.Where("guest_id = ?", guestID).Delete(&models.GuestNote{}).Error
	})
}

// AddNote inserts a note on a guest.
func (r *GuestRepo) AddNote(ctx context.Context, n *models.GuestNote) error {
	return r.db.WithContext(ctx).Create(n).Error
}

// ListNotes returns a guest's notes, newest first.
func (r *GuestRepo) ListNotes(ctx context.Context, guestID uuid.UUID) ([]models.GuestNote, error) {
	var out []models.GuestNote
	err := r.db.WithContext(ctx).Where("guest_id = ?", guestID).
		Order("created_at DESC").Find(&out).Error
	return out, err
}
