package test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"zawaj/internal/repository"

	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

	const workers = 20
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make(chan int, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release together so the inserts genuinely race
			codes <- serve(e, newReq(http.MethodPost, "/api/v1/invite/"+tok+"/accept", omar, nil)).Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != http.StatusAccepted {
			t.Fatalf("parallel accept: want every call 202, got %d", c)
		}
	}

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

	const workers = 20
	codes := make(chan int, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		action := []string{"approve", "decline"}[i%2]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := serve(e, newReq(http.MethodPost, "/api/v1/weddings/"+wid+"/join-requests/"+rid+"/"+action, sarah, nil))
			codes <- rec.Code
		}()
	}
	close(start)
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

// pgx caches prepared statements per connection, and after five runs Postgres
// may switch to a generic plan. A bind-parameter predicate in the ON CONFLICT
// target then no longer matches the partial unique index (SQLSTATE 42P10), so
// repeated accepts on one connection must keep working. Each accept comes from
// a different user so every call reaches the INSERT (a re-tap by the same user
// returns the existing request before it).
func TestRepeatedAcceptsOnOneConnection(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	users := make([]string, 10)
	for i := range users {
		users[i], _ = register(t, e, fmt.Sprintf("guest%d", i))
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)

	for i, u := range users {
		if code, body := acceptLink(t, e, tok, u); code != http.StatusAccepted {
			t.Fatalf("accept #%d: want 202, got %d %v", i+1, code, body)
		}
	}
	var n int64
	db.Raw("SELECT count(*) FROM join_requests WHERE status = 'pending'").Scan(&n)
	if n != int64(len(users)) {
		t.Fatalf("want %d pending requests, got %d", len(users), n)
	}
}

// pendingRequest makes omar request to join through an editor link and
// returns the request id.
func pendingRequest(t *testing.T, e *gin.Engine, ownerTok, wid, userTok string) string {
	t.Helper()
	code, body := acceptLink(t, e, editorLink(t, e, ownerTok, wid), userTok)
	if code != http.StatusAccepted {
		t.Fatalf("join request: want 202, got %d %v", code, body)
	}
	rid, _ := dataOf(body)["request_id"].(string)
	return rid
}

func requestStatus(t *testing.T, db *gorm.DB, rid string) string {
	t.Helper()
	var s string
	db.Raw("SELECT status FROM join_requests WHERE id = ?", rid).Scan(&s)
	return s
}

func TestUsernameInviteClosesPendingRequest(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)

	if code := acceptInvite(t, e, ownerInvite(t, e, sarah, wid, "omar", "viewer"), omar); code != http.StatusOK {
		t.Fatalf("accept username invite: want 200, got %d", code)
	}
	if s := requestStatus(t, db, rid); s != "closed" {
		t.Fatalf("join request after joining by username invite: want closed, got %q", s)
	}
	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 0 {
		t.Fatalf("owner still sees a request from a member: %v", reqs)
	}
}

func TestOwnerRemovalClosesPendingRequest(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "viewer", omar)
	// A request left pending while already a member (e.g. a race).
	db.Exec(`INSERT INTO join_requests (id, created_at, updated_at, wedding_id, user_id, link_id, role, status)
		VALUES (gen_random_uuid(), now(), now(), ?, ?, gen_random_uuid(), 'editor', 'pending')`, wid, omarID)

	leave(t, e, wid, omarID, sarah)
	var n int64
	db.Raw("SELECT count(*) FROM join_requests WHERE wedding_id = ? AND user_id = ? AND status = 'pending'", wid, omarID).Scan(&n)
	if n != 0 {
		t.Fatalf("owner removal left %d pending request(s)", n)
	}
}

func TestApproveRefusesRemovedUser(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)
	// The owner removed omar after he asked (ceiling "none").
	db.Exec(`INSERT INTO membership_ceilings (wedding_id, user_id, max_role, set_at) VALUES (?, ?, 'none', now())`, wid, omarID)

	code, body := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/join-requests/"+rid+"/approve", sarah, nil)
	if code != http.StatusForbidden || errCode(body) != "removed_from_wedding" {
		t.Fatalf("approve a removed user's request: want 403 removed_from_wedding, got %d %v", code, errCode(body))
	}
	if r := myRole(t, e, wid, omar); r != nil {
		t.Fatalf("removed user re-admitted by approval: %v", r)
	}
	if s := requestStatus(t, db, rid); s != "closed" {
		t.Fatalf("refused request: want closed, got %q", s)
	}
}

