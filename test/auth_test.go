package test

import (
	"net/http"
	"testing"
)

func TestAuthFlow(t *testing.T) {
	e := newApp(t)

	// Register
	code, body := do(t, e, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"username": "ahmed", "display_name": "Ahmed", "password": "correcthorse",
	})
	if code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d (%v)", code, body)
	}
	data := dataOf(body)
	if data["recovery_code"] == "" || data["recovery_code"] == nil {
		t.Fatal("register: expected a recovery_code")
	}
	access, _ := data["access"].(string)
	refresh, _ := data["refresh"].(string)
	recovery, _ := data["recovery_code"].(string)

	// /me with token
	if code, _ := do(t, e, http.MethodGet, "/api/v1/me", access, nil); code != http.StatusOK {
		t.Fatalf("me: want 200, got %d", code)
	}
	// /me without token -> 401
	if code, _ := do(t, e, http.MethodGet, "/api/v1/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("me no-token: want 401, got %d", code)
	}

	// Duplicate username -> 409
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"username": "ahmed", "display_name": "X", "password": "anotherpass",
	}); code != http.StatusConflict {
		t.Fatalf("dup register: want 409, got %d", code)
	}

	// Too-short password -> 400
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"username": "zed", "display_name": "Z", "password": "short",
	}); code != http.StatusBadRequest {
		t.Fatalf("short password: want 400, got %d", code)
	}

	// Wrong password -> 401
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "ahmed", "password": "wrongpassword",
	}); code != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401, got %d", code)
	}

	// Refresh -> 200
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{
		"refresh": refresh,
	}); code != http.StatusOK {
		t.Fatalf("refresh: want 200, got %d", code)
	}

	// Recover -> 200, then login with new password
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/recover", "", map[string]any{
		"username": "ahmed", "recovery_code": recovery, "new_password": "brandnewpass",
	}); code != http.StatusOK {
		t.Fatalf("recover: want 200, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "ahmed", "password": "brandnewpass",
	}); code != http.StatusOK {
		t.Fatalf("login new password: want 200, got %d", code)
	}
}
