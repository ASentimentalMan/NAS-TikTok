package captcha

import (
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCaptchaIsSvgAndConsumedOnlyOnce(t *testing.T) {
	service, err := New(time.Minute, 4)
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.Create()
	if err != nil {
		t.Fatal(err)
	}
	encoded := strings.TrimPrefix(response.Data, "data:image/svg+xml;base64,")
	markup, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || !strings.Contains(string(markup), "<svg") {
		t.Fatalf("invalid SVG data URI: %v", err)
	}
	answer := captchaAnswer(t, string(markup))
	if !service.Consume(response.ID, answer) {
		t.Fatal("correct captcha answer was rejected")
	}
	if service.Consume(response.ID, answer) {
		t.Fatal("captcha answer was accepted twice")
	}
}

func TestCaptchaWrongAnswerConsumesChallenge(t *testing.T) {
	service, _ := New(time.Minute, 4)
	response, err := service.Create()
	if err != nil {
		t.Fatal(err)
	}
	answer := captchaAnswer(t, decodeSVG(t, response.Data))
	wrong := answer + "x"
	if service.Consume(response.ID, wrong) || service.Consume(response.ID, answer) {
		t.Fatal("failed captcha attempt did not consume the challenge")
	}
}

func TestCaptchaExpires(t *testing.T) {
	service, _ := New(time.Second, 4)
	response, err := service.Create()
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.now = func() time.Time { return time.Now().Add(2 * time.Second) }
	service.mu.Unlock()
	if service.Consume(response.ID, "1") {
		t.Fatal("expired challenge was accepted")
	}
}

func TestCaptchaCanOnlyBeConsumedOnceConcurrently(t *testing.T) {
	service, _ := New(time.Minute, 4)
	response, err := service.Create()
	if err != nil {
		t.Fatal(err)
	}
	answer := captchaAnswer(t, decodeSVG(t, response.Data))
	var group sync.WaitGroup
	var successes int
	var mu sync.Mutex
	for index := 0; index < 32; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if service.Consume(response.ID, answer) {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	group.Wait()
	if successes != 1 {
		t.Fatalf("successful concurrent captcha consumptions = %d, want 1", successes)
	}
}

func decodeSVG(t *testing.T, data string) string {
	t.Helper()
	encoded := strings.TrimPrefix(data, "data:image/svg+xml;base64,")
	markup, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return string(markup)
}

func captchaAnswer(t *testing.T, markup string) string {
	t.Helper()
	match := regexp.MustCompile(`>([0-9]+) ([+-]) ([0-9]+) = \?</text>`).FindStringSubmatch(markup)
	if len(match) != 4 {
		t.Fatalf("captcha expression not found")
	}
	left, _ := strconv.Atoi(match[1])
	right, _ := strconv.Atoi(match[3])
	if match[2] == "+" {
		return strconv.Itoa(left + right)
	}
	return strconv.Itoa(left - right)
}
