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

	// The link he joined with was revoked by the demotion; a fresh editor link
	// is capped at the owner's decision.
	if code, _ := acceptLink(t, e, tok, omar); code != http.StatusNotFound {
		t.Fatalf("joined-with link after demotion: want 404 (revoked), got %d", code)
	}
	code, ab := acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
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

	// The link he joined with was revoked by the removal...
	if code, _ := acceptLink(t, e, tok, omar); code != http.StatusNotFound {
		t.Fatalf("joined-with link after removal: want 404 (revoked), got %d", code)
	}
	// ...and any other link is refused for him.
	code, body := acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	if code != http.StatusForbidden || errCode(body) != "removed_from_wedding" {
		t.Fatalf("rejoin via another link: want 403 removed_from_wedding, got %d %v", code, errCode(body))
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
	// The re-invite is the owner's new decision ("viewer"), not a reset: after
	// leaving, the editor link still only gives viewer.
	leave(t, e, wid, omarID, omar)
	if code, ab := acceptLink(t, e, editorLink(t, e, sarah, wid), omar); code != http.StatusOK || dataOf(ab)["role"] != "viewer" {
		t.Fatalf("editor link after viewer re-invite: want 200 viewer, got %d %v", code, dataOf(ab)["role"])
	}
}

func TestOwnerInviteAfterDemotionLiftsCap(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	acceptLink(t, e, tok, omar)
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": "viewer"}); code != http.StatusOK {
		t.Fatalf("demote: want 200, got %d", code)
	}
	leave(t, e, wid, omarID, omar)

	inviteID := ownerInvite(t, e, sarah, wid, "omar", "editor")
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", omar, nil); code != http.StatusOK {
		t.Fatalf("accept editor re-invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "editor" {
		t.Fatalf("my_role after editor re-invite: want editor, got %v", r)
	}
	leave(t, e, wid, omarID, omar)
	if code, ab := acceptLink(t, e, editorLink(t, e, sarah, wid), omar); code != http.StatusOK || dataOf(ab)["role"] != "editor" {
		t.Fatalf("editor link after editor re-invite: want 200 editor, got %d %v", code, dataOf(ab)["role"])
	}
}

func TestPromotionLiftsCeiling(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	acceptLink(t, e, tok, omar)
	for _, role := range []string{"viewer", "editor"} {
		if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": role}); code != http.StatusOK {
			t.Fatalf("set role %s: want 200, got %d", role, code)
		}
	}
	leave(t, e, wid, omarID, omar)
	// The demotion revoked the link he joined with; a fresh editor link is no
	// longer capped.
	if code, _ := acceptLink(t, e, tok, omar); code != http.StatusNotFound {
		t.Fatalf("link revoked by the demotion: want 404, got %d", code)
	}
	if code, ab := acceptLink(t, e, editorLink(t, e, sarah, wid), omar); code != http.StatusOK || dataOf(ab)["role"] != "editor" {
		t.Fatalf("fresh editor link after promotion: want 200 editor, got %d %v", code, dataOf(ab)["role"])
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
	acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	leave(t, e, wid, omarID, sarah)

	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+inviteID+"/accept", omar, nil); code != http.StatusNotFound {
		t.Fatalf("stale invite after removal: want 404 (withdrawn), got %d", code)
	}
	_, lb := do(t, e, http.MethodGet, "/api/v1/invites", omar, nil)
	if items, _ := dataOf(lb)["items"].([]any); len(items) != 0 {
		t.Fatalf("withdrawn invite still listed: %v", items)
	}

	// ...and doesn't block the owner from inviting him back.
	fresh := ownerInvite(t, e, sarah, wid, "omar", "viewer")
	if code, _ := do(t, e, http.MethodPost, "/api/v1/invites/"+fresh+"/accept", omar, nil); code != http.StatusOK {
		t.Fatalf("accept fresh invite: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("my_role after fresh invite: want viewer, got %v", r)
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

// Ceilings follow the user id, so a removed or demoted member could rejoin
// through the same link with a second account. The link they joined with is
// therefore revoked when the owner removes them, or demotes them below the
// role it grants. Other links keep working.

func activeLinkIDs(t *testing.T, e *gin.Engine, ownerTok, wid string) map[string]bool {
	t.Helper()
	_, lb := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/invite-links", ownerTok, nil)
	ids := map[string]bool{}
	items, _ := lb["data"].([]any)
	if items == nil {
		items, _ = dataOf(lb)["items"].([]any)
	}
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			id, _ := m["id"].(string)
			ids[id] = true
		}
	}
	return ids
}

func newLink(t *testing.T, e *gin.Engine, ownerTok, wid, role string) (id, tok string) {
	t.Helper()
	_, lb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", ownerTok, map[string]any{"role": role})
	id, _ = dataOf(lb)["id"].(string)
	tok, _ = dataOf(lb)["token"].(string)
	return id, tok
}

func TestRemovalRevokesTheLinkTheyJoinedWith(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	omar2, _ := register(t, e, "omar2") // omar's second account
	wid := createWedding(t, e, sarah, "L&O")
	usedID, usedTok := newLink(t, e, sarah, wid, "editor")
	otherID, _ := newLink(t, e, sarah, wid, "editor")

	acceptLink(t, e, usedTok, omar)
	leave(t, e, wid, omarID, sarah)

	if code, _ := acceptLink(t, e, usedTok, omar2); code != http.StatusNotFound {
		t.Fatalf("second account via the removed member's link: want 404, got %d", code)
	}
	links := activeLinkIDs(t, e, sarah, wid)
	if links[usedID] || !links[otherID] {
		t.Fatalf("want only the used link revoked; active=%v used=%s other=%s", links, usedID, otherID)
	}
}

func TestDemotionRevokesTheEditorLinkTheyJoinedWith(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	omar2, _ := register(t, e, "omar2")
	wid := createWedding(t, e, sarah, "L&O")
	usedID, usedTok := newLink(t, e, sarah, wid, "editor")

	acceptLink(t, e, usedTok, omar)
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": "viewer"}); code != http.StatusOK {
		t.Fatalf("demote: want 200, got %d", code)
	}
	if code, _ := acceptLink(t, e, usedTok, omar2); code != http.StatusNotFound {
		t.Fatalf("second account via editor link after demotion: want 404, got %d", code)
	}
	if activeLinkIDs(t, e, sarah, wid)[usedID] {
		t.Fatalf("editor link still active after demoting the member who used it")
	}
}

func TestLinksSurviveSelfLeaveAndNonDemotingChanges(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	lina, linaID := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")
	viewerID, viewerTok := newLink(t, e, sarah, wid, "viewer")
	editorID, editorTok := newLink(t, e, sarah, wid, "editor")

	// lina joined via the viewer link; promoting then demoting her back to
	// viewer never goes below what that link grants.
	acceptLink(t, e, viewerTok, lina)
	for _, role := range []string{"editor", "viewer"} {
		do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+linaID, sarah, map[string]any{"role": role})
	}
	// omar joined via the editor link and leaves on his own.
	acceptLink(t, e, editorTok, omar)
	leave(t, e, wid, omarID, omar)

	links := activeLinkIDs(t, e, sarah, wid)
	if !links[viewerID] || !links[editorID] {
		t.Fatalf("links revoked without an owner removal/demotion below them: active=%v", links)
	}
}
