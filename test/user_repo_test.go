package test

import (
	"context"
	"errors"
	"testing"

	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
)

// A write that races an account deletion must not bring the user back: GORM's
// Save falls back to INSERT when the UPDATE matches no row.
func TestUserUpdateDoesNotResurrectDeletedUser(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	_, id := register(t, e, "sarah")
	ctx := context.Background()
	users := repository.NewUserRepo(db)

	u, err := users.ByID(ctx, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if err := users.DeleteWithOwnedData(ctx, u.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	u.DisplayName = "stale write"
	if err := users.Update(ctx, u); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("update of deleted user: want ErrNotFound, got %v", err)
	}
	var n int64
	db.Model(&models.User{}).Where("id = ?", u.ID).Count(&n)
	if n != 0 {
		t.Fatalf("deleted user was re-inserted (%d rows)", n)
	}
}
