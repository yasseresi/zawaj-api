package test

import (
	"context"
	"errors"
	"net/http"
	"sync"
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

// Parallel wrong guesses must all count: a read-modify-write counter loses
// increments, letting an attacker exceed maxAttempts without ever locking.
func TestParallelFailedLoginsStillLock(t *testing.T) {
	e := newApp(t)
	register(t, e, "sarah") // harness maxAttempts=5

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(e, newReq(http.MethodPost, "/api/v1/auth/login", "", map[string]any{
				"username": "sarah", "password": "wrongpassword",
			}))
		}()
	}
	wg.Wait()

	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("after 5 parallel failures: want 423, got %d", code)
	}
}

// A profile/settings write built from a user loaded before the lock landed
// must not write the stale (unlocked) lockout state back.
func TestStaleUserWriteKeepsLock(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	_, id := register(t, e, "sarah")
	ctx := context.Background()
	users := repository.NewUserRepo(db)

	stale, err := users.ByID(ctx, uuid.MustParse(id))
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	for i := 0; i < 5; i++ {
		do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "sarah", "password": "wrongpassword",
		})
	}
	stale.DisplayName = "Sara"
	if err := users.Update(ctx, stale); err != nil {
		t.Fatalf("update: %v", err)
	}

	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("stale write cleared the lock: want 423, got %d", code)
	}
}
