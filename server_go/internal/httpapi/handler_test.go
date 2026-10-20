package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/auth"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/captcha"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/config"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/session"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/web"
)

func TestCookieLoginSessionListRangeAndLogout(t *testing.T) {
	mediaRoot := t.TempDir()
	mediaPath := filepath.Join(mediaRoot, "假期 folder", "clip 01.mp4")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		BasePath: "/photos", PublicAPIBase: "/photos/api", Account: "owner", Password: "test-password",
		JWTSecret: []byte(strings.Repeat("k", 32)), SessionTTL: time.Minute,
		CookieSecure: true, CookieSameSite: "none",
		CaptchaTTL: time.Minute, CaptchaLimit: 32, SessionLimit: 32,
		CaptchaRateLimit: 20, LoginRateLimit: 10,
		AllowedOrigins: []string{"https://ui.example.test"},
	}
	catalog, err := catalog.New(mediaRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catalog.Close() })
	captchas, _ := captcha.New(cfg.CaptchaTTL, cfg.CaptchaLimit)
	sessions, _ := session.New(cfg.JWTSecret, cfg.SessionTTL, cfg.SessionLimit)
	handler := New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, catalog)
	staticRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("spa"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler.SetWeb(web.New(config.Config{BasePath: cfg.BasePath, PublicAPIBase: cfg.PublicAPIBase, StaticRoot: staticRoot}))

	unauthorized := invoke(handler, http.MethodGet, "/photos/api/list?type=video", nil, nil, "127.0.0.1:1234")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d", unauthorized.Code)
	}
	streamPath := "/photos/api/stream/%E5%81%87%E6%9C%9F%20folder/clip%2001.mp4"
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		response := invoke(handler, method, streamPath, nil, http.Header{"Range": []string{"bytes=0-3"}}, "127.0.0.1:1234")
		if response.Code != http.StatusUnauthorized || response.Header().Get("Content-Range") != "" || strings.Contains(response.Body.String(), "0123456789") {
			t.Errorf("anonymous %s Range response = %d headers=%v body=%q", method, response.Code, response.Header(), response.Body.String())
		}
	}

	captchaResponse := invoke(handler, http.MethodGet, "/photos/api/captcha", nil, nil, "127.0.0.1:1234")
	var challenge captcha.Response
	if err := json.Unmarshal(captchaResponse.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	answer := answerFromDataURI(t, challenge.Data)
	payload, _ := json.Marshal(map[string]string{
		"account": "owner", "password": "test-password", "codeId": challenge.ID, "code": answer,
	})
	loginResponse := invoke(handler, http.MethodPost, "/photos/api/logIn", payload,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Content-Type": []string{"application/json"}}, "127.0.0.1:1234")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", loginResponse.Code, loginResponse.Body.String())
	}
	if loginResponse.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("credentialed CORS header missing")
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteNoneMode || cookies[0].Path != "/photos/api" {
		t.Fatalf("unexpected session cookie attributes: %#v", cookies)
	}
	if loginResponse.Body.String() != "{\"ok\":true}\n" {
		t.Fatalf("login response exposed unexpected data: %s", loginResponse.Body.String())
	}

	list := invoke(handler, http.MethodGet, "/photos/api/list?type=video", nil, nil, "127.0.0.1:1234", cookies...)
	if list.Code != http.StatusOK {
		t.Fatalf("media list status = %d body=%s", list.Code, list.Body.String())
	}
	var result struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &result); err != nil || len(result.Data) != 1 {
		t.Fatalf("media list = %#v, %v", result, err)
	}
	crossSite := invoke(handler, http.MethodGet, "/photos/api/session", nil,
		http.Header{"Sec-Fetch-Site": []string{"cross-site"}, "Referer": []string{"https://ui.example.test/view"}}, "127.0.0.1:1234", cookies...)
	if crossSite.Code != http.StatusOK {
		t.Fatalf("trusted cross-site session request status = %d body=%s", crossSite.Code, crossSite.Body.String())
	}
	untrustedCrossSite := invoke(handler, http.MethodGet, "/photos/api/session", nil,
		http.Header{"Sec-Fetch-Site": []string{"cross-site"}, "Referer": []string{"https://attacker.example.test/"}}, "127.0.0.1:1234", cookies...)
	if untrustedCrossSite.Code != http.StatusForbidden {
		t.Fatalf("untrusted cross-site session request status = %d", untrustedCrossSite.Code)
	}
	mediaURL, err := url.Parse(result.Data[0])
	if err != nil || mediaURL.EscapedPath() != "/photos/api/stream/%E5%81%87%E6%9C%9F%20folder/clip%2001.mp4" {
		t.Fatalf("media URL was not safely encoded: %q, %v", result.Data[0], err)
	}

	stream := invoke(handler, http.MethodGet, mediaURL.RequestURI(), nil,
		http.Header{"Range": []string{"bytes=3-6"}}, "127.0.0.1:1234", cookies...)
	if stream.Code != http.StatusPartialContent || stream.Body.String() != "3456" || stream.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("media range response = %d %q", stream.Code, stream.Body.String())
	}
	for _, attackPath := range []string{
		"/photos/api/stream/%2e%2e/outside.jpg",
		"/photos/api/stream/%252e%252e/outside.jpg",
		"/photos/api/stream/%2Fetc%2Fpasswd.jpg",
	} {
		response := invoke(handler, http.MethodGet, attackPath, nil, nil, "127.0.0.1:1234", cookies...)
		if response.Code != http.StatusNotFound {
			t.Errorf("unsafe media path %q returned %d", attackPath, response.Code)
		}
	}

	logoutWithoutOrigin := invoke(handler, http.MethodPost, "/photos/api/logout", nil, nil, "127.0.0.1:1234", cookies...)
	if logoutWithoutOrigin.Code != http.StatusForbidden {
		t.Fatalf("logout without Origin status = %d, want forbidden", logoutWithoutOrigin.Code)
	}
	logout := invoke(handler, http.MethodPost, "/photos/api/logout", nil,
		http.Header{"Origin": []string{"https://ui.example.test"}}, "127.0.0.1:1234", cookies...)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d", logout.Code)
	}
	if response := invoke(handler, http.MethodGet, "/photos/api/session", nil, nil, "127.0.0.1:1234", cookies...); response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d", response.Code)
	}
}

