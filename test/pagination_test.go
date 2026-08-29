package test

import (
	"net/http"
	"testing"
)

// itemIDs pulls the id field out of each item in a {items:[...]} data envelope.
func itemIDs(data map[string]any) []string {
	items, _ := data["items"].([]any)
	ids := make([]string, 0, len(items))
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			if id, ok := m["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func TestActivityFeed_CursorPagination(t *testing.T) {
	e := newApp(t)
	owner, _ := register(t, e, "page_owner")
	wid := createWedding(t, e, owner, "Paging Wedding")

	// Three guest additions => three activity rows.
	for _, name := range []string{"A", "B", "C"} {
		if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/guests", owner, map[string]any{"full_name": name}); code != http.StatusCreated {
			t.Fatalf("add guest %s: want 201, got %d", name, code)
		}
	}

	base := "/api/v1/weddings/" + wid + "/activity?page_size=2"

	// Page 1: 2 items + a next_cursor.
	code, body := do(t, e, "GET", base, owner, nil)
	if code != http.StatusOK {
		t.Fatalf("page1: want 200, got %d", code)
	}
	d := dataOf(body)
	page1 := itemIDs(d)
	if len(page1) != 2 {
		t.Fatalf("page1: want 2 items, got %d", len(page1))
	}
	cursor, _ := d["next_cursor"].(string)
	if cursor == "" {
		t.Fatal("page1: expected a next_cursor")
	}

	// Page 2: remaining 1 item, no further cursor, no overlap with page 1.
	code, body = do(t, e, "GET", base+"&cursor="+cursor, owner, nil)
	if code != http.StatusOK {
		t.Fatalf("page2: want 200, got %d", code)
	}
	d = dataOf(body)
	page2 := itemIDs(d)
	if len(page2) != 1 {
		t.Fatalf("page2: want 1 item, got %d", len(page2))
	}
	if d["next_cursor"] != nil {
		t.Fatalf("page2: expected null next_cursor, got %v", d["next_cursor"])
	}
	for _, a := range page1 {
		if a == page2[0] {
			t.Fatalf("page2 item %s overlaps page1", a)
		}
	}
}
