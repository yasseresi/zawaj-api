package test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// Owner role decisions are durable: a join request made through an invite link
// never asks for more than the owner last allowed, a member the owner removed
// can't even request (only a fresh owner invite re-admits them), and every
// owner decision — role change, removal, approval, accepted username invite —
// is recorded as the new ceiling.

func editorLink(t *testing.T, e *gin.Engine, ownerTok, wid string) string {
	t.Helper()
	_, tok := newLink(t, e, ownerTok, wid, "editor")
	return tok
}

func newLink(t *testing.T, e *gin.Engine, ownerTok, wid, role string) (id, tok string) {
	t.Helper()
	_, lb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", ownerTok, map[string]any{"role": role})
	id, _ = dataOf(lb)["id"].(string)
	tok, _ = dataOf(lb)["token"].(string)
	return id, tok
}

func acceptLink(t *testing.T, e *gin.Engine, tok, userTok string) (int, map[string]any) {
	t.Helper()
	return do(t, e, http.MethodPost, "/api/v1/invite/"+tok+"/accept", userTok, nil)
}

// requestedRole accepts the link and asserts a pending request was created,
// returning the role it asks for.
func requestedRole(t *testing.T, e *gin.Engine, tok, userTok string) any {
	t.Helper()
	code, body := acceptLink(t, e, tok, userTok)
	if code != http.StatusAccepted || dataOf(body)["status"] != "pending" {
		t.Fatalf("join request: want 202 pending, got %d %v", code, body)
	}
	return dataOf(body)["role"]
}

func myRole(t *testing.T, e *gin.Engine, wid, tok string) any {
	t.Helper()
	code, wb := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid, tok, nil)
	if code != http.StatusOK {
		return nil
	}
	return dataOf(wb)["my_role"]
}

func setRole(t *testing.T, e *gin.Engine, ownerTok, wid, userID, role string) {
	t.Helper()
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+userID, ownerTok, map[string]any{"role": role}); code != http.StatusOK {
		t.Fatalf("set role %s: want 200, got %d", role, code)
	}
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

func ownerInvite(t *testing.T, e *gin.Engine, ownerTok, wid, username, role string) string {
	t.Helper()
	code, body := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invites", ownerTok, map[string]any{"username": username, "role": role})
	if code != http.StatusCreated {
		t.Fatalf("owner invite: want 201, got %d (%v)", code, body)
	}
	id, _ := dataOf(body)["id"].(string)
	return id
}

func acceptInvite(t *testing.T, e *gin.Engine, inviteID, userTok string) int {
	t.Helper()
	code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", userTok, nil)
	return code
}

func activeLinkIDs(t *testing.T, e *gin.Engine, ownerTok, wid string) map[string]bool {
	t.Helper()
	_, lb := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/invite-links", ownerTok, nil)
	ids := map[string]bool{}
	items, _ := dataOf(lb)["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			id, _ := m["id"].(string)
			ids[id] = true
		}
	}
	return ids
}

func TestDemotedFormerMemberRequestIsCapped(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	joinViaLink(t, e, sarah, wid, tok, omar)
	setRole(t, e, sarah, wid, omarID, "viewer")
	leave(t, e, wid, omarID, omar) // self-leave keeps the owner's decision

	if r := requestedRole(t, e, tok, omar); r != "viewer" {
		t.Fatalf("request via editor link after demotion: want viewer, got %v", r)
	}
}

func TestRemovedMemberCannotRequestToJoin(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	joinViaLink(t, e, sarah, wid, tok, omar)
	leave(t, e, wid, omarID, sarah) // owner removes omar

	for _, link := range []string{tok, editorLink(t, e, sarah, wid)} {
		code, body := acceptLink(t, e, link, omar)
		if code != http.StatusForbidden || errCode(body) != "removed_from_wedding" {
			t.Fatalf("removed user requests: want 403 removed_from_wedding, got %d %v", code, errCode(body))
		}
	}
	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 0 {
		t.Fatalf("removed user's request was created: %v", reqs)
	}
}

