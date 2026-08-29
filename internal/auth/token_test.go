package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestManager() *Manager {
	return NewManager("access-secret-abcdefghijklmnop", "refresh-secret-abcdefghijklmnop", 15*time.Minute, 24*time.Hour)
}

func TestIssueAndParseRoundTrip(t *testing.T) {
	m := newTestManager()
	uid := uuid.New()

	pair, err := m.Issue(uid)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	gotAccess, err := m.ParseAccess(pair.Access)
	if err != nil || gotAccess != uid {
		t.Fatalf("parse access: got %s err %v", gotAccess, err)
	}

	gotUID, gotJTI, err := m.ParseRefresh(pair.Refresh)
	if err != nil || gotUID != uid {
		t.Fatalf("parse refresh: got %s err %v", gotUID, err)
	}
	if gotJTI != pair.RefreshJTI || gotJTI == uuid.Nil {
		t.Fatalf("refresh jti mismatch: got %s want %s", gotJTI, pair.RefreshJTI)
	}
	if pair.RefreshExpiry.Before(time.Now()) {
		t.Fatal("refresh expiry should be in the future")
	}
}

func TestTokenTypeConfusionRejected(t *testing.T) {
	m := newTestManager()
	pair, _ := m.Issue(uuid.New())

	// A refresh token must not validate as an access token, and vice versa.
	if _, err := m.ParseAccess(pair.Refresh); err == nil {
		t.Fatal("refresh token accepted as access token")
	}
	if _, _, err := m.ParseRefresh(pair.Access); err == nil {
		t.Fatal("access token accepted as refresh token")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	// Negative TTL => token is already expired at issue time.
	m := NewManager("a-secret-abcdefghijklmnop", "r-secret-abcdefghijklmnop", -time.Minute, -time.Minute)
	pair, _ := m.Issue(uuid.New())
	if _, err := m.ParseAccess(pair.Access); err == nil {
		t.Fatal("expired access token accepted")
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	m := newTestManager()
	pair, _ := m.Issue(uuid.New())
	tampered := pair.Access[:len(pair.Access)-2] + "xy"
	if _, err := m.ParseAccess(tampered); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestWrongSecretRejected(t *testing.T) {
	m := newTestManager()
	pair, _ := m.Issue(uuid.New())
	other := NewManager("different-secret-abcdefghijkl", "different-refresh-abcdefghijkl", time.Minute, time.Minute)
	if _, err := other.ParseAccess(pair.Access); err == nil {
		t.Fatal("token validated under a different secret")
	}
}

func TestGarbageTokenRejected(t *testing.T) {
	m := newTestManager()
	for _, s := range []string{"", "not.a.jwt", "a.b.c"} {
		if _, err := m.ParseAccess(s); err == nil {
			t.Fatalf("garbage token %q accepted", s)
		}
	}
}
