package test

import (
	"net/http"
	"testing"
)

// TestReminders covers POST /weddings/:id/reminders: it returns the pending
// guest count, requires editor role, and records a reminders_sent activity.
func TestReminders(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "viewer", omar) // omar = viewer (cannot send)

	guests := "/api/v1/weddings/" + wid + "/guests"
	do(t, e, http.MethodPost, guests, sarah, map[string]any{"full_name": "هند"})                        // pending (default)
	do(t, e, http.MethodPost, guests, sarah, map[string]any{"full_name": "يوسف"})                       // pending
	do(t, e, http.MethodPost, guests, sarah, map[string]any{"full_name": "خالد", "status": "confirmed"}) // not pending

	reminders := "/api/v1/weddings/" + wid + "/reminders"

	// Viewer is forbidden.
	if code, _ := do(t, e, http.MethodPost, reminders, omar, nil); code != http.StatusForbidden {
		t.Fatalf("viewer send reminders: want 403, got %d", code)
	}

	// Editor/owner succeeds and counts only pending guests.
	code, body := do(t, e, http.MethodPost, reminders, sarah, nil)
	if code != http.StatusOK {
		t.Fatalf("send reminders: want 200, got %d (%v)", code, body)
	}
	if got := dataOf(body)["count"].(float64); got != 2 {
		t.Fatalf("reminders count: want 2, got %v", got)
	}

	// A reminders_sent entry lands in the activity feed.
	_, ab := do(t, e, http.MethodGet, "/api/v1/weddings/"+wid+"/activity", sarah, nil)
	items, _ := dataOf(ab)["items"].([]any)
	found := false
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok || m["action"] != "reminders_sent" {
			continue
		}
		found = true
		// Feed is enriched with the actor's display name.
		if m["actor_name"] != "sarah" {
			t.Fatalf("reminders_sent actor_name: want sarah, got %v", m["actor_name"])
		}
		// Meta serializes as a real JSON object (not base64) carrying the count.
		meta, _ := m["meta"].(map[string]any)
		if meta == nil || meta["count"].(float64) != 2 {
			t.Fatalf("reminders_sent meta.count: want 2, got %v", m["meta"])
		}
		break
	}
	if !found {
		t.Fatalf("expected reminders_sent activity entry, feed: %v", items)
	}
}
