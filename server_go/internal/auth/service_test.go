package auth

import "testing"

func TestVerifyRequiresBothAccountAndPassword(t *testing.T) {
	service := New("owner", "a test password")
	if !service.Verify("owner", "a test password") {
		t.Fatal("valid credentials were rejected")
	}
	if service.Verify("owner", "wrong") || service.Verify("other", "a test password") {
		t.Fatal("invalid credentials were accepted")
	}
}
