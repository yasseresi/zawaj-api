package test

import (
	"net/http"
	"testing"
)

// Accepting a lower-role link must not downgrade an existing higher role.
func TestInviteNoDowngrade(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")

	joinAs(t, e, sarah, wid, "editor", omar) // omar becomes editor

	// omar opens a viewer link — should stay editor, not drop to viewer.
	_, lb := do(t, e, "POST", "/api/v1/weddings/"+wid+"/invite-links", sarah, map[string]any{"role": "viewer"})
	tok, _ := dataOf(lb)["token"].(string)
	_, ab := do(t, e, "POST", "/api/v1/invite/"+tok+"/accept", omar, nil)
	if dataOf(ab)["role"] != "editor" {
		t.Fatalf("accept viewer link as editor: want role editor kept, got %v", dataOf(ab)["role"])
	}
	// Confirm via /me role on the wedding.
	_, wb := do(t, e, "GET", "/api/v1/weddings/"+wid, omar, nil)
	if dataOf(wb)["my_role"] != "editor" {
		t.Fatalf("wedding my_role: want editor, got %v", dataOf(wb)["my_role"])
	}
}

func TestWeddingAndRoles(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	zed, _ := register(t, e, "zed")

	// Owner creates a wedding.
	code, body := do(t, e, http.MethodPost, "/api/v1/weddings", sarah, map[string]any{
		"name": "Layla & Omar", "event_date": "2026-06-12", "description": "t",
	})
	if code != http.StatusCreated {
		t.Fatalf("create wedding: want 201, got %d (%v)", code, body)
	}
	wid, _ := dataOf(body)["id"].(string)

	// List shows owner role + zero guests.
	_, lb := do(t, e, http.MethodGet, "/api/v1/weddings", sarah, nil)
	items, _ := dataOf(lb)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list: want 1 wedding, got %d", len(items))
	}
	if first, _ := items[0].(map[string]any); first["my_role"] != "owner" {
		t.Fatalf("list: want role owner, got %v", first["my_role"])
	}

	// Owner mints an editor invite link.
	_, lkb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", sarah, map[string]any{"role": "editor"})
	tok, _ := dataOf(lkb)["token"].(string)
	if tok == "" {
		t.Fatal("invite-link: empty token")
	}

	// Omar previews then accepts -> becomes editor.
	if code, _ := do(t, e, http.MethodGet, "/api/v1/invite/"+tok, omar, nil); code != http.StatusOK {
		t.Fatalf("preview: want 200, got %d", code)
	}
	_, ab := do(t, e, http.MethodPost, "/api/v1/invite/"+tok+"/accept", omar, nil)
	if dataOf(ab)["role"] != "editor" {
		t.Fatalf("accept: want role editor, got %v", dataOf(ab)["role"])
	}

	// Role enforcement matrix.
	cases := []struct {
		name              string
		method, path, tok string
		body              any
		want              int
	}{
		{"editor reads", http.MethodGet, "/api/v1/weddings/" + wid, omar, nil, http.StatusOK},
		{"editor cant patch", http.MethodPatch, "/api/v1/weddings/" + wid, omar, map[string]any{"name": "x"}, http.StatusForbidden},
		{"nonmember 404", http.MethodGet, "/api/v1/weddings/" + wid, zed, nil, http.StatusNotFound},
		{"owner sets role", http.MethodPatch, "/api/v1/weddings/" + wid + "/members/" + omarID, sarah, map[string]any{"role": "viewer"}, http.StatusOK},
		{"viewer cant delete", http.MethodDelete, "/api/v1/weddings/" + wid, omar, nil, http.StatusForbidden},
		{"owner deletes", http.MethodDelete, "/api/v1/weddings/" + wid, sarah, nil, http.StatusOK},
	}
	for _, tc := range cases {
		if code, b := do(t, e, tc.method, tc.path, tc.tok, tc.body); code != tc.want {
			t.Errorf("%s: want %d, got %d (%v)", tc.name, tc.want, code, b)
		}
	}
}

