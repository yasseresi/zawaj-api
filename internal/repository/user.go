// Package repository holds GORM data access, one file per aggregate. Services
// depend on these; handlers never import GORM directly.
package repository

import (
	"context"
	"errors"
	"time"

	"zawaj/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a row does not exist. Services translate this to
// the appropriate apperr.
var ErrNotFound = errors.New("record not found")

// UserRepo is the users data access.
type UserRepo struct{ db *gorm.DB }

// NewUserRepo builds the repo.
func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

// Create inserts a new user.
func (r *UserRepo) Create(ctx context.Context, u *models.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// ByUsername loads a user by username, or ErrNotFound.
func (r *UserRepo) ByUsername(ctx context.Context, username string) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ByID loads a user by id, or ErrNotFound.
func (r *UserRepo) ByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).First(&u, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// ExistsByUsername reports whether a username is taken.
func (r *UserRepo) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.User{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

// UpdateColumns writes only the given columns of an existing user, or returns
// ErrNotFound if the row is gone. Callers name exactly the columns they
// changed, so overlapping writes (two toggles, a profile edit during a
// password change) can't revert each other, and an UPDATE never falls back to
// INSERT, so a write racing an account deletion can't resurrect the user.
// Lockout state has its own atomic writers (RecordFailure / ClearFailures).
func (r *UserRepo) UpdateColumns(ctx context.Context, userID uuid.UUID, cols map[string]any) error {
	if len(cols) == 0 {
		return nil
	}
	res := r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(cols)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordFailure atomically counts one failed credential check. The attempt
// that reaches maxAttempts locks the account until lockedUntil and resets the
// counter. A single UPDATE (row-locked by Postgres) means parallel guesses
// can't lose increments.
func (r *UserRepo) RecordFailure(ctx context.Context, userID uuid.UUID, maxAttempts int, lockedUntil time.Time) error {
	return r.db.WithContext(ctx).Exec(`UPDATE users SET
		locked_until    = CASE WHEN failed_attempts + 1 >= ? THEN ? ELSE locked_until END,
		failed_attempts = CASE WHEN failed_attempts + 1 >= ? THEN 0 ELSE failed_attempts + 1 END
		WHERE id = ?`, maxAttempts, lockedUntil, maxAttempts, userID).Error
}

// ClearFailures resets the failed-attempt counter and lifts any lock.
func (r *UserRepo) ClearFailures(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).Exec(
		"UPDATE users SET failed_attempts = 0, locked_until = NULL WHERE id = ?", userID).Error
}

// DeleteWithOwnedData hard-deletes a user and cascade-deletes every wedding they
// own (guests, notes, activity, invite links, memberships), removes their
// memberships in others' weddings, and their notifications. Rows they authored
// in other people's weddings (added_by/author_id/actor_id) are left as harmless
// orphan uuids. Runs in one transaction.
func (r *UserRepo) DeleteWithOwnedData(ctx context.Context, userID uuid.UUID) error {
	const owned = "SELECT id FROM weddings WHERE owner_id = ?"
	stmts := []string{
		"DELETE FROM guest_notes WHERE guest_id IN (SELECT id FROM guests WHERE wedding_id IN (" + owned + "))",
		"DELETE FROM guests WHERE wedding_id IN (" + owned + ")",
		"DELETE FROM activity_logs WHERE wedding_id IN (" + owned + ")",
		"DELETE FROM invite_links WHERE wedding_id IN (" + owned + ")",
		"DELETE FROM memberships WHERE wedding_id IN (" + owned + ")",
		"DELETE FROM weddings WHERE owner_id = ?",
		"DELETE FROM memberships WHERE user_id = ?",
		"DELETE FROM notifications WHERE user_id = ?",
		"DELETE FROM device_tokens WHERE user_id = ?",
		"DELETE FROM refresh_tokens WHERE user_id = ?",
		"DELETE FROM idempotency_keys WHERE user_id = ?",
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, q := range stmts {
			if err := tx.Exec(q, userID).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&models.User{}, "id = ?", userID).Error
	})
}