func TestListReturnsEmptyDataForEmptyCatalog(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	cookie := authenticatedCookie(t, handler)

	response := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
	if response.Code != http.StatusOK || response.Body.String() != "{\"data\":[]}\n" {
		t.Fatalf("empty media list = %d %q, want 200 {\"data\":[]}", response.Code, response.Body.String())
	}
}

func TestListReturnsServiceUnavailableWhenCatalogScanFails(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	cookie := authenticatedCookie(t, handler)
	if err := handler.catalog.Close(); err != nil {
		t.Fatal(err)
	}

	response := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("failed media scan status = %d body=%q, want 503", response.Code, response.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result["message"] == "" {
		t.Fatalf("failed media scan error body = %q, %v; want JSON message", response.Body.String(), err)
	}
}

func TestListDoesNotWriteTemporaryFailureForCanceledRequest(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	cookie := authenticatedCookie(t, handler)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/list?type=image", nil).WithContext(ctx)
	request.RemoteAddr = "127.0.0.1:1234"
	request.AddCookie(cookie)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)
	if response.Code == http.StatusServiceUnavailable || response.Body.Len() != 0 {
		t.Fatalf("canceled list response = %d body=%q, want no response body and no 503 classification", response.Code, response.Body.String())
	}
}

func TestIPAllowlistAppliesToPublicRoutes(t *testing.T) {
	media, err := catalog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	cfg := config.Config{
		PublicAPIBase: "/api", Account: "owner", Password: "pw", JWTSecret: []byte(strings.Repeat("k", 32)),
		SessionTTL: time.Minute, CaptchaTTL: time.Minute, CaptchaLimit: 2, SessionLimit: 2,
		CaptchaRateLimit: 2, LoginRateLimit: 2, AllowedIPs: []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")},
	}
	captchas, _ := captcha.New(cfg.CaptchaTTL, cfg.CaptchaLimit)
	sessions, _ := session.New(cfg.JWTSecret, cfg.SessionTTL, cfg.SessionLimit)
	handler := New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, media)
	response := invoke(handler, http.MethodGet, "/api/captcha", nil, nil, "192.0.2.55:1234")
	if response.Code != http.StatusForbidden {
		t.Fatalf("public captcha ignored IP allowlist: status %d", response.Code)
	}
	preflight := invoke(handler, http.MethodOptions, "/api/logIn", nil,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Access-Control-Request-Method": []string{"POST"}}, "192.0.2.55:1234")
	if preflight.Code != http.StatusForbidden {
		t.Fatalf("preflight ignored IP allowlist: status %d", preflight.Code)
	}
	loopbackCaptcha := invoke(handler, http.MethodGet, "/api/captcha", nil, nil, "127.0.0.1:1234")
	if loopbackCaptcha.Code != http.StatusForbidden {
		t.Fatalf("loopback bypassed API IP allowlist: status %d", loopbackCaptcha.Code)
	}
	healthBlocked := invoke(handler, http.MethodGet, "/healthz", nil, nil, "192.0.2.55:1234")
	if healthBlocked.Code != http.StatusForbidden {
		t.Fatalf("health endpoint ignored IP allowlist: status %d", healthBlocked.Code)
	}
	health := invoke(handler, http.MethodGet, "/healthz", nil, nil, "198.51.100.55:1234")
	if health.Code != http.StatusOK || health.Body.String() != "ok\n" || health.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("health response = %d %q content-type=%q", health.Code, health.Body.String(), health.Header().Get("Content-Type"))
	}
	loopbackHealth := invoke(handler, http.MethodGet, "/healthz", nil, nil, "127.0.0.1:1234")
	if loopbackHealth.Code != http.StatusOK || loopbackHealth.Body.String() != "ok\n" {
		t.Fatalf("loopback health response = %d %q", loopbackHealth.Code, loopbackHealth.Body.String())
	}
	for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodOptions} {
		response := invoke(handler, method, "/healthz", nil, nil, "127.0.0.1:1234")
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Errorf("health method %s response = %d Allow=%q, want method not allowed and GET", method, response.Code, response.Header().Get("Allow"))
		}
	}
}