// Transferring ownership promotes the target member to owner and demotes the
// previous owner to editor; guards reject self-transfer, non-members, and
// non-owner callers.
func TestTransferOwnership(t *testing.T) {
	e := newApp(t)
	sarah, sarahID := register(t, e, "sarah_t")
	omar, omarID := register(t, e, "omar_t")
	nour, nourID := register(t, e, "nour_t")
	wid := createWedding(t, e, sarah, "Transfer")

	joinAs(t, e, sarah, wid, "editor", omar) // omar is an editor member

	// Non-owner cannot transfer.
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", omar, map[string]any{"user_id": sarahID}); code != http.StatusForbidden {
		t.Fatalf("editor transfer: want 403, got %d", code)
	}
	// Cannot transfer to self.
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", sarah, map[string]any{"user_id": sarahID}); code != http.StatusBadRequest {
		t.Fatalf("self transfer: want 400, got %d", code)
	}
	// Cannot transfer to a non-member.
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", sarah, map[string]any{"user_id": nourID}); code != http.StatusNotFound {
		t.Fatalf("non-member transfer: want 404, got %d", code)
	}
	_ = nour

	// Valid transfer to omar.
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", sarah, map[string]any{"user_id": omarID}); code != http.StatusOK {
		t.Fatalf("transfer: want 200, got %d", code)
	}
	// omar is now owner.
	if _, wb := do(t, e, "GET", "/api/v1/weddings/"+wid, omar, nil); dataOf(wb)["my_role"] != "owner" {
		t.Fatalf("new owner role: want owner, got %v", dataOf(wb)["my_role"])
	}
	// sarah is now editor (demoted), and can no longer transfer.
	if _, wb := do(t, e, "GET", "/api/v1/weddings/"+wid, sarah, nil); dataOf(wb)["my_role"] != "editor" {
		t.Fatalf("old owner role: want editor, got %v", dataOf(wb)["my_role"])
	}
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", sarah, map[string]any{"user_id": omarID}); code != http.StatusForbidden {
		t.Fatalf("demoted owner transfer: want 403, got %d", code)
	}
}

// Device registration: register upserts a token, unregister removes it, and
// unregistering an unknown token 404s. (Push delivery uses the no-op sender in
// tests, so no external calls occur.)
func TestDeviceRegistration(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah_d")

	if code, _ := do(t, e, "POST", "/api/v1/me/devices", sarah, map[string]any{
		"token": "fcm-token-abc", "platform": "android",
	}); code != http.StatusCreated {
		t.Fatalf("register device: want 201, got %d", code)
	}
	if code, _ := do(t, e, "POST", "/api/v1/me/devices", sarah, map[string]any{
		"token": "fcm-token-abc", "platform": "ios",
	}); code != http.StatusCreated {
		t.Fatalf("re-register device: want 201, got %d", code)
	}
	if code, _ := do(t, e, "POST", "/api/v1/me/devices", sarah, map[string]any{
		"token": "x", "platform": "nokia",
	}); code != http.StatusBadRequest {
		t.Fatalf("bad platform: want 400, got %d", code)
	}
	if code, _ := do(t, e, "DELETE", "/api/v1/me/devices", sarah, map[string]any{"token": "fcm-token-abc"}); code != http.StatusOK {
		t.Fatalf("unregister device: want 200, got %d", code)
	}
	if code, _ := do(t, e, "DELETE", "/api/v1/me/devices", sarah, map[string]any{"token": "fcm-token-abc"}); code != http.StatusNotFound {
		t.Fatalf("unregister missing: want 404, got %d", code)
	}
	if code, _ := do(t, e, "POST", "/api/v1/me/devices", "", map[string]any{"token": "y", "platform": "web"}); code != http.StatusUnauthorized {
		t.Fatalf("unauth register: want 401, got %d", code)
	}
}

// Owner role decisions stick: a demoted member can't re-open the editor link
// they joined with to get editor back. Links only ever create memberships.
func TestInviteLinkCannotUndoDemotion(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")

	_, lb := do(t, e, "POST", "/api/v1/weddings/"+wid+"/invite-links", sarah, map[string]any{"role": "editor"})
	tok, _ := dataOf(lb)["token"].(string)
	if code, _ := do(t, e, "POST", "/api/v1/invite/"+tok+"/accept", omar, nil); code != 200 {
		t.Fatalf("join: want 200, got %d", code)
	}
	if code, _ := do(t, e, "PATCH", "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": "viewer"}); code != 200 {
		t.Fatalf("demote: want 200, got %d", code)
	}

	_, ab := do(t, e, "POST", "/api/v1/invite/"+tok+"/accept", omar, nil)
	if dataOf(ab)["role"] != "viewer" {
		t.Fatalf("re-accept after demotion: want viewer, got %v", dataOf(ab)["role"])
	}
	_, wb := do(t, e, "GET", "/api/v1/weddings/"+wid, omar, nil)
	if dataOf(wb)["my_role"] != "viewer" {
		t.Fatalf("my_role after re-accept: want viewer, got %v", dataOf(wb)["my_role"])
	}
}
