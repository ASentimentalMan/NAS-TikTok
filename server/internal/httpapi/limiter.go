package httpapi

import (
	"container/list"
	"crypto/sha256"
	"sync"
	"time"
)

const (
	defaultRateLimiterCapacity = 10000
	loginFailureThreshold      = 5
	loginFailureWindow         = time.Minute
	loginFailureCooldown       = 15 * time.Second
)

type rateEntry struct {
	window        time.Time
	count         int
	failureWindow time.Time
	failures      int
	cooldownUntil time.Time
	element       *list.Element
}

type rateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*rateEntry
	recent   *list.List
	capacity int
}

func newRateLimiter() *rateLimiter {
	return newRateLimiterWithCapacity(defaultRateLimiterCapacity)
}

func newRateLimiterWithCapacity(capacity int) *rateLimiter {
	if capacity < 1 {
		capacity = 1
	}
	return &rateLimiter{
		entries:  make(map[string]*rateEntry),
		recent:   list.New(),
		capacity: capacity,
	}
}

func (limiter *rateLimiter) allow(key string, limit int, now time.Time) bool {
	if limit <= 0 {
		return false
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	window := now.Truncate(time.Minute)
	entry := limiter.touchLocked(key)
	if entry != nil {
		if !entry.cooldownUntil.IsZero() && !entry.cooldownUntil.After(now) {
			entry.cooldownUntil = time.Time{}
			entry.failureWindow = time.Time{}
			entry.failures = 0
		}
		if entry.cooldownUntil.After(now) || (entry.window.Equal(window) && entry.count >= limit) {
			return false
		}
	}

	entry = limiter.getOrCreateLocked(key)
	if !entry.window.Equal(window) {
		entry.window = window
		entry.count = 0
	}
	entry.count++
	return true
}

func (limiter *rateLimiter) allowLoginAccount(account string, limit int, now time.Time) bool {
	return limiter.allow(loginAccountRateKey(account), limit, now)
}

func (limiter *rateLimiter) recordLoginFailure(account string, now time.Time) {
	key := loginAccountRateKey(account)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry := limiter.getOrCreateLocked(key)
	if entry.cooldownUntil.After(now) {
		return
	}
	window := now.Truncate(loginFailureWindow)
	if !entry.failureWindow.Equal(window) {
		entry.failureWindow = window
		entry.failures = 0
	}
	entry.failures++
	if entry.failures >= loginFailureThreshold {
		entry.cooldownUntil = now.Add(loginFailureCooldown)
		entry.failureWindow = time.Time{}
		entry.failures = 0
	}
}

func (limiter *rateLimiter) recordLoginSuccess(account string) {
	key := loginAccountRateKey(account)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	entry := limiter.touchLocked(key)
	if entry == nil {
		return
	}
	entry.failureWindow = time.Time{}
	entry.failures = 0
	entry.cooldownUntil = time.Time{}
}

func loginAccountRateKey(account string) string {
	accountHash := sha256.Sum256([]byte(account))
	return "login:account:" + string(accountHash[:])
}

func (limiter *rateLimiter) touchLocked(key string) *rateEntry {
	entry, exists := limiter.entries[key]
	if !exists {
		return nil
	}
	limiter.recent.MoveToFront(entry.element)
	return entry
}

func (limiter *rateLimiter) getOrCreateLocked(key string) *rateEntry {
	if entry := limiter.touchLocked(key); entry != nil {
		return entry
	}
	if len(limiter.entries) >= limiter.capacity {
		oldest := limiter.recent.Back()
		if oldest != nil {
			oldestKey := oldest.Value.(string)
			delete(limiter.entries, oldestKey)
			limiter.recent.Remove(oldest)
		}
	}
	entry := &rateEntry{}
	limiter.entries[key] = entry
	entry.element = limiter.recent.PushFront(key)
	return entry
}
