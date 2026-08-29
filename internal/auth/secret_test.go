package auth

import "testing"

func TestHashAndVerifySecret(t *testing.T) {
	hash, err := HashSecret("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "correct horse battery staple" {
		t.Fatal("hash equals plaintext")
	}
	if !VerifySecret(hash, "correct horse battery staple") {
		t.Fatal("verify: correct secret rejected")
	}
	if VerifySecret(hash, "wrong secret") {
		t.Fatal("verify: wrong secret accepted")
	}
}

func TestGenerateRecoveryCode(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		code, err := GenerateRecoveryCode()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if len(code) != 16 {
			t.Fatalf("recovery code length: want 16, got %d (%q)", len(code), code)
		}
		if seen[code] {
			t.Fatalf("duplicate recovery code %q", code)
		}
		seen[code] = true
	}
}
