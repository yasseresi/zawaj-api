package token

import "testing"

func TestNewLengthAndUniqueness(t *testing.T) {
	// 10 bytes -> 16 base32 chars, 15 -> 24.
	if s, _ := New(10); len(s) != 16 {
		t.Fatalf("New(10) length: want 16, got %d (%q)", len(s), s)
	}
	if s, _ := New(15); len(s) != 24 {
		t.Fatalf("New(15) length: want 24, got %d (%q)", len(s), s)
	}

	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		s, err := New(15)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if seen[s] {
			t.Fatalf("duplicate token %q", s)
		}
		seen[s] = true
	}
}
