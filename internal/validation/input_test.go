package validation

import "testing"

func TestUsername(t *testing.T) {
	if got := NormalizeUsername("  Leila  "); got != "leila" {
		t.Fatalf("got %q", got)
	}
	for _, value := range []string{"", "ab", "le ila", "leila!"} {
		if err := Username(value); err == nil {
			t.Fatalf("Username(%q) unexpectedly passed", value)
		}
	}
	if err := Username("leila"); err != nil {
		t.Fatal(err)
	}
}

func TestPassword(t *testing.T) {
	for _, value := range []string{"", "short1", "password", "12345678"} {
		if err := Password(value); err == nil {
			t.Fatalf("Password(%q) unexpectedly passed", value)
		}
	}
	if err := Password("Password1!"); err != nil {
		t.Fatal(err)
	}
	if err := Password("Password1"); err == nil {
		t.Fatal("password without a symbol unexpectedly passed")
	}
}
