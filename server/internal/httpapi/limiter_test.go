package httpapi

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiterEnforcesConcurrentLimit(t *testing.T) {
	limiter := newRateLimiter()
	var group sync.WaitGroup
	var successes int
	var mu sync.Mutex
	for index := 0; index < 100; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if limiter.allow("test-client", 7, time.Now()) {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	group.Wait()
	if successes != 7 {
		t.Fatalf("allowed requests = %d, want 7", successes)
	}
}

func TestLoginRateLimiterUsesIPAndExactAccountDimensions(t *testing.T) {
	limiter := newRateLimiter()
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	allow := func(ip, account string) bool {
		if !limiter.allow("login:ip:"+ip, 2, now) {
			return false
		}
		return limiter.allowLoginAccount(account, 2, now)
	}

	if !allow("192.0.2.1", "alice") || !allow("192.0.2.2", "alice") {
		t.Fatal("requests below the account limit were rejected")
	}
	if allow("192.0.2.3", "alice") {
		t.Fatal("account limit did not apply across source IPs")
	}
	if !allow("192.0.2.4", "Alice") {
		t.Fatal("case-distinct account was treated as the same identity")
	}

	if !allow("198.51.100.1", "bob") || !allow("198.51.100.1", "carol") {
		t.Fatal("requests below the source-IP limit were rejected")
	}
	if allow("198.51.100.1", "dave") {
		t.Fatal("source-IP limit did not apply across account names")
	}
}

func TestRateLimiterEvictsOldEntriesInsteadOfRejectingNewKeys(t *testing.T) {
	limiter := newRateLimiterWithCapacity(2)
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	if !limiter.allow("first", 1, now) || !limiter.allow("second", 1, now) {
		t.Fatal("initial keys were rejected")
	}
	if !limiter.allow("first", 2, now) {
		t.Fatal("active key was rejected")
	}
	if !limiter.allow("third", 1, now) {
		t.Fatal("new key was rejected when the bounded store was full")
	}
	if len(limiter.entries) != 2 {
		t.Fatalf("stored entries = %d, want capacity 2", len(limiter.entries))
	}
	if limiter.allow("first", 2, now) {
		t.Fatal("recently used entry was evicted instead of the least-recent entry")
	}
	if !limiter.allow("second", 1, now) {
		t.Fatal("evicted key could not be admitted as a new client")
	}
	if len(limiter.entries) != 2 {
		t.Fatalf("stored entries after churn = %d, want capacity 2", len(limiter.entries))
	}
}

func TestLoginFailureCooldownExpires(t *testing.T) {
	limiter := newRateLimiter()
	start := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	for attempt := 0; attempt < loginFailureThreshold; attempt++ {
		at := start.Add(time.Duration(attempt) * time.Second)
		if !limiter.allowLoginAccount("owner", 100, at) {
			t.Fatalf("attempt %d was rejected before the cooldown threshold", attempt+1)
		}
		limiter.recordLoginFailure("owner", at)
	}
	cooldownStart := start.Add(time.Duration(loginFailureThreshold-1) * time.Second)
	if limiter.allowLoginAccount("owner", 100, cooldownStart.Add(loginFailureCooldown-time.Second)) {
		t.Fatal("account was admitted before its failure cooldown expired")
	}
	if !limiter.allowLoginAccount("owner", 100, cooldownStart.Add(loginFailureCooldown+time.Second)) {
		t.Fatal("account remained blocked after its failure cooldown expired")
	}
}

func TestLoginSuccessResetsFailureStreakButPreservesRequestCount(t *testing.T) {
	limiter := newRateLimiter()
	start := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	for attempt := 0; attempt < loginFailureThreshold-1; attempt++ {
		at := start.Add(time.Duration(attempt) * time.Second)
		if !limiter.allowLoginAccount("owner", 100, at) {
			t.Fatalf("failed attempt %d was rejected", attempt+1)
		}
		limiter.recordLoginFailure("owner", at)
	}

	successAt := start.Add(time.Duration(loginFailureThreshold-1) * time.Second)
	if !limiter.allowLoginAccount("owner", 100, successAt) {
		t.Fatal("successful login was rejected")
	}
	limiter.recordLoginSuccess("owner")

	entry := limiter.entries[loginAccountRateKey("owner")]
	if entry.count != loginFailureThreshold {
		t.Fatalf("request count after success = %d, want %d", entry.count, loginFailureThreshold)
	}
	if entry.failures != 0 || !entry.failureWindow.IsZero() || !entry.cooldownUntil.IsZero() {
		t.Fatalf("failure state after success = failures:%d window:%v cooldown:%v, want cleared", entry.failures, entry.failureWindow, entry.cooldownUntil)
	}

	for attempt := 0; attempt < loginFailureThreshold-1; attempt++ {
		at := successAt.Add(time.Duration(attempt+1) * time.Second)
		if !limiter.allowLoginAccount("owner", 100, at) {
			t.Fatalf("failed attempt after success %d was rejected", attempt+1)
		}
		limiter.recordLoginFailure("owner", at)
	}
	if entry.failures != loginFailureThreshold-1 || !entry.cooldownUntil.IsZero() {
		t.Fatalf("failure streak after successful login = failures:%d cooldown:%v", entry.failures, entry.cooldownUntil)
	}
}
