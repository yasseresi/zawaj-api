package test

import (
	"net/http"
	"testing"
)

// TestInvites covers the targeted-invite lifecycle: an owner invites a user,
// the invitee sees it, guards hold, and accept grants membership.
func TestInvites(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")

	invites := "/api/v1/weddings/" + wid + "/invites"

	// Non-owner cannot invite.
	if code, _ := do(t, e, http.MethodPost, invites, omar, map[string]any{"username": "lina", "role": "viewer"}); code != http.StatusNotFound && code != http.StatusForbidden {
		t.Fatalf("non-member invite: want 403/404, got %d", code)
	}

	// Inviting an unknown user → 404.
	if code, _ := do(t, e, http.MethodPost, invites, sarah, map[string]any{"username": "ghost", "role": "editor"}); code != http.StatusNotFound {
		t.Fatalf("invite unknown user: want 404, got %d", code)
	}

	// Owner invites omar as editor.
	code, body := do(t, e, http.MethodPost, invites, sarah, map[string]any{"username": "omar", "role": "editor"})
	if code != http.StatusCreated {
		t.Fatalf("invite omar: want 201, got %d (%v)", code, body)
	}

	// Duplicate pending invite → 409.
	if code, _ := do(t, e, http.MethodPost, invites, sarah, map[string]any{"username": "omar", "role": "editor"}); code != http.StatusConflict {
		t.Fatalf("duplicate invite: want 409, got %d", code)
	}

	// omar sees exactly one pending invite, enriched.
	_, lb := do(t, e, http.MethodGet, "/api/v1/invites", omar, nil)
	items, _ := dataOf(lb)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("omar pending invites: want 1, got %d (%v)", len(items), items)
	}
	inv, _ := items[0].(map[string]any)
	if inv["wedding_name"] != "L&O" || inv["inviter_name"] != "sarah" || inv["role"] != "editor" {
		t.Fatalf("invite not enriched correctly: %v", inv)
	}
	inviteID, _ := inv["id"].(string)

	// A stranger cannot accept omar's invite (existence hidden → 404).
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", lina, nil); code != http.StatusNotFound {
		t.Fatalf("stranger accept: want 404, got %d", code)
	}

	// omar accepts → becomes a member.
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", omar, nil); code != http.StatusOK {
		t.Fatalf("accept invite: want 200, got %d", code)
	}
	// Now omar can read the wedding's guests (viewer+).
	if code, _ := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/guests", omar, nil); code != http.StatusOK {
		t.Fatalf("omar guest access after accept: want 200, got %d", code)
	}
	// Invite is no longer pending.
	_, lb2 := do(t, e, http.MethodGet, "/api/v1/invites", omar, nil)
	if items2, _ := dataOf(lb2)["items"].([]any); len(items2) != 0 {
		t.Fatalf("omar invites after accept: want 0, got %d", len(items2))
	}

	// Decline flow: invite lina, she declines.
	_, cb := do(t, e, http.MethodPost, invites, sarah, map[string]any{"username": "lina", "role": "viewer"})
	_ = cb
	_, llb := do(t, e, http.MethodGet, "/api/v1/invites", lina, nil)
	linaItems, _ := dataOf(llb)["items"].([]any)
	if len(linaItems) != 1 {
		t.Fatalf("lina pending: want 1, got %d", len(linaItems))
	}
	linaInvite, _ := linaItems[0].(map[string]any)
	lid, _ := linaInvite["id"].(string)
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+lid+"/decline", lina, nil); code != http.StatusOK {
		t.Fatalf("decline: want 200, got %d", code)
	}
	// Declined invite: lina is NOT a member.
	if code, _ := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/guests", lina, nil); code != http.StatusNotFound {
		t.Fatalf("lina access after decline: want 404, got %d", code)
	}
}
