package test

import (
	"net/http"
	"testing"
)

func TestLoginLockout(t *testing.T) {
	e := newApp(t)
	register(t, e, "sarah") // password123, harness maxAttempts=5

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
		"username": "sarah", "password": "password123",
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
		"username": "sarah", "display_name": "Sarah", "password": "password123",
	})
	recovery, _ := dataOf(body)["recovery_code"].(string)

	// 5 wrong recovery codes trip the lockout.
	for i := 0; i < 5; i++ {
		if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/recover", "", map[string]any{
			"username": "sarah", "recovery_code": "WRONGWRONGWRONG", "new_password": "newpassword1",
		}); code != http.StatusUnauthorized {
			t.Fatalf("recover attempt %d: want 401, got %d", i+1, code)
		}
	}
	// 6th attempt with the CORRECT code is now locked (423).
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/recover", "", map[string]any{
		"username": "sarah", "recovery_code": recovery, "new_password": "newpassword1",
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