func TestApproveWhenAlreadyMemberClosesQuietly(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)
	// omar became a viewer some other way while the request was pending.
	db.Exec(`INSERT INTO memberships (id, created_at, updated_at, wedding_id, user_id, role, joined_at)
		VALUES (gen_random_uuid(), now(), now(), ?, ?, 'viewer', now())`, wid, omarID)

	if code := decideRequest(t, e, sarah, wid, rid, "approve", nil); code != http.StatusOK {
		t.Fatalf("approve: want 200, got %d", code)
	}
	if r := myRole(t, e, wid, omar); r != "viewer" {
		t.Fatalf("existing membership changed by approval: %v", r)
	}
	if s := requestStatus(t, db, rid); s != "closed" {
		t.Fatalf("request for an existing member: want closed, got %q", s)
	}
	if contains(notificationTypes(t, e, omar), "join_approved") {
		t.Fatalf("existing member was told their request was approved")
	}
}

func TestPendingListHidesExistingMembers(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	pendingRequest(t, e, sarah, wid, omar)
	db.Exec(`INSERT INTO memberships (id, created_at, updated_at, wedding_id, user_id, role, joined_at)
		VALUES (gen_random_uuid(), now(), now(), ?, ?, 'viewer', now())`, wid, omarID)

	if _, reqs := joinRequests(t, e, sarah, wid); len(reqs) != 0 {
		t.Fatalf("owner list shows a request from a member: %v", reqs)
	}
}

func TestRevokingLinkClosesItsPendingRequests(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")
	leakedID, leaked := newLink(t, e, sarah, wid, "editor")
	_, other := newLink(t, e, sarah, wid, "editor")
	_, ob := acceptLink(t, e, leaked, omar)
	omarReq, _ := dataOf(ob)["request_id"].(string)
	acceptLink(t, e, other, lina)

	if code, _ := do(t, e, http.MethodDelete, "/api/v1/weddings/"+wid+"/invite-links/"+leakedID, sarah, nil); code != http.StatusOK {
		t.Fatalf("revoke: want 200, got %d", code)
	}
	if s := requestStatus(t, db, omarReq); s != "closed" {
		t.Fatalf("request through the revoked link: want closed, got %q", s)
	}
	_, reqs := joinRequests(t, e, sarah, wid)
	if len(reqs) != 1 || reqs[0]["username"] != "lina" {
		t.Fatalf("other links' requests must stay: %v", reqs)
	}
}

func TestPendingRequestsAreCappedPerWedding(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	acceptLink(t, e, tok, omar) // omar is in the queue
	// Fill the rest of the queue (cap 50) with other requesters.
	for i := 0; i < 49; i++ {
		db.Exec(`INSERT INTO join_requests (id, created_at, updated_at, wedding_id, user_id, link_id, role, status)
			VALUES (gen_random_uuid(), now(), now(), ?, gen_random_uuid(), gen_random_uuid(), 'viewer', 'pending')`, wid)
	}

	code, body := acceptLink(t, e, tok, lina)
	if code != http.StatusTooManyRequests || errCode(body) != "join_queue_full" {
		t.Fatalf("51st requester: want 429 join_queue_full, got %d %v", code, errCode(body))
	}
	// Someone already queued still gets their request back.
	if code, _ := acceptLink(t, e, tok, omar); code != http.StatusAccepted {
		t.Fatalf("queued requester re-tapping: want 202, got %d", code)
	}
}