func TestHealthzSupportsRootAndDeploymentPrefix(t *testing.T) {
	media, err := catalog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	cfg := config.Config{
		BasePath: "/photos", PublicAPIBase: "/photos/api", JWTSecret: []byte(strings.Repeat("k", 32)),
		AllowedIPs: []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")},
	}
	captchas, err := captcha.New(time.Minute, 4)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.New(cfg.JWTSecret, time.Minute, 4)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(cfg, auth.New("owner", "pw"), captchas, sessions, media)

	for _, target := range []string{"/healthz", "/photos/healthz"} {
		response := invoke(handler, http.MethodGet, target, nil, nil, "127.0.0.1:1234")
		if response.Code != http.StatusOK || response.Body.String() != "ok\n" {
			t.Errorf("loopback GET %s = %d %q, want 200 ok", target, response.Code, response.Body.String())
		}
		blocked := invoke(handler, http.MethodGet, target, nil, nil, "192.0.2.55:1234")
		if blocked.Code != http.StatusForbidden {
			t.Errorf("external GET %s = %d, want forbidden", target, blocked.Code)
		}
	}
	api := invoke(handler, http.MethodGet, "/photos/api/captcha", nil, nil, "127.0.0.1:1234")
	if api.Code != http.StatusForbidden {
		t.Fatalf("prefixed API inherited health loopback exception: %d", api.Code)
	}
}

func TestCORSPreflightIsScopedToRouteMethodAndHeaders(t *testing.T) {
	media, err := catalog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	cfg := config.Config{
		PublicAPIBase: "/api", JWTSecret: []byte(strings.Repeat("k", 32)),
		AllowedOrigins: []string{"https://ui.example.test"},
	}
	captchas, _ := captcha.New(time.Minute, 4)
	sessions, _ := session.New(cfg.JWTSecret, time.Minute, 4)
	handler := New(cfg, auth.New("owner", "pw"), captchas, sessions, media)

	valid := invoke(handler, http.MethodOptions, "/api/logIn", nil,
		http.Header{
			"Origin":                         []string{"https://ui.example.test"},
			"Access-Control-Request-Method":  []string{"POST"},
			"Access-Control-Request-Headers": []string{"content-type"},
		}, "127.0.0.1:1234")
	if valid.Code != http.StatusNoContent || valid.Header().Get("Access-Control-Allow-Origin") != "https://ui.example.test" || valid.Header().Get("Access-Control-Allow-Credentials") != "true" || valid.Header().Get("Access-Control-Allow-Methods") != http.MethodPost {
		t.Fatalf("valid login preflight = %d headers=%v", valid.Code, valid.Header())
	}

	badMethod := invoke(handler, http.MethodOptions, "/api/logIn", nil,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Access-Control-Request-Method": []string{"DELETE"}}, "127.0.0.1:1234")
	if badMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unsupported preflight method status = %d", badMethod.Code)
	}
	badHeader := invoke(handler, http.MethodOptions, "/api/logIn", nil,
		http.Header{
			"Origin":                         []string{"https://ui.example.test"},
			"Access-Control-Request-Method":  []string{"POST"},
			"Access-Control-Request-Headers": []string{"x-unapproved"},
		}, "127.0.0.1:1234")
	if badHeader.Code != http.StatusForbidden {
		t.Fatalf("unsupported preflight header status = %d", badHeader.Code)
	}
	badOrigin := invoke(handler, http.MethodOptions, "/api/logIn", nil,
		http.Header{"Origin": []string{"https://attacker.example.test"}, "Access-Control-Request-Method": []string{"POST"}}, "127.0.0.1:1234")
	if badOrigin.Code != http.StatusForbidden || badOrigin.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted preflight origin = %d headers=%v", badOrigin.Code, badOrigin.Header())
	}
	unknown := invoke(handler, http.MethodOptions, "/api/unknown", nil,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Access-Control-Request-Method": []string{"GET"}}, "127.0.0.1:1234")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown API preflight status = %d", unknown.Code)
	}
}

