package test

import (
	"net/http"
	"testing"
)

func TestGuests(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, _ := register(t, e, "omar")
	wid := createWedding(t, e, sarah, "L&O")
	joinAs(t, e, sarah, wid, "viewer", omar) // omar = viewer

	base := "/api/v1/weddings/" + wid + "/guests"

	// Create three guests with different statuses.
	code, body := do(t, e, http.MethodPost, base, sarah, map[string]any{
		"full_name": "هند", "contact": "hind@example.com", "status": "confirmed", "companions": 2, "note": "t+2",
	})
	if code != http.StatusCreated {
		t.Fatalf("create guest: want 201, got %d (%v)", code, body)
	}
	gid, _ := dataOf(body)["id"].(string)
	do(t, e, http.MethodPost, base, sarah, map[string]any{"full_name": "يوسف", "companions": 1})
	do(t, e, http.MethodPost, base, sarah, map[string]any{"full_name": "خالد", "status": "declined"})

	// Tab counts.
	_, lb := do(t, e, http.MethodGet, base, sarah, nil)
	counts, _ := dataOf(lb)["counts"].(map[string]any)
	if counts["all"].(float64) != 3 || counts["confirmed"].(float64) != 1 || counts["declined"].(float64) != 1 || counts["pending"].(float64) != 1 {
		t.Fatalf("counts wrong: %v", counts)
	}

	// Filter + search.
	_, fb := do(t, e, http.MethodGet, base+"?status=confirmed", sarah, nil)
	if items, _ := dataOf(fb)["items"].([]any); len(items) != 1 {
		t.Fatalf("filter confirmed: want 1, got %d", len(items))
	}
	_, sb := do(t, e, http.MethodGet, base+"?q=%D8%AE%D8%A7%D9%84%D8%AF", sarah, nil) // "خالد"
	if items, _ := dataOf(sb)["items"].([]any); len(items) != 1 {
		t.Fatalf("search خالد: want 1, got %d", len(items))
	}

	// Note + status change generate activity/history.
	do(t, e, http.MethodPost, base+"/"+gid+"/notes", sarah, map[string]any{"body": "no nuts"})
	do(t, e, http.MethodPatch, base+"/"+gid+"/status", sarah, map[string]any{"status": "pending"})

	_, db := do(t, e, http.MethodGet, base+"/"+gid, sarah, nil)
	d := dataOf(db)
	notes, _ := d["notes"].([]any)
	history, _ := d["history"].([]any)
	guest, _ := d["guest"].(map[string]any)
	if guest["status"] != "pending" {
		t.Fatalf("status: want pending, got %v", guest["status"])
	}
	if len(notes) != 2 { // initial "t+2" + "no nuts"
		t.Fatalf("notes: want 2, got %d", len(notes))
	}
	if len(history) < 3 { // guest_added, note_added, guest_status_changed
		t.Fatalf("history: want >=3, got %d", len(history))
	}

	// Role enforcement: viewer cannot write, can read; bad status = 400.
	cases := []struct {
		name, method, path, tok string
		body                    any
		want                    int
	}{
		{"viewer create", http.MethodPost, base, omar, map[string]any{"full_name": "x"}, http.StatusForbidden},
		{"viewer list", http.MethodGet, base, omar, nil, http.StatusOK},
		{"bad status", http.MethodPost, base, sarah, map[string]any{"full_name": "y", "status": "maybe"}, http.StatusBadRequest},
		{"missing name", http.MethodPost, base, sarah, map[string]any{"companions": 1}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if code, b := do(t, e, tc.method, tc.path, tc.tok, tc.body); code != tc.want {
			t.Errorf("%s: want %d, got %d (%v)", tc.name, tc.want, code, b)
		}
	}
}
