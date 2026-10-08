package test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoginLockout(t *testing.T) {
	e := newApp(t)
	register(t, e, "sarah") // Password123!, harness maxAttempts=5

	// 5 wrong attempts trip the lockout.
	for i := 0; i < 5; i++ {
		if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": "sarah", "password": "wrongpassword",
		}); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i+1, code)
		}
	}
	// 6th attempt with the CORRECT password is now locked (423).
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("after lockout: want 423, got %d", code)
	}

	// Unknown username still returns generic 401 (no enumeration).
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "ghost", "password": "whatever1",
	}); code != http.StatusUnauthorized {
		t.Fatalf("unknown user: want 401, got %d", code)
	}
}

func TestRecoverLockout(t *testing.T) {
	e := newApp(t)
	// Register directly to capture the recovery code.
	_, body := do(t, e, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"username": "sarah", "display_name": "Sarah", "password": "Password123!",
	})
	recovery, _ := dataOf(body)["recovery_code"].(string)

	// 5 wrong recovery codes trip the lockout.
	for i := 0; i < 5; i++ {
		if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/recover", "", map[string]any{
			"username": "sarah", "recovery_code": "WRONGWRONGWRONG", "new_password": "NewPassword1!",
		}); code != http.StatusUnauthorized {
			t.Fatalf("recover attempt %d: want 401, got %d", i+1, code)
		}
	}
	// 6th attempt with the CORRECT code is now locked (423).
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/recover", "", map[string]any{
		"username": "sarah", "recovery_code": recovery, "new_password": "NewPassword1!",
	}); code != http.StatusLocked {
		t.Fatalf("recover after lockout: want 423, got %d", code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	e := newApp(t)
	rec := serve(e, newReq(http.MethodGet, "/healthz", "", nil))
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: want nosniff, got %q", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options: want DENY, got %q", got)
	}
}

// A stolen access token must not turn PATCH /me/password into an unlimited
// password oracle: wrong current passwords share the login lockout.
func TestChangePasswordLockout(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah") // Password123!, harness maxAttempts=5

	for i := 0; i < 5; i++ {
		if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
			"old_password": "wrongpassword", "new_password": "NewPassword1!",
		}); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i+1, code)
		}
	}
	// Locked: even the correct current password is refused...
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "Password123!", "new_password": "NewPassword1!",
	}); code != http.StatusLocked {
		t.Fatalf("change after lockout: want 423, got %d", code)
	}
	// ...and so is login.
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("login after change-password lockout: want 423, got %d", code)
	}
}

func wrongLogins(t *testing.T, e *gin.Engine, username string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"username": username, "password": "wrongpassword",
		}); code != http.StatusUnauthorized {
			t.Fatalf("wrong login %d: want 401, got %d", i+1, code)
		}
	}
}

// A successful login clears earlier failures, so they don't add up across
// separate typos (harness maxAttempts=5).
func TestSuccessfulLoginResetsFailureCount(t *testing.T) {
	e := newApp(t)
	register(t, e, "sarah")
	wrongLogins(t, e, "sarah", 4)
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusOK {
		t.Fatalf("correct login: want 200, got %d", code)
	}
	wrongLogins(t, e, "sarah", 4) // would lock on the 1st if the count carried over
}

// A lock lifts by itself once locked_until has passed.
func TestLockoutExpires(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	register(t, e, "sarah")
	wrongLogins(t, e, "sarah", 5)
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusLocked {
		t.Fatalf("while locked: want 423, got %d", code)
	}
	db.Exec("UPDATE users SET locked_until = now() - interval '1 second' WHERE username = 'sarah'")
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusOK {
		t.Fatalf("after the lock expired: want 200, got %d", code)
	}
}

// Proving the current password by changing it also clears earlier failures.
func TestSuccessfulPasswordChangeResetsFailureCount(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")
	wrongLogins(t, e, "sarah", 4)
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "Password123!", "new_password": "NewPassword1!",
	}); code != http.StatusOK {
		t.Fatalf("change password: want 200, got %d", code)
	}
	wrongLogins(t, e, "sarah", 4)
}