func TestApprovalIsRecordedAsOwnerDecision(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	_, body := acceptLink(t, e, tok, omar)
	rid, _ := dataOf(body)["request_id"].(string)
	if code := decideRequest(t, e, sarah, wid, rid, "approve", map[string]any{"role": "viewer"}); code != http.StatusOK {
		t.Fatalf("approve as viewer: want 200, got %d", code)
	}
	leave(t, e, wid, omarID, omar)
	if r := requestedRole(t, e, tok, omar); r != "viewer" {
		t.Fatalf("request after a viewer approval: want viewer, got %v", r)
	}
}

func TestOwnerInviteReadmitsRemovedMember(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	joinViaLink(t, e, sarah, wid, tok, omar)
	leave(t, e, wid, omarID, sarah)

	if code := acceptInvite(t, e, ownerInvite(t, e, sarah, wid, "omar", "viewer"), omar); code != http.StatusOK {
		t.Fatalf("accept fresh owner invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("my_role after re-invite: want viewer, got %v", r)
	}
	// The re-invite is the owner's new decision ("viewer"), not a reset.
	leave(t, e, wid, omarID, omar)
	if r := requestedRole(t, e, tok, omar); r != "viewer" {
		t.Fatalf("request via editor link after viewer re-invite: want viewer, got %v", r)
	}
}

func TestOwnerInviteAfterDemotionLiftsCap(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	joinViaLink(t, e, sarah, wid, tok, omar)
	setRole(t, e, sarah, wid, omarID, "viewer")
	leave(t, e, wid, omarID, omar)

	if code := acceptInvite(t, e, ownerInvite(t, e, sarah, wid, "omar", "editor"), omar); code != http.StatusOK {
		t.Fatalf("accept editor re-invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "editor" {
		t.Fatalf("my_role after editor re-invite: want editor, got %v", r)
	}
	leave(t, e, wid, omarID, omar)
	if r := requestedRole(t, e, tok, omar); r != "editor" {
		t.Fatalf("request after editor re-invite: want editor, got %v", r)
	}
}

func TestPromotionLiftsCeiling(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	joinViaLink(t, e, sarah, wid, tok, omar)
	setRole(t, e, sarah, wid, omarID, "viewer")
	setRole(t, e, sarah, wid, omarID, "editor")
	leave(t, e, wid, omarID, omar)

	if r := requestedRole(t, e, tok, omar); r != "editor" {
		t.Fatalf("request after promotion: want editor, got %v", r)
	}
}

func TestInviteSentBeforeRemovalCannotReadmit(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")

	// Invite pending, but omar joins by link first, then is removed. The
	// removal withdraws the pending invite, so it can't re-admit him...
	inviteID := ownerInvite(t, e, sarah, wid, "omar", "editor")
	joinViaLink(t, e, sarah, wid, editorLink(t, e, sarah, wid), omar)
	leave(t, e, wid, omarID, sarah)

	if code := acceptInvite(t, e, inviteID, omar); code != http.StatusNotFound {
		t.Fatalf("stale invite after removal: want 404 (withdrawn), got %d", code)
	}
	_, lb := do(t, e, http.MethodGet, "/api/v1/invites", omar, nil)
	if items, _ := dataOf(lb)["items"].([]any); len(items) != 0 {
		t.Fatalf("withdrawn invite still listed: %v", items)
	}
	// ...and doesn't block the owner from inviting him back.
	if code := acceptInvite(t, e, ownerInvite(t, e, sarah, wid, "omar", "viewer"), omar); code != http.StatusOK {
		t.Fatalf("accept fresh invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("my_role after fresh invite: want viewer, got %v", r)
	}
}

func TestSelfLeaveWithoutOwnerDecisionCanRequestAgain(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	joinViaLink(t, e, sarah, wid, tok, omar)
	leave(t, e, wid, omarID, omar)
	if r := requestedRole(t, e, tok, omar); r != "editor" {
		t.Fatalf("request after own leave: want editor, got %v", r)
	}
}

func TestCeilingsGoWithTheirWeddingAndUser(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	lina, linaID := register(t, e, "lina")
	w1 := createWedding(t, e, sarah, "W1")
	w2 := createWedding(t, e, sarah, "W2")
	joinViaLink(t, e, sarah, w1, editorLink(t, e, sarah, w1), omar)
	joinViaLink(t, e, sarah, w2, editorLink(t, e, sarah, w2), lina)
	leave(t, e, w1, omarID, sarah) // ceiling (w1, omar) = none
	leave(t, e, w2, linaID, sarah) // ceiling (w2, lina) = none

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
