package test

import (
	"net/http"
	"testing"
)

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	e := newApp(t)
	code, body := do(t, e, "POST", "/api/v1/auth/register", "", map[string]any{
		"username": "rot_user", "display_name": "Rot", "password": "Password123!",
	})
	if code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d (%v)", code, body)
	}
	r1, _ := dataOf(body)["refresh"].(string)
	if r1 == "" {
		t.Fatal("no refresh token in register response")
	}

	// Rotate: r1 -> r2.
	code, body = do(t, e, "POST", "/api/v1/auth/refresh", "", map[string]any{"refresh": r1})
	if code != http.StatusOK {
		t.Fatalf("refresh: want 200, got %d (%v)", code, body)
	}
	r2, _ := dataOf(body)["refresh"].(string)
	if r2 == "" || r2 == r1 {
		t.Fatalf("expected a new, different refresh token; got %q", r2)
	}

	// Reuse the already-rotated r1 -> 401 (reuse detected).
	if code, _ := do(t, e, "POST", "/api/v1/auth/refresh", "", map[string]any{"refresh": r1}); code != http.StatusUnauthorized {
		t.Fatalf("reuse of r1: want 401, got %d", code)
	}

	// Reuse detection kills the whole family: r2 is now revoked too.
	if code, _ := do(t, e, "POST", "/api/v1/auth/refresh", "", map[string]any{"refresh": r2}); code != http.StatusUnauthorized {
		t.Fatalf("r2 after reuse-detection: want 401, got %d", code)
	}
}

func TestLogoutRevokesRefresh(t *testing.T) {
	e := newApp(t)
	code, body := do(t, e, "POST", "/api/v1/auth/register", "", map[string]any{
		"username": "logout_user", "display_name": "Lo", "password": "Password123!",
	})
	if code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", code)
	}
	r1, _ := dataOf(body)["refresh"].(string)

	if code, _ := do(t, e, "POST", "/api/v1/auth/logout", "", map[string]any{"refresh": r1}); code != http.StatusOK {
		t.Fatalf("logout: want 200, got %d", code)
	}
	if code, _ := do(t, e, "POST", "/api/v1/auth/refresh", "", map[string]any{"refresh": r1}); code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: want 401, got %d", code)
	}
}

func TestChangePasswordRevokesSessions(t *testing.T) {
	e := newApp(t)
	code, body := do(t, e, "POST", "/api/v1/auth/register", "", map[string]any{
		"username": "cp_user", "display_name": "Cp", "password": "Password123!",
	})
	if code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", code)
	}
	access, _ := dataOf(body)["access"].(string)
	r1, _ := dataOf(body)["refresh"].(string)

	if code, _ := do(t, e, "PATCH", "/api/v1/me/password", access, map[string]any{
		"old_password": "Password123!", "new_password": "NewPassword123!",
	}); code != http.StatusOK {
		t.Fatalf("change password: want 200, got %d", code)
	}
	// Existing refresh token must be revoked after a password change.
	if code, _ := do(t, e, "POST", "/api/v1/auth/refresh", "", map[string]any{"refresh": r1}); code != http.StatusUnauthorized {
		t.Fatalf("refresh after password change: want 401, got %d", code)
	}
}
