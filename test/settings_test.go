package test

import (
	"net/http"
	"testing"
)

func TestAccountSettings(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")

	// Wrong current password -> 401.
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "wrongpassword", "new_password": "newpassword1",
	}); code != http.StatusUnauthorized {
		t.Fatalf("change pw wrong old: want 401, got %d", code)
	}

	// Correct change -> 200, then new password logs in and old does not.
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "password123", "new_password": "newpassword1",
	}); code != http.StatusOK {
		t.Fatalf("change pw: want 200, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "newpassword1",
	}); code != http.StatusOK {
		t.Fatalf("login new pw: want 200, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "password123",
	}); code != http.StatusUnauthorized {
		t.Fatalf("login old pw: want 401, got %d", code)
	}

	// Settings update returns the new preference values.
	_, body := do(t, e, http.MethodPatch, "/api/v1/me/settings", tok, map[string]any{
		"dark_mode": true, "notif_rsvp": false,
	})
	d := dataOf(body)
	if d["dark_mode"] != true || d["notif_rsvp"] != false {
		t.Fatalf("settings not applied: %v", d)
	}
}
