package test

import (
	"net/http"
	"testing"
)

func TestWeddingAndRoles(t *testing.T) {
	e := newApp(t)
	sarah, _ := register(t, e, "sarah")
	omar, omarID := register(t, e, "omar")
	zed, _ := register(t, e, "zed")

	// Owner creates a wedding.
	code, body := do(t, e, http.MethodPost, "/api/v1/weddings", sarah, map[string]any{
		"name": "Layla & Omar", "event_date": "2026-06-12", "description": "t",
	})
	if code != http.StatusCreated {
		t.Fatalf("create wedding: want 201, got %d (%v)", code, body)
	}
	wid, _ := dataOf(body)["id"].(string)

	// List shows owner role + zero guests.
	_, lb := do(t, e, http.MethodGet, "/api/v1/weddings", sarah, nil)
	items, _ := dataOf(lb)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list: want 1 wedding, got %d", len(items))
	}
	if first, _ := items[0].(map[string]any); first["my_role"] != "owner" {
		t.Fatalf("list: want role owner, got %v", first["my_role"])
	}

	// Owner mints an editor invite link.
	_, lkb := do(t, e, http.MethodPost, "/api/v1/weddings/"+wid+"/invite-links", sarah, map[string]any{"role": "editor"})
	tok, _ := dataOf(lkb)["token"].(string)
	if tok == "" {
		t.Fatal("invite-link: empty token")
	}

	// Omar previews then accepts -> becomes editor.
	if code, _ := do(t, e, http.MethodGet, "/api/v1/invite/"+tok, omar, nil); code != http.StatusOK {
		t.Fatalf("preview: want 200, got %d", code)
	}
	_, ab := do(t, e, http.MethodPost, "/api/v1/invite/"+tok+"/accept", omar, nil)
	if dataOf(ab)["role"] != "editor" {
		t.Fatalf("accept: want role editor, got %v", dataOf(ab)["role"])
	}

	// Role enforcement matrix.
	cases := []struct {
		name              string
		method, path, tok string
		body              any
		want              int
	}{
		{"editor reads", http.MethodGet, "/api/v1/weddings/" + wid, omar, nil, http.StatusOK},
		{"editor cant patch", http.MethodPatch, "/api/v1/weddings/" + wid, omar, map[string]any{"name": "x"}, http.StatusForbidden},
		{"nonmember 404", http.MethodGet, "/api/v1/weddings/" + wid, zed, nil, http.StatusNotFound},
		{"owner sets role", http.MethodPatch, "/api/v1/weddings/" + wid + "/members/" + omarID, sarah, map[string]any{"role": "viewer"}, http.StatusOK},
		{"viewer cant delete", http.MethodDelete, "/api/v1/weddings/" + wid, omar, nil, http.StatusForbidden},
		{"owner deletes", http.MethodDelete, "/api/v1/weddings/" + wid, sarah, nil, http.StatusOK},
	}
	for _, tc := range cases {
		if code, b := do(t, e, tc.method, tc.path, tc.tok, tc.body); code != tc.want {
			t.Errorf("%s: want %d, got %d (%v)", tc.name, tc.want, code, b)
		}
	}
}
