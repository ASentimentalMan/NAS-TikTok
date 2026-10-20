package captcha

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"math/big"
	"strings"
	"sync"
	"time"
)

type Response struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type entry struct {
	answer  string
	expires time.Time
}

type Service struct {
	mu      sync.Mutex
	entries map[string]entry
	ttl     time.Duration
	max     int
	now     func() time.Time
}

func New(ttl time.Duration, maximum int) (*Service, error) {
	if ttl <= 0 || maximum <= 0 {
		return nil, errors.New("captcha TTL and storage limit must be positive")
	}
	return &Service{entries: make(map[string]entry), ttl: ttl, max: maximum, now: time.Now}, nil
}

func (service *Service) Create() (Response, error) {
	id, err := secureID(24)
	if err != nil {
		return Response{}, err
	}
	left, right, operator, answer, err := expression()
	if err != nil {
		return Response{}, err
	}
	markup := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="180" height="56" viewBox="0 0 180 56"><rect width="180" height="56" fill="#141414"/><path d="M5 42 Q45 5 90 39 T175 14" fill="none" stroke="#545454" stroke-width="2"/><text x="90" y="37" text-anchor="middle" font-family="sans-serif" font-size="26" font-weight="700" fill="#f5f5f5">%s %s %s = ?</text></svg>`, left, html.EscapeString(operator), right)
	data := "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(markup))

	now := service.now()
	service.mu.Lock()
	defer service.mu.Unlock()
	service.expire(now)
	if len(service.entries) >= service.max {
		return Response{}, errors.New("captcha storage is full")
	}
	service.entries[id] = entry{answer: answer, expires: now.Add(service.ttl)}
	return Response{ID: id, Data: data}, nil
}

func (service *Service) Consume(id, answer string) bool {
	now := service.now()
	service.mu.Lock()
	defer service.mu.Unlock()
	item, exists := service.entries[id]
	delete(service.entries, id)
	if !exists || !now.Before(item.expires) {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	expected := strings.ToLower(item.answer)
	if len(answer) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(answer), []byte(expected)) == 1
}

func (service *Service) expire(now time.Time) {
	for id, item := range service.entries {
		if !now.Before(item.expires) {
			delete(service.entries, id)
		}
	}
}

func expression() (string, string, string, string, error) {
	first, err := randomInt(1, 10)
	if err != nil {
		return "", "", "", "", err
	}
	second, err := randomInt(1, 10)
	if err != nil {
		return "", "", "", "", err
	}
	operator, err := randomInt(0, 2)
	if err != nil {
		return "", "", "", "", err
	}
	if operator == 0 {
		return fmt.Sprint(first), fmt.Sprint(second), "+", fmt.Sprint(first + second), nil
	}
	if first < second {
		first, second = second, first
	}
	return fmt.Sprint(first), fmt.Sprint(second), "-", fmt.Sprint(first - second), nil
}

func randomInt(minimum, maximum int64) (int64, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(maximum-minimum))
	if err != nil {
		return 0, err
	}
	return value.Int64() + minimum, nil
}

func secureID(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
