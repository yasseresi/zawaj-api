package test

import (
	"net/http"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// Joining through an invite link needs the owner's approval: the link stays
// shareable, but nobody becomes a member until the owner approves them.

func joinRequests(t *testing.T, e *gin.Engine, tok, wid string) (int, []map[string]any) {
	t.Helper()
	code, body := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/join-requests", tok, nil)
	var out []map[string]any
	items, _ := dataOf(body)["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return code, out
}

func decideRequest(t *testing.T, e *gin.Engine, ownerTok, wid, rid, action string, body any) int {
	t.Helper()
	code, _ := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/join-requests/"+rid+"/"+action, ownerTok, body)
	return code
}

func notificationTypes(t *testing.T, e *gin.Engine, tok string) []string {
	t.Helper()
	_, body := do(t, e, http.MethodGet, "/api/v1/notifications", tok, nil)
	var types []string
	items, _ := dataOf(body)["items"].([]any)
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			s, _ := m["type"].(string)
			types = append(types, s)
		}
	}
	return types
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestLinkAcceptCreatesPendingRequest(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	code, body := acceptLink(t, e, tok, omar)
	d := dataOf(body)
	if code != http.StatusAccepted || d["status"] != "pending" || d["role"] != "editor" || d["wedding_name"] != "L&O" {
		t.Fatalf("accept link: want 202 pending editor, got %d %v", code, d)
	}
	rid, _ := d["request_id"].(string)
	if rid == "" {
		t.Fatalf("accept link: missing request_id: %v", d)
	}
	if r := myRole(t, e, wid, omar); r != nil {
		t.Fatalf("requester is a member before approval: %v", r)
	}

	// Tapping again returns the same pending request (no duplicates).
	if code, again := acceptLink(t, e, tok, omar); code != http.StatusAccepted || dataOf(again)["request_id"] != rid {
		t.Fatalf("repeat accept: want 202 same request, got %d %v", code, dataOf(again))
	}

	code, reqs := joinRequests(t, e, sarah, wid)
	if code != http.StatusOK || len(reqs) != 1 {
		t.Fatalf("owner list: want 200 with 1 request, got %d %v", code, reqs)
	}
	if reqs[0]["id"] != rid || reqs[0]["username"] != "omar" || reqs[0]["role"] != "editor" {
		t.Fatalf("request row: %v", reqs[0])
	}
	if !contains(notificationTypes(t, e, sarah), "join_requested") {
		t.Fatalf("owner was not notified of the join request")
	}
}

func TestExistingMemberAcceptingLinkStaysMember(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	wid := createWedding(t, e, sarah, "L&O")

	code, body := acceptLink(t, e, editorLink(t, e, sarah, wid), sarah)
	if code != http.StatusOK || dataOf(body)["status"] != "member" || dataOf(body)["role"] != "owner" {
		t.Fatalf("owner opening a link: want 200 member owner, got %d %v", code, dataOf(body))
	}
	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 0 {
		t.Fatalf("member's own accept created a request: %v", reqs)
	}
}

func TestApproveJoinRequest(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	_, body := acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	rid, _ := dataOf(body)["request_id"].(string)

	if code := decideRequest(t, e, sarah, wid, rid, "approve", nil); code != http.StatusOK {
		t.Fatalf("approve: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "editor" {
		t.Fatalf("my_role after approval: want editor, got %v", r)
	}
	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 0 {
		t.Fatalf("approved request still pending: %v", reqs)
	}
	if !contains(notificationTypes(t, e, omar), "join_approved") {
		t.Fatalf("requester was not notified of the approval")
	}
	if code := decideRequest(t, e, sarah, wid, rid, "approve", nil); code != http.StatusNotFound {
		t.Fatalf("approve twice: want 404, got %d", code)
	}
}

func TestApproveCanLowerButNotRaiseRole(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")

	_, vb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", sarah, map[string]any{"role": "viewer"})
	viewerTok, _ := dataOf(vb)["token"].(string)
	_, ob := acceptLink(t, e, viewerTok, omar)
	omarReq, _ := dataOf(ob)["request_id"].(string)
	if code := decideRequest(t, e, sarah, wid, omarReq, "approve", map[string]any{"role": "editor"}); code != http.StatusBadRequest {
		t.Fatalf("approve above the link's role: want 400, got %d", code)
	}

	_, lb := acceptLink(t, e, editorLink(t, e, sarah, wid), lina)
	linaReq, _ := dataOf(lb)["request_id"].(string)
	if code := decideRequest(t, e, sarah, wid, linaReq, "approve", map[string]any{"role": "viewer"}); code != http.StatusOK {
		t.Fatalf("approve lowered: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, lina); r != "viewer" {
		t.Fatalf("my_role after lowered approval: want viewer, got %v", r)
	}
}

func TestDeclineJoinRequest(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	_, body := acceptLink(t, e, tok, omar)
	rid, _ := dataOf(body)["request_id"].(string)

	if code := decideRequest(t, e, sarah, wid, rid, "decline", nil); code != http.StatusOK {
		t.Fatalf("decline: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != nil {
		t.Fatalf("declined requester became a member: %v", r)
	}
	if !contains(notificationTypes(t, e, omar), "join_declined") {
		t.Fatalf("requester was not notified of the decline")
	}
	// Asking again right away is refused, so a declined user can't spam the owner.
	code, again := acceptLink(t, e, tok, omar)
	if code != http.StatusConflict || errCode(again) != "join_request_declined" {
		t.Fatalf("re-request right after decline: want 409 join_request_declined, got %d %v", code, errCode(again))
	}
}

func TestJoinRequestEndpointsAreOwnerOnly(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	lina, _ := register(t, e, "lina")
	omar, _ := register(t, e, "omar")
	stranger, _ := register(t, e, "stranger")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "editor", lina)
	_, body := acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	rid, _ := dataOf(body)["request_id"].(string)

	if code, _ := joinRequests(t, e, lina, wid); code != http.StatusForbidden {
		t.Fatalf("editor lists requests: want 403, got %d", code)
	}
	if code := decideRequest(t, e, lina, wid, rid, "approve", nil); code != http.StatusForbidden {
		t.Fatalf("editor approves: want 403, got %d", code)
	}
	if code := decideRequest(t, e, stranger, wid, rid, "decline", nil); code != http.StatusNotFound {
		t.Fatalf("stranger declines: want 404, got %d", code)
	}
	// A request id from another wedding is not found.
	other := createWedding(t, e, sarah, "Other")
	if code := decideRequest(t, e, sarah, other, rid, "approve", nil); code != http.StatusNotFound {
		t.Fatalf("approve via another wedding: want 404, got %d", code)
	}
}

func TestRemovalKeepsLinksActive(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	linkID, tok := newLink(t, e, sarah, wid, "editor")
	joinViaLink(t, e, sarah, wid, tok, omar)

	do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/members/"+omarID, sarah, map[string]any{"role": "viewer"})
	leave(t, e, wid, omarID, sarah)
	if !activeLinkIDs(t, e, sarah, wid)[linkID] {
		t.Fatalf("a shared link was revoked by a member's demotion/removal")
	}
}

func TestJoinRequestsGoWithTheirWeddingAndUser(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	w1 := createWedding(t, e, sarah, "W1")
	w2 := createWedding(t, e, sarah, "W2")
	acceptLink(t, e, editorLink(t, e, sarah, w1), omar)
	acceptLink(t, e, editorLink(t, e, sarah, w2), lina)

	count := func() (n int64) {
		db.Raw("SELECT count(*) FROM join_requests").Scan(&n)
		return n
	}
	if n := count(); n != 2 {
		t.Fatalf("setup: want 2 requests, got %d", n)
	}
	do(t, e, http.MethodDelete, "/api/v1/weddings/"+w1, sarah, nil)
	if n := count(); n != 1 {
		t.Fatalf("after wedding delete: want 1 request, got %d", n)
	}
	do(t, e, http.MethodDelete, "/api/v1/me", lina, map[string]any{"password": "Password123!"})
	if n := count(); n != 0 {
		t.Fatalf("after account delete: want 0 requests, got %d", n)
	}
}

// joinViaLink sends a join request through the link and has the owner approve
// it as requested; returns the resulting role.
func joinViaLink(t *testing.T, e *gin.Engine, ownerTok, wid, linkTok, userTok string) any {
	t.Helper()
	code, body := acceptLink(t, e, linkTok, userTok)
	if code == http.StatusOK { // already a member
		return dataOf(body)["role"]
	}
	if code != http.StatusAccepted {
		t.Fatalf("join request: want 202, got %d (%v)", code, body)
	}
	rid, _ := dataOf(body)["request_id"].(string)
	if c := decideRequest(t, e, ownerTok, wid, rid, "approve", nil); c != http.StatusOK {
		t.Fatalf("approve: want 200, got %d", c)
	}
	return dataOf(body)["role"]
}

func TestParallelAcceptsCreateOneRequest(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			serve(e, newReq(http.MethodPost, "/api/v1/invite/"+tok+"/accept", omar, nil))
		}()
	}
	wg.Wait()

	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 1 {
		t.Fatalf("parallel accepts: want 1 pending request, got %d", len(reqs))
	}
	n := 0
	for _, typ := range notificationTypes(t, e, sarah) {
		if typ == "join_requested" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("parallel accepts: want 1 owner notification, got %d", n)
	}
}

func TestParallelDecisionsOnOneRequest(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	_, body := acceptLink(t, e, editorLink(t, e, sarah, wid), omar)
	rid, _ := dataOf(body)["request_id"].(string)

	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, action := range []string{"approve", "decline"} {
		wg.Add(1)
		go func(action string) {
			defer wg.Done()
			rec := serve(e, newReq(http.MethodPost, "/api/v1/weddings/"+wid+"/join-requests/"+rid+"/"+action, sarah, nil))
			codes <- rec.Code
		}(action)
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusNotFound:
		default:
			t.Fatalf("parallel decision: unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("parallel approve+decline: want exactly one to win, got %d", ok)
	}
}