func TestLoginLimitsByIPAndExactAccountWithoutDifferentResponses(t *testing.T) {
	handler := newLoginTestHandler(t, 1)
	wrongPassword := func(ip, account string) *httptest.ResponseRecorder {
		return loginWithFreshCaptcha(t, handler, ip, account, "incorrect")
	}

	invalidCaptcha := loginWithInvalidCaptcha(t, handler, "192.0.2.99:1234", "owner", "incorrect")
	if invalidCaptcha.Code != http.StatusBadRequest {
		t.Fatalf("invalid CAPTCHA status = %d, want bad request", invalidCaptcha.Code)
	}
	ownerFailure := wrongPassword("192.0.2.1:1234", "owner")
	if ownerFailure.Code != http.StatusUnauthorized {
		t.Fatalf("first owner attempt = %d, want unauthorized", ownerFailure.Code)
	}
	ownerLimited := wrongPassword("192.0.2.2:1234", "owner")
	if ownerLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("owner account limit = %d, want too many requests", ownerLimited.Code)
	}
	ipLimited := wrongPassword("192.0.2.1:1234", "another-account")
	if ipLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("source-IP limit = %d, want too many requests", ipLimited.Code)
	}

	unknownFailure := wrongPassword("198.51.100.1:1234", "not-an-account")
	if unknownFailure.Code != http.StatusUnauthorized {
		t.Fatalf("first unknown-account attempt = %d, want unauthorized", unknownFailure.Code)
	}
	unknownLimited := wrongPassword("198.51.100.2:1234", "not-an-account")
	if unknownLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("unknown-account limit = %d, want too many requests", unknownLimited.Code)
	}
	if ownerLimited.Body.String() != unknownLimited.Body.String() || ownerLimited.Body.String() != ipLimited.Body.String() {
		t.Fatalf("throttle responses differ by dimension/account: owner=%q unknown=%q ip=%q", ownerLimited.Body.String(), unknownLimited.Body.String(), ipLimited.Body.String())
	}

	caseDistinct := wrongPassword("203.0.113.1:1234", "Owner")
	if caseDistinct.Code != http.StatusUnauthorized {
		t.Fatalf("case-distinct account was treated as the same identity: status %d", caseDistinct.Code)
	}
}

func TestLoginFailureCooldownIsAppliedAfterValidCaptcha(t *testing.T) {
	handler := newLoginTestHandler(t, 100)
	for attempt := 0; attempt < loginFailureThreshold; attempt++ {
		response := loginWithFreshCaptcha(t, handler, fmt.Sprintf("192.0.2.%d:1234", attempt+1), "owner", "incorrect")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("failed-password attempt %d = %d, want unauthorized", attempt+1, response.Code)
		}
	}
	response := loginWithFreshCaptcha(t, handler, "198.51.100.1:1234", "owner", "incorrect")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("login during failure cooldown = %d, want too many requests", response.Code)
	}
}

func TestLoginRejectsMissingCaptcha(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	payload, err := json.Marshal(loginRequest{Account: "owner", Password: "correct"})
	if err != nil {
		t.Fatal(err)
	}
	response := invoke(handler, http.MethodPost, "/api/logIn", payload,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Content-Type": []string{"application/json"}}, "192.0.2.10:1234")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("login without CAPTCHA = %d body=%s, want bad request", response.Code, response.Body.String())
	}
}