// The cap must hold when the last free slot is raced: CreatePending's advisory
// lock serializes the count-then-insert, so exactly one racer gets the slot.
// The window is narrow, so the race is repeated over several rounds.
func TestParallelAcceptsRespectQueueCap(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	const workers, rounds = 10, 10
	users := make([]string, workers)
	for i := range users {
		users[i], _ = register(t, e, fmt.Sprintf("racer%d", i))
	}
	// Leave exactly one free slot (cap 50).
	for i := 0; i < 49; i++ {
		db.Exec(`INSERT INTO join_requests (id, created_at, updated_at, wedding_id, user_id, link_id, role, status)
			VALUES (gen_random_uuid(), now(), now(), ?, gen_random_uuid(), gen_random_uuid(), 'viewer', 'pending')`, wid)
	}

	type result struct {
		code int
		err  string
	}
	for round := 1; round <= rounds; round++ {
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan result, workers)
		for _, u := range users {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				<-start // release together so the count checks genuinely race
				rec := serve(e, newReq(http.MethodPost, "/api/v1/invite/"+tok+"/accept", u, nil))
				var body map[string]any
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				code, _ := errCode(body).(string)
				results <- result{rec.Code, code}
			}(u)
		}
		close(start)
		wg.Wait()
		close(results)

		accepted := 0
		for r := range results {
			switch {
			case r.code == http.StatusAccepted:
				accepted++
			case r.code == http.StatusTooManyRequests && r.err == "join_queue_full":
			default:
				t.Fatalf("round %d: unexpected %d %q", round, r.code, r.err)
			}
		}
		var n int64
		db.Raw("SELECT count(*) FROM join_requests WHERE wedding_id = ? AND status = 'pending'", wid).Scan(&n)
		if accepted != 1 || n != 50 {
			t.Fatalf("round %d racing for the last slot: want 1 accepted and 50 pending, got %d accepted, %d pending", round, accepted, n)
		}
		// Free the slot again: drop this round's winner.
		db.Exec(`DELETE FROM join_requests WHERE wedding_id = ? AND status = 'pending'
			AND user_id IN (SELECT id FROM users)`, wid)
	}
}

func TestOwnerIsPushedOncePerBatchOfRequests(t *testing.T) {
	rec := &recordingSender{}
	a := newTestApp(t, rec)
	e := a.e
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	zed, _ := register(t, e, "zed")
	wid := createWedding(t, e, sarah, "L&O")
	if code, _ := do(t, e, http.MethodPost, "/api/v1/me/devices", sarah, map[string]any{"token": "sarah-device", "platform": "ios"}); code >= 300 {
		t.Fatalf("register device: %d", code)
	}
	tok := editorLink(t, e, sarah, wid)

	_, ob := acceptLink(t, e, tok, omar) // queue empty → push
	acceptLink(t, e, tok, lina)          // queue not empty → stored, no push
	a.notif.Wait()
	if n := rec.count("join_requested"); n != 1 {
		t.Fatalf("pushes for a batch of 2 requests: want 1, got %d", n)
	}
	if n := countOf(notificationTypes(t, e, sarah), "join_requested"); n != 2 {
		t.Fatalf("in-app notifications: want 2, got %d", n)
	}

	// Once the owner has cleared the queue, the next request pushes again.
	_, reqs := joinRequests(t, e, sarah, wid)
	for _, r := range reqs {
		decideRequest(t, e, sarah, wid, r["id"].(string), "decline", nil)
	}
	_ = ob
	acceptLink(t, e, tok, zed)
	a.notif.Wait()
	if n := rec.count("join_requested"); n != 2 {
		t.Fatalf("push after the queue emptied: want 2 total, got %d", n)
	}
}

func countOf(xs []string, x string) int {
	n := 0
	for _, v := range xs {
		if v == x {
			n++
		}
	}
	return n
}

// The one-pending-request-per-user guarantee rests on this partial unique
// index; assert it exists as shipped (concurrency tests can't prove it).
func TestPendingIndexIsUniqueAndPartial(t *testing.T) {
	_, db := newAppWithDB(t, nil)
	var def string
	db.Raw("SELECT indexdef FROM pg_indexes WHERE tablename = 'join_requests' AND indexname = 'idx_join_requests_pending'").Scan(&def)
	if !strings.Contains(def, "UNIQUE") || !strings.Contains(def, "(wedding_id, user_id)") || !strings.Contains(def, "'pending'") {
		t.Fatalf("idx_join_requests_pending must be UNIQUE (wedding_id, user_id) WHERE status = 'pending'; got %q", def)
	}
}

// A decision waits for the row lock held by another decision, then finds the
// request already decided (no double decision).
func TestDecisionWaitsForRowLock(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, sarahID := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)
	joins := repository.NewJoinRequestRepo(db)

	tx := db.Begin()
	if err := tx.Exec("SELECT 1 FROM join_requests WHERE id = ? FOR UPDATE", rid).Error; err != nil {
		t.Fatalf("lock: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := joins.Decline(context.Background(), uuid.MustParse(wid), uuid.MustParse(rid), uuid.MustParse(sarahID))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("decline didn't wait for the row lock (returned %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	// The lock holder decides first.
	tx.Exec("UPDATE join_requests SET status = 'approved', decided_at = now() WHERE id = ?", rid)
	tx.Commit()
	if err := <-done; !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("decline after the request was decided: want ErrNotFound, got %v", err)
	}
}

