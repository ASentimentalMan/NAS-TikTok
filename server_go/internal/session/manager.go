package session

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const CookieName = "nas_tiktok_session"

var ErrStoreFull = errors.New("session storage is full")

type Claims struct {
	Account string `json:"sub"`
	ID      string `json:"sid"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

type Manager struct {
	secret []byte
	ttl    time.Duration
	max    int
	now    func() time.Time
	mu     sync.Mutex
	active map[string]time.Time
}

func New(secret []byte, ttl time.Duration, maximum int) (*Manager, error) {
	if len(secret) < 32 {
		return nil, errors.New("session signing key must contain at least 32 bytes")
	}
	if ttl <= 0 || maximum <= 0 {
		return nil, errors.New("session TTL and storage limit must be positive")
	}
	return &Manager{secret: append([]byte(nil), secret...), ttl: ttl, max: maximum, now: time.Now, active: make(map[string]time.Time)}, nil
}

func (manager *Manager) Create(account string) (string, time.Time, error) {
	var identifier [32]byte
	if _, err := rand.Read(identifier[:]); err != nil {
		return "", time.Time{}, err
	}
	now := manager.now()
	expires := time.Unix(now.Add(manager.ttl).Unix(), 0)
	claims := Claims{Account: account, ID: base64.RawURLEncoding.EncodeToString(identifier[:]), Issued: now.Unix(), Expires: expires.Unix()}
	encodedClaims, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	const header = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9"
	payload := base64.RawURLEncoding.EncodeToString(encodedClaims)
	unsigned := header + "." + payload
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(manager.sign(unsigned))

	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.expire(now)
	if len(manager.active) >= manager.max {
		return "", time.Time{}, ErrStoreFull
	}
	manager.active[claims.ID] = expires
	return token, expires, nil
}

func (manager *Manager) Validate(token string) (Claims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" {
		return Claims{}, false
	}
	unsigned := parts[0] + "." + parts[1]
	provided, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, false
	}
	expected := manager.sign(unsigned)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		return Claims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, false
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ID == "" || claims.Account == "" {
		return Claims{}, false
	}
	now := manager.now()
	if claims.Expires <= now.Unix() || claims.Issued > now.Add(time.Minute).Unix() {
		return Claims{}, false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.expire(now)
	expires, active := manager.active[claims.ID]
	if !active || !expires.Equal(time.Unix(claims.Expires, 0)) || !now.Before(expires) {
		return Claims{}, false
	}
	return claims, true
}

func (manager *Manager) Revoke(token string) bool {
	claims, valid := manager.Validate(token)
	if !valid {
		return false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, exists := manager.active[claims.ID]; !exists {
		return false
	}
	delete(manager.active, claims.ID)
	return true
}

func (manager *Manager) sign(value string) []byte {
	mac := hmac.New(sha256.New, manager.secret)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (manager *Manager) expire(now time.Time) {
	for id, expires := range manager.active {
		if !now.Before(expires) {
			delete(manager.active, id)
		}
	}
}

func CookiePath(basePath string) string {
	return fmt.Sprintf("%s/api", strings.TrimSuffix(basePath, "/"))
}
