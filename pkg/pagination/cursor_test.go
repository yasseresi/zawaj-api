package pagination

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	ts := time.Unix(0, 1_726_000_000_123_456_789)

	c, ok, err := Decode(Encode(ts, id))
	if err != nil || !ok {
		t.Fatalf("decode: ok=%v err=%v", ok, err)
	}
	if !c.CreatedAt.Equal(ts) {
		t.Fatalf("time mismatch: want %v got %v", ts, c.CreatedAt)
	}
	if c.ID != id {
		t.Fatalf("id mismatch: want %s got %s", id, c.ID)
	}
}

func TestDecodeEmptyIsFirstPage(t *testing.T) {
	_, ok, err := Decode("")
	if err != nil {
		t.Fatalf("empty cursor: unexpected err %v", err)
	}
	if ok {
		t.Fatal("empty cursor should report ok=false (first page)")
	}
}

func TestDecodeInvalid(t *testing.T) {
	for _, s := range []string{"not-base64!!", "Zm9v", "bad:parts:here"} {
		if _, _, err := Decode(s); err == nil {
			t.Fatalf("expected error decoding %q", s)
		}
	}
}
