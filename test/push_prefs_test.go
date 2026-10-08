package test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"zawaj/internal/push"
)

// recordingSender captures every push the notification service sends.
type recordingSender struct {
	mu   sync.Mutex
	sent []sentPush
}

type sentPush struct {
	typ    string
	tokens []string
}

func (r *recordingSender) Send(_ context.Context, tokens []string, msg push.Message) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, sentPush{typ: msg.Data["type"], tokens: tokens})
	return nil
}

// count reports how many pushes of typ were sent.
func (r *recordingSender) count(typ string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, p := range r.sent {
		if p.typ == typ {
			n++
		}
	}
	return n
}

// waitFor polls until a push of typ arrives (push delivery is asynchronous).
func (r *recordingSender) waitFor(typ string, d time.Duration) (sentPush, bool) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		for _, p := range r.sent {
			if p.typ == typ {
				r.mu.Unlock()
				return p, true
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	return sentPush{}, false
}

// notif_rsvp=false must stop RSVP-change pushes while other pushes (and the
// in-app notification) still go through.
func TestPushHonorsRSVPPreference(t *testing.T) {
	rec := &recordingSender{}
	e := newAppWithPusher(t, rec)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "editor", omar)

	if code, _ := do(t, e, http.MethodPost, "/api/v1/me/devices", omar, map[string]any{"token": "omar-device", "platform": "ios"}); code >= 300 {
		t.Fatalf("register device: %d", code)
	}
	if code, _ := do(t, e, http.MethodPatch, "/api/v1/me/settings", omar, map[string]any{"notif_rsvp": false}); code != http.StatusOK {
		t.Fatalf("disable rsvp: %d", code)
	}

	_, body := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/guests", sarah, map[string]any{"full_name": "Amina"})
	gid, _ := dataOf(body)["id"].(string)
	if p, ok := rec.waitFor("guest_added", 2*time.Second); !ok || len(p.tokens) != 1 || p.tokens[0] != "omar-device" {
		t.Fatalf("guest_added push should still reach omar: %+v ok=%v", p, ok)
	}

	if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/guests/"+gid+"/status", sarah, map[string]any{"status": "confirmed"}); code != http.StatusOK {
		t.Fatalf("set status: %d", code)
	}
	if p, ok := rec.waitFor("guest_status_changed", 500*time.Millisecond); ok {
		t.Fatalf("rsvp push sent despite notif_rsvp=false: %+v", p)
	}

	// The in-app notification is still recorded.
	_, body = do(t, e, http.MethodGet, "/api/v1/notifications", omar, nil)
	items, _ := dataOf(body)["items"].([]any)
	found := false
	for _, it := range items {
		if m, ok := it.(map[string]any); ok && m["type"] == "guest_status_changed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("in-app rsvp notification missing: %v", body)
	}

	// Re-enabling RSVP updates restores the push.
	do(t, e, http.MethodPatch, "/api/v1/me/settings", omar, map[string]any{"notif_rsvp": true})
	do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/guests/"+gid+"/status", sarah, map[string]any{"status": "declined"})
	if _, ok := rec.waitFor("guest_status_changed", 2*time.Second); !ok {
		t.Fatal("rsvp push missing after re-enabling notif_rsvp")
	}
}