func TestDeclineCooldownExpires(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	tok := editorLink(t, e, sarah, wid)
	_, body := acceptLink(t, e, tok, omar)
	rid, _ := dataOf(body)["request_id"].(string)
	decideRequest(t, e, sarah, wid, rid, "decline", nil)

	db.Exec("UPDATE join_requests SET decided_at = now() - interval '23 hours' WHERE id = ?", rid)
	if code, _ := acceptLink(t, e, tok, omar); code != http.StatusConflict {
		t.Fatalf("23h after a decline: want 409, got %d", code)
	}
	db.Exec("UPDATE join_requests SET decided_at = now() - interval '25 hours' WHERE id = ?", rid)
	code, again := acceptLink(t, e, tok, omar)
	if code != http.StatusAccepted || dataOf(again)["request_id"] == rid {
		t.Fatalf("25h after a decline: want 202 with a new request, got %d %v", code, dataOf(again))
	}
	if s := requestStatus(t, db, rid); s != "declined" {
		t.Fatalf("the old declined request must be kept for history, got %q", s)
	}
}

func TestJoinRequestedNotificationCarriesRequestID(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)

	_, body := do(t, e, http.MethodGet, "/api/v1/notifications", sarah, nil)
	items, _ := dataOf(body)["items"].([]any)
	for _, it := range items {
		n, _ := it.(map[string]any)
		if n["type"] != "join_requested" {
			continue
		}
		data, _ := n["data"].(map[string]any)
		if n["wedding_id"] != wid || data["request_id"] != rid {
			t.Fatalf("join_requested notification: want wedding %s request %s, got %v", wid, rid, n)
		}
		return
	}
	t.Fatalf("owner has no join_requested notification: %v", items)
}

func TestJoinDecisionsAreAudited(t *testing.T) {
	e, db := newAppWithDB(t, nil)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")
	decideRequest(t, e, sarah, wid, pendingRequest(t, e, sarah, wid, omar), "approve", nil)
	decideRequest(t, e, sarah, wid, pendingRequest(t, e, sarah, wid, lina), "decline", nil)

	for _, action := range []string{"join_request_approved", "join_request_declined"} {
		var n int64
		db.Raw("SELECT count(*) FROM audit_logs WHERE action = ?", action).Scan(&n)
		if n != 1 {
			t.Fatalf("audit %s: want 1 row, got %d", action, n)
		}
	}
}

func TestApproveRejectsBadInput(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	rid := pendingRequest(t, e, sarah, wid, omar)
	base := "/api/v1/weddings/" + wid + "/join-requests/"

	if code, _ := do(t, e, http.MethodPost, base+rid+"/approve", sarah, map[string]any{"role": "owner"}); code != http.StatusBadRequest {
		t.Fatalf("approve as owner: want 400, got %d", code)
	}
	if code, _ := do(t, e, http.MethodPost, base+"not-a-uuid/approve", sarah, nil); code != http.StatusNotFound {
		t.Fatalf("malformed request id: want 404, got %d", code)
	}
	req := newReq(http.MethodPost, base+rid+"/approve", sarah, nil)
	req.Body = io.NopCloser(strings.NewReader("{not json"))
	if rec := serve(e, req); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: want 400, got %d", rec.Code)
	}
	if r := myRole(t, e, wid, omar); r != nil {
		t.Fatalf("a rejected approval admitted the user: %v", r)
	}
}

// If requests have been waiting over a day, a new one pushes the owner again
// (otherwise an undrained queue would never notify them again).
func TestOwnerIsPushedAgainWhenTheQueueIsStale(t *testing.T) {
	rec := &recordingSender{}
	a := newTestApp(t, rec)
	e := a.e
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	lina, _ := register(t, e, "lina")
	wid := createWedding(t, e, sarah, "L&O")
	do(t, e, http.MethodPost, "/api/v1/me/devices", sarah, map[string]any{"token": "sarah-device", "platform": "ios"})
	tok := editorLink(t, e, sarah, wid)

	_, ob := acceptLink(t, e, tok, omar)
	omarReq, _ := dataOf(ob)["request_id"].(string)
	a.db.Exec("UPDATE join_requests SET created_at = now() - interval '25 hours' WHERE id = ?", omarReq)
	acceptLink(t, e, tok, lina)
	a.notif.Wait()
	if n := rec.count("join_requested"); n != 2 {
		t.Fatalf("pushes with a request waiting >24h: want 2, got %d", n)
	}
}
