package test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// Owner role decisions are durable: an invite link can never give a member
// more than the owner last allowed, and a member the owner removed can only
// come back through a fresh owner invite.

func editorLink(t *testing.T, e *gin.Engine, ownerTok, wid string) string {
	t.Helper()
	_, lb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", ownerTok, map[string]any{"role": "editor"})
	tok, _ := dataOf(lb)["token"].(string)
	return tok
}

func acceptLink(t *testing.T, e *gin.Engine, tok, userTok string) (int, map[string]any) {
	t.Helper()
	return do(t, e, http.MethodPost, "/api/v1/invite/"+tok+"/accept", userTok, nil)
}

func myRole(t *testing.T, e *gin.Engine, wid, tok string) any {
	t.Helper()
	code, wb := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid, tok, nil)
	if code != http.StatusOK {
		return nil
	}
	return dataOf(wb)["my_role"]
}

func leave(t *testing.T, e *gin.Engine, wid, userID, tok string) {
	t.Helper()
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/weddings/"+wid+"/members/"+userID, tok, nil); code != http.StatusOK {
		t.Fatalf("remove/leave: want 200, got %d", code)
	}
}

func errCode(body map[string]any) any {
	if e, ok := body["error"].(map[string]any); ok {
		return e["code"]
	}
	return nil
}

func TestDemotedMemberRejoinsCappedAtOwnerRole(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	acceptLink(t, e, tok, omar)
	do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": "viewer"})
	leave(t, e, wid, omarID, omar) // self-leave keeps the owner's decision

	code, ab := acceptLink(t, e, tok, omar)
	if code != http.StatusOK || dataOf(ab)["role"] != "viewer" {
		t.Fatalf("rejoin via editor link after demotion: want 200 viewer, got %d %v", code, dataOf(ab)["role"])
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("my_role: want viewer, got %v", r)
	}
}

func TestRemovedMemberCannotRejoinByLink(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	acceptLink(t, e, tok, omar)
	leave(t, e, wid, omarID, sarah) // owner removes omar

	code, body := acceptLink(t, e, tok, omar)
	if code != http.StatusForbidden || errCode(body) != "removed_from_wedding" {
		t.Fatalf("rejoin after removal: want 403 removed_from_wedding, got %d %v", code, errCode(body))
	}
	// A brand-new link doesn't help either.
	if code, _ := acceptLink(t, e, editorLink(t, e, sarah, wid), omar); code != http.StatusForbidden {
		t.Fatalf("rejoin via new link: want 403, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != nil {
		t.Fatalf("removed member still has access: %v", r)
	}
}

func TestOwnerInviteReadmitsRemovedMember(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	acceptLink(t, e, tok, omar)
	leave(t, e, wid, omarID, sarah)

	inviteID := ownerInvite(t, e, sarah, wid, "omar", "viewer")
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", omar, nil); code != http.StatusOK {
		t.Fatalf("accept fresh owner invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("my_role after re-invite: want viewer, got %v", r)
	}
	// The owner's new decision replaces the old one: after leaving, the editor
	// link works again.
	leave(t, e, wid, omarID, omar)
	if code, ab := acceptLink(t, e, tok, omar); code != http.StatusOK || dataOf(ab)["role"] != "editor" {
		t.Fatalf("link after owner re-admission: want 200 editor, got %d %v", code, dataOf(ab)["role"])
	}
}

func TestInviteSentBeforeRemovalCannotReadmit(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")

	// Invite pending, but omar joins by link first, then is removed.
	inviteID := ownerInvite(t, e, sarah, wid, "omar", "editor")
	acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	leave(t, e, wid, omarID, sarah)

	code, body := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", omar, nil)
	if code != http.StatusForbidden || errCode(body) != "removed_from_wedding" {
		t.Fatalf("stale invite after removal: want 403 removed_from_wedding, got %d %v", code, errCode(body))
	}
}

func TestSelfLeaveWithoutOwnerDecisionCanRejoin(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	acceptLink(t, e, tok, omar)
	leave(t, e, wid, omarID, omar)
	if code, ab := acceptLink(t, e, tok, omar); code != http.StatusOK || dataOf(ab)["role"] != "editor" {
		t.Fatalf("rejoin after own leave: want 200 editor, got %d %v", code, dataOf(ab)["role"])
	}
}

func ownerInvite(t *testing.T, e *gin.Engine, ownerTok, wid, username, role string) string {
	t.Helper()
	code, body := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invites", ownerTok, map[string]any{"username": username, "role": role})
	if code != http.StatusCreated {
		t.Fatalf("owner invite: want 201, got %d (%v)", code, body)
	}
	id, _ := dataOf(body)["id"].(string)
	return id
}

func TestCeilingsGoWithTheirWeddingAndUser(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	lina, linaID := register(t, e, "lina")
	w1 := createWedding(t, e, sarah, "W1")
	w2 := createWedding(t, e, sarah, "W2")
	acceptLink(t, e, editorLink(t, e, sarah, w1), omar)
	acceptLink(t, e, editorLink(t, e, sarah, w2), lina)
	leave(t, e, w1, omarID, sarah) // ceiling (w1, omar)
	leave(t, e, w2, linaID, sarah) // ceiling (w2, lina)

	count := func() (n int64) {
		db.Raw("SELECT count(*) FROM membership_ceilings").Scan(&n)
		return n
	}
	if n := count(); n != 2 {
		t.Fatalf("setup: want 2 ceilings, got %d", n)
	}
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/weddings/"+w1, sarah, nil); code != http.StatusOK {
		t.Fatalf("delete wedding: want 200, got %d", code)
	}
	if n := count(); n != 1 {
		t.Fatalf("after wedding delete: want 1 ceiling, got %d", n)
	}
	if code, _ := do(t, e, http.MethodDelete, "/api/v1/me", lina, map[string]any{"password": "Password123!"}); code != http.StatusOK {
		t.Fatalf("delete account: want 200, got %d", code)
	}
	if n := count(); n != 0 {
		t.Fatalf("after account delete: want 0 ceilings, got %d", n)
	}
}
