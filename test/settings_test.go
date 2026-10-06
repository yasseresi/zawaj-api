package test

import (
	"net/http"
	"testing"
)

func TestDeleteAccount(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/guests", sarah, map[string]any{"full_name": "A"})
	joinAs(t, e, sarah, wid, "editor", omar) // omar collaborates

	// Deletion requires the current password.
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/me", sarah, nil); code != http.StatusBadRequest {
		t.Fatalf("delete without password: want 400, got %d", code)
	}
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/me", sarah, map[string]any{"password": "wrongpassword"}); code != http.StatusUnauthorized {
		t.Fatalf("delete with wrong password: want 401, got %d", code)
	}
	if code, _ := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid, omar, nil); code != http.StatusOK {
		t.Fatalf("wedding must survive a rejected delete: want 200, got %d", code)
	}

	// Sarah deletes her account.
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/me", sarah, map[string]any{"password": "Password123!"}); code != http.StatusOK {
		t.Fatalf("delete account: want 200, got %d", code)
	}
	// Her login no longer works.
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
	}); code != http.StatusUnauthorized {
		t.Fatalf("login after delete: want 401, got %d", code)
	}
	// Her owned wedding is gone — omar (former editor) now gets 404.
	if code, _ := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid, omar, nil); code != http.StatusNotFound {
		t.Fatalf("wedding after owner delete: want 404, got %d", code)
	}
	// Omar's own account is untouched.
	if code, _ := do(t, e, http.MethodGet, "/api/v1/me", omar, nil); code != http.StatusOK {
		t.Fatalf("collaborator account: want 200, got %d", code)
	}
}

func TestAccountSettings(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")

	// Wrong current password -> 401.
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "wrongpassword", "new_password": "NewPassword1!",
	}); code != http.StatusUnauthorized {
		t.Fatalf("change pw wrong old: want 401, got %d", code)
	}

	// Correct change -> 200, then new password logs in and old does not.
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/password", tok, map[string]any{
		"old_password": "Password123!", "new_password": "NewPassword1!",
	}); code != http.StatusOK {
		t.Fatalf("change pw: want 200, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "NewPassword1!",
	}); code != http.StatusOK {
		t.Fatalf("login new pw: want 200, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"username": "sarah", "password": "Password123!",
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

func TestProfilePhone(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")

	// International input is normalized to the local Algerian format.
	code, body := do(t, e, http.MethodPatch, "/api/v1/me", tok, map[string]any{"phone": "+213 672 85 99 65"})
	if code != http.StatusOK || dataOf(body)["phone"] != "0672859965" {
		t.Fatalf("set phone: want 200 + 0672859965, got %d %v", code, body)
	}

	// Invalid numbers are rejected and leave the stored phone unchanged.
	for _, bad := range []string{"0812345678", "0551"} {
		if code, _ := do(t, e, http.MethodPatch, "/api/v1/me", tok, map[string]any{"phone": bad}); code != http.StatusBadRequest {
			t.Fatalf("phone %q: want 400, got %d", bad, code)
		}
	}

	// GET /me exposes phone + notification preferences, never email.
	_, body = do(t, e, http.MethodGet, "/api/v1/me", tok, nil)
	me := dataOf(body)
	if me["phone"] != "0672859965" || me["notif_push"] != true || me["notif_rsvp"] != true {
		t.Fatalf("GET /me: unexpected %v", me)
	}
	if _, ok := me["email"]; ok {
		t.Fatalf("GET /me must not expose email: %v", me)
	}

	// Settings response uses the same public shape and is read back by GET /me.
	_, body = do(t, e, http.MethodPatch, "/api/v1/me/settings", tok, map[string]any{"notif_push": false})
	if d := dataOf(body); d["notif_push"] != false || d["phone"] != "0672859965" {
		t.Fatalf("settings response: unexpected %v", d)
	}
	if _, ok := dataOf(body)["email"]; ok {
		t.Fatalf("settings response must not expose email")
	}
	_, body = do(t, e, http.MethodGet, "/api/v1/me", tok, nil)
	if dataOf(body)["notif_push"] != false {
		t.Fatalf("notif_push not persisted: %v", body)
	}

	// Empty string clears the phone.
	_, body = do(t, e, http.MethodPatch, "/api/v1/me", tok, map[string]any{"phone": ""})
	if _, ok := dataOf(body)["phone"]; ok {
		t.Fatalf("clear phone: want no phone key, got %v", body)
	}
}

// Wrong passwords on DELETE /me share the login lockout counter, so the
// endpoint cannot be used to brute-force a password.
func TestDeleteAccountLockout(t *testing.T) {
	e := newApp(t)
	tok, _ := register(t, e, "sarah")
	locked := false
	for i := 0; i < 20 && !locked; i++ {
		code, _ := do(t, e, http.MethodDelete, "/api/v1/me", tok, map[string]any{"password": "wrongpassword"})
		switch code {
		case http.StatusUnauthorized:
		case http.StatusLocked:
			locked = true
		default:
			t.Fatalf("attempt %d: want 401 or 423, got %d", i, code)
		}
	}
	if !locked {
		t.Fatal("repeated wrong passwords never locked the account")
	}
	// Even the correct password is refused while locked, and the account survives.
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/me", tok, map[string]any{"password": "Password123!"}); code != http.StatusLocked {
		t.Fatalf("delete while locked: want 423, got %d", code)
	}
	if code, _ := do(t, e, http.MethodGet, "/api/v1/me", tok, nil); code != http.StatusOK {
		t.Fatalf("account must survive: want 200, got %d", code)
	}
}