func TestSuccessfulLoginResetsFailureStreak(t *testing.T) {
	handler := newLoginTestHandler(t, 100)
	for attempt := 0; attempt < loginFailureThreshold-1; attempt++ {
		response := loginWithFreshCaptcha(t, handler, fmt.Sprintf("192.0.2.%d:1234", attempt+1), "owner", "incorrect")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("failed-password attempt %d = %d, want unauthorized", attempt+1, response.Code)
		}
	}

	success := loginWithFreshCaptcha(t, handler, "192.0.2.10:1234", "owner", "correct")
	if success.Code != http.StatusOK {
		t.Fatalf("successful login = %d body=%s, want OK", success.Code, success.Body.String())
	}

	for attempt := 0; attempt < loginFailureThreshold-1; attempt++ {
		response := loginWithFreshCaptcha(t, handler, fmt.Sprintf("198.51.100.%d:1234", attempt+1), "owner", "incorrect")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("failed-password attempt after success %d = %d, want unauthorized", attempt+1, response.Code)
		}
	}
}

func invoke(handler http.Handler, method, target string, body []byte, headers http.Header, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.RemoteAddr = remote
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func authenticatedCookie(t *testing.T, handler *Handler) *http.Cookie {
	t.Helper()
	token, _, err := handler.sessions.Create("owner")
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: session.CookieName, Value: token}
}

func answerFromDataURI(t *testing.T, data string) string {
	t.Helper()
	encoded := strings.TrimPrefix(data, "data:image/svg+xml;base64,")
	markup, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`>([0-9]+) ([+-]) ([0-9]+) = \?</text>`).FindStringSubmatch(string(markup))
	if len(match) != 4 {
		t.Fatal("captcha expression not found")
	}
	left, _ := strconv.Atoi(match[1])
	right, _ := strconv.Atoi(match[3])
	if match[2] == "+" {
		return strconv.Itoa(left + right)
	}
	return fmt.Sprint(left - right)
}

func newLoginTestHandler(t *testing.T, loginRateLimit int) *Handler {
	t.Helper()
	media, err := catalog.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = media.Close() })
	cfg := config.Config{
		PublicAPIBase: "/api", Account: "owner", Password: "correct", JWTSecret: []byte(strings.Repeat("k", 32)),
		SessionTTL: time.Minute, CaptchaTTL: time.Minute, CaptchaLimit: 64, SessionLimit: 4,
		CaptchaRateLimit: 20, LoginRateLimit: loginRateLimit, AllowedOrigins: []string{"https://ui.example.test"},
	}
	captchas, err := captcha.New(cfg.CaptchaTTL, cfg.CaptchaLimit)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := session.New(cfg.JWTSecret, cfg.SessionTTL, cfg.SessionLimit)
	if err != nil {
		t.Fatal(err)
	}
	return New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, media)
}

func loginWithFreshCaptcha(t *testing.T, handler *Handler, remote, account, password string) *httptest.ResponseRecorder {
	t.Helper()
	captchaResponse := invoke(handler, http.MethodGet, "/api/captcha", nil, nil, remote)
	if captchaResponse.Code != http.StatusOK {
		t.Fatalf("captcha status = %d body=%s", captchaResponse.Code, captchaResponse.Body.String())
	}
	var challenge captcha.Response
	if err := json.Unmarshal(captchaResponse.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(loginRequest{
		Account: account, Password: password, CodeID: challenge.ID, Code: answerFromDataURI(t, challenge.Data),
	})
	if err != nil {
		t.Fatal(err)
	}
	return invoke(handler, http.MethodPost, "/api/logIn", payload,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Content-Type": []string{"application/json"}}, remote)
}

func loginWithInvalidCaptcha(t *testing.T, handler *Handler, remote, account, password string) *httptest.ResponseRecorder {
	t.Helper()
	captchaResponse := invoke(handler, http.MethodGet, "/api/captcha", nil, nil, remote)
	if captchaResponse.Code != http.StatusOK {
		t.Fatalf("captcha status = %d body=%s", captchaResponse.Code, captchaResponse.Body.String())
	}
	var challenge captcha.Response
	if err := json.Unmarshal(captchaResponse.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(loginRequest{
		Account: account, Password: password, CodeID: challenge.ID, Code: answerFromDataURI(t, challenge.Data) + "invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	return invoke(handler, http.MethodPost, "/api/logIn", payload,
		http.Header{"Origin": []string{"https://ui.example.test"}, "Content-Type": []string{"application/json"}}, remote)
}
