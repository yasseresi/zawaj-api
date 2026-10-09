package test

import (
	"context"
	"net/http"
	"sync"
	"testing"

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

// find returns the first push of typ. Call NotificationService.Wait first:
// delivery is asynchronous.
func (r *recordingSender) find(typ string) (sentPush, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.sent {
		if p.typ == typ {
			return p, true
		}
	}
	return sentPush{}, false
}

// notif_rsvp=false must stop RSVP-change pushes while other pushes (and the
// in-app notification) still go through.
func TestPushHonorsRSVPPreference(t *testing.T) {
	rec := &recordingSender{}
	a := newTestApp(t, rec)
	e := a.e
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
	a.notif.Wait()
	if p, ok := rec.find("guest_added"); !ok || len(p.tokens) != 1 || p.tokens[0] != "omar-device" {
		t.Fatalf("guest_added push should still reach omar: %+v ok=%v", p, ok)
	}

	if code, _ := do(t, e, http.MethodPatch, "/api/v1/weddings/"+wid+"/guests/"+gid+"/status", sarah, map[string]any{"status": "confirmed"}); code != http.StatusOK {
		t.Fatalf("set status: %d", code)
	}
	a.notif.Wait() // every push this request started has been sent (or skipped)
	if p, ok := rec.find("guest_status_changed"); ok {
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
	a.notif.Wait()
	if _, ok := rec.find("guest_status_changed"); !ok {
		t.Fatal("rsvp push missing after re-enabling notif_rsvp")
	}
}
