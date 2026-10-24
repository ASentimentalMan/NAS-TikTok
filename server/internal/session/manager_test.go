package session

import (
	"strings"
	"testing"
	"time"
)

func TestSessionCreateValidateTamperAndRevoke(t *testing.T) {
	manager, err := New([]byte(strings.Repeat("s", 32)), time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	claims, ok := manager.Validate(token)
	if !ok || claims.Account != "owner" {
		t.Fatal("new session failed validation")
	}
	if _, ok := manager.Validate(token + "x"); ok {
		t.Fatal("tampered session validated")
	}
	if !manager.Revoke(token) {
		t.Fatal("session could not be revoked")
	}
	if _, ok := manager.Validate(token); ok {
		t.Fatal("revoked session validated")
	}
}

func TestSessionExpiresAndRequiresItsSigningKey(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	manager, err := New(secret, time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now }
	token, _, err := manager.Create("owner")
	if err != nil {
		t.Fatal(err)
	}

	otherKey, err := New([]byte(strings.Repeat("x", 32)), time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	otherKey.now = func() time.Time { return now }
	if _, ok := otherKey.Validate(token); ok {
		t.Fatal("session validated with a different signing key")
	}

	now = now.Add(2 * time.Minute)
	if _, ok := manager.Validate(token); ok {
		t.Fatal("expired session validated")
	}
}

func TestNewRejectsInvalidSigningKey(t *testing.T) {
	if _, err := New([]byte("too short"), time.Minute, 2); err == nil {
		t.Fatal("short signing key was accepted")
	}
}

func TestCookiePathContainsOnlyAPI(t *testing.T) {
	if got := CookiePath("/photos/"); got != "/photos/api" {
		t.Fatalf("CookiePath() = %q", got)
	}
}
