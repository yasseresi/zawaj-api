package test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"zawaj/internal/models"
	"zawaj/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
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

	if err := users.UpdateColumns(ctx, u.ID, map[string]any{"display_name": "stale write"}); !errors.Is(err, repository.ErrNotFound) {
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

// A profile/settings write made after a lock landed must not lift it.
func TestStaleUserWriteKeepsLock(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	_, id := register(t, e, "sarah")
	ctx := context.Background()
	users := repository.NewUserRepo(db)

	for i := 0; i < 5; i++ {
		do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "sarah", "password": "wrongpassword",
		})
	}
	if err := users.UpdateColumns(ctx, uuid.MustParse(id), map[string]any{"display_name": "Sara"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("stale write cleared the lock: want 423, got %d", code)
	}
}

// ChangePassword spends two bcrypt operations between loading the user and
// writing it. A profile edit landing in that window must survive.
func TestChangePasswordKeepsConcurrentProfileEdit(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	tok, _ := register(t, e, "sarah")

	// Hold the user row with an uncommitted profile edit, so ChangePassword
	// reads the old row and then blocks on its write until we commit — the
	// exact interleaving a whole-row write would get wrong, made deterministic.
	edit := db.Begin()
	if err := edit.Exec("UPDATE users SET display_name = 'Sara' WHERE username = 'sarah'").Error; err != nil {
		t.Fatalf("profile edit: %v", err)
	}
	done := make(chan int, 1)
	go func() {
		code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
			"old_password": "Password123!", "new_password": "NewPassword1!",
		})
		done <- code
	}()
	waitForLockWait(t, db)
	if err := edit.Commit().Error; err != nil {
		t.Fatalf("commit profile edit: %v", err)
	}
	if code := <-done; code != http.StatusOK {
		t.Fatalf("change password: want 200, got %d", code)
	}

	_, body := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "NewPassword1!",
	})
	user, _ := dataOf(body)["user"].(map[string]any)
	if got := user["display_name"]; got != "Sara" {
		t.Fatalf("display_name after overlapping writes: want Sara, got %v", got)
	}
}

// waitForLockWait blocks until some session is waiting on a row lock (the
// write we're holding up), failing after 10s.
func waitForLockWait(t *testing.T, db *gorm.DB) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int64
		db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'").Scan(&n)
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no session started waiting on the held row lock")
}

// Two preference toggles in flight at once (the app allows it) must both stick.
func TestOverlappingSettingsWritesBothApply(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")

	for round := 0; round < 20; round++ {
		do(t, e, http.MethodPatch, "/api/v1/me/settings", tok, map[string]any{
			"notif_push": true, "notif_rsvp": true,
		})
		var wg sync.WaitGroup
		for _, field := range []string{"notif_push", "notif_rsvp"} {
			wg.Add(1)
			go func(field string) {
				defer wg.Done()
				serve(e, newReq(http.MethodPatch, "/api/v1/me/settings", tok, map[string]any{field: false}))
			}(field)
		}
		wg.Wait()

		_, body := do(t, e, http.MethodGet, "/api/v1/me", tok, nil)
		me := dataOf(body)
		if me["notif_push"] != false || me["notif_rsvp"] != false {
			t.Fatalf("round %d: a toggle was lost: push=%v rsvp=%v", round, me["notif_push"], me["notif_rsvp"])
		}
	}
}
