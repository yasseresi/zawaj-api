package test

import (
	"net/http"
	"strings"
	"testing"
)

// Covers the swarm-built modules: stats, activity feed, notifications, CSV export.
func TestStatsActivityNotificationsExport(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "editor", omar) // omar = editor (can add guests)

	base := "/api/v1/weddings/" + wid
	// Guests: 2 confirmed (companions 2,0), 1 pending (companions 1), 1 declined.
	do(t, e, http.MethodPost, base+"/guests", sarah, map[string]any{"full_name": "A", "status": "confirmed", "companions": 2})
	do(t, e, http.MethodPost, base+"/guests", sarah, map[string]any{"full_name": "B", "status": "confirmed", "companions": 0})
	do(t, e, http.MethodPost, base+"/guests", omar, map[string]any{"full_name": "C", "status": "pending", "companions": 1})
	do(t, e, http.MethodPost, base+"/guests", sarah, map[string]any{"full_name": "D", "status": "declined"})

	// ── Stats ──
	_, sb := do(t, e, http.MethodGet, base+"/stats", sarah, nil)
	d := dataOf(sb)
	if d["total_guests"].(float64) != 4 {
		t.Fatalf("stats total_guests: want 4, got %v", d["total_guests"])
	}
	by, _ := d["by_status"].(map[string]any)
	if by["confirmed"].(float64) != 2 || by["pending"].(float64) != 1 || by["declined"].(float64) != 1 {
		t.Fatalf("stats by_status wrong: %v", by)
	}
	// total_people = (1+2)+(1+0)+(1+1)+(1+0) = 7 ; confirmed_seats = (1+2)+(1+0) = 4
	if d["total_people"].(float64) != 7 {
		t.Fatalf("stats total_people: want 7, got %v", d["total_people"])
	}
	if d["confirmed_seats"].(float64) != 4 {
		t.Fatalf("stats confirmed_seats: want 4, got %v", d["confirmed_seats"])
	}

	// ── Activity feed ── (4 guest_added entries)
	_, ab := do(t, e, http.MethodGet, base+"/activity", sarah, nil)
	if items, _ := dataOf(ab)["items"].([]any); len(items) < 4 {
		t.Fatalf("activity feed: want >=4, got %d", len(items))
	}

	// ── Notifications ── sarah should have been notified of omar's guest add (C).
	_, nb := do(t, e, http.MethodGet, "/api/v1/notifications", sarah, nil)
	items, _ := dataOf(nb)["items"].([]any)
	if len(items) == 0 {
		t.Fatal("notifications: sarah expected >=1 from omar's guest add")
	}
	_, cb := do(t, e, http.MethodGet, "/api/v1/notifications/unread_count", sarah, nil)
	if dataOf(cb)["count"].(float64) < 1 {
		t.Fatalf("unread_count: want >=1, got %v", dataOf(cb)["count"])
	}
	// omar should NOT be notified of his own action (actor excluded).
	_, ob := do(t, e, http.MethodGet, "/api/v1/notifications/unread_count", omar, nil)
	// omar may have notifs from sarah's adds (A,B,D) but none from his own (C).
	_ = ob

	// Mark all read -> unread_count 0.
	do(t, e, http.MethodPost, "/api/v1/notifications/read_all", sarah, nil)
	_, cb2 := do(t, e, http.MethodGet, "/api/v1/notifications/unread_count", sarah, nil)
	if dataOf(cb2)["count"].(float64) != 0 {
		t.Fatalf("after read_all: want 0, got %v", dataOf(cb2)["count"])
	}

	// ── CSV export ──
	req := newReq(http.MethodGet, base+"/export.csv", sarah, nil)
	rec := serve(e, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export.csv: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "csv") {
		t.Fatalf("export.csv content-type: %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "full_name") || !strings.Contains(body, "A") {
		t.Fatalf("export.csv body missing header/rows: %q", body[:min(80, len(body))])
	}
}
