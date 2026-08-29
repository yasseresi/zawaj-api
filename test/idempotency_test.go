package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// doKey performs a request with an Idempotency-Key header and returns status +
// the raw response body.
func doKey(t *testing.T, e interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, method, path, token, idemKey string, body any) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func TestIdempotency_ReplaysCreate(t *testing.T) {
	e := newApp(t)
	owner, _ := register(t, e, "idem_owner")
	wid := createWedding(t, e, owner, "Idem Wedding")

	path := "/api/v1/weddings/" + wid + "/guests"
	guest := map[string]any{"full_name": "Amina"}

	// First create with a key.
	code1, body1 := doKey(t, e, "POST", path, owner, "key-abc", guest)
	if code1 != http.StatusCreated {
		t.Fatalf("first create: want 201, got %d", code1)
	}

	// Same key + same body → replay of the identical response, no new guest.
	code2, body2 := doKey(t, e, "POST", path, owner, "key-abc", guest)
	if code2 != http.StatusCreated {
		t.Fatalf("replay: want 201, got %d", code2)
	}
	if !bytes.Equal(body1, body2) {
		t.Fatalf("replay body differs from original:\n%s\n%s", body1, body2)
	}

	// Exactly one guest should exist.
	if code, list := do(t, e, "GET", path, owner, nil); code == http.StatusOK {
		items, _ := dataOf(list)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("want 1 guest after replay, got %d", len(items))
		}
	} else {
		t.Fatalf("list guests: want 200, got %d", code)
	}
}

func TestIdempotency_KeyReuseDifferentBodyConflicts(t *testing.T) {
	e := newApp(t)
	owner, _ := register(t, e, "idem_owner2")
	wid := createWedding(t, e, owner, "Idem Wedding 2")
	path := "/api/v1/weddings/" + wid + "/guests"

	if code, _ := doKey(t, e, "POST", path, owner, "key-x", map[string]any{"full_name": "First"}); code != http.StatusCreated {
		t.Fatalf("first: want 201, got %d", code)
	}
	// Same key, different body → 409.
	if code, _ := doKey(t, e, "POST", path, owner, "key-x", map[string]any{"full_name": "Second"}); code != http.StatusConflict {
		t.Fatalf("reuse with different body: want 409, got %d", code)
	}
}
