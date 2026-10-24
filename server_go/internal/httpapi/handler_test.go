package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	mediaindex "github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/index"
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
	handler := New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, catalog, nil)
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

func TestListUsesIndexWhenReadyAndCatalogWhileIndexIsUnavailable(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	cookie := authenticatedCookie(t, handler)
	diskPath := filepath.Join(handler.catalog.RootPath(), "disk image.jpg")
	if err := os.WriteFile(diskPath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	index := &fakeMediaIndex{paths: []string{"indexed image.jpg"}, ready: true}
	handler.index = index

	indexed := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
	if indexed.Code != http.StatusOK {
		t.Fatalf("indexed list status = %d body=%q", indexed.Code, indexed.Body.String())
	}
	var indexedResult struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(indexed.Body.Bytes(), &indexedResult); err != nil {
		t.Fatal(err)
	}
	if len(indexedResult.Data) != 1 || !strings.Contains(indexedResult.Data[0], "indexed%20image.jpg") {
		t.Fatalf("ready index list = %#v, want indexed media path", indexedResult.Data)
	}

	index.ready = false
	index.paths = []string{"stale index image.jpg"}
	disk := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
	if disk.Code != http.StatusOK {
		t.Fatalf("disk fallback list status = %d body=%q", disk.Code, disk.Body.String())
	}
	var diskResult struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(disk.Body.Bytes(), &diskResult); err != nil {
		t.Fatal(err)
	}
	if len(diskResult.Data) != 1 || !strings.Contains(diskResult.Data[0], "disk%20image.jpg") {
		t.Fatalf("unready index list = %#v, want disk scan result", diskResult.Data)
	}

	index.ready = true
	index.sampleErr = errors.New("private database detail")
	failed := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
	if failed.Code != http.StatusServiceUnavailable || strings.Contains(failed.Body.String(), "private database detail") {
		t.Fatalf("database sample failure = %d body=%q, want generic 503", failed.Code, failed.Body.String())
	}
}

func TestReinitializeUsesSessionAndBasePathAndReturnsAccepted(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.BasePath = "/photos"
	handler.config.PublicAPIBase = "/photos/api"
	index := &fakeMediaIndex{reinitializeStarted: true}
	handler.index = index

	unauthorized := invoke(handler, http.MethodGet, "/photos/api/reinit", nil, nil, "127.0.0.1:1234")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated reinitialize status = %d body=%q", unauthorized.Code, unauthorized.Body.String())
	}
	if outsideBasePath := invoke(handler, http.MethodGet, "/api/reinit", nil, nil, "127.0.0.1:1234", authenticatedCookie(t, handler)); outsideBasePath.Code != http.StatusNotFound {
		t.Fatalf("reinitialize outside base path status = %d, want 404", outsideBasePath.Code)
	}
	preflight := invoke(handler, http.MethodOptions, "/photos/api/reinit", nil,
		http.Header{"Access-Control-Request-Method": []string{http.MethodGet}}, "127.0.0.1:1234")
	if preflight.Code != http.StatusNoContent || !strings.Contains(preflight.Header().Get("Access-Control-Allow-Methods"), http.MethodGet) {
		t.Fatalf("reinitialize preflight = %d methods=%q", preflight.Code, preflight.Header().Get("Access-Control-Allow-Methods"))
	}

	cookie := authenticatedCookie(t, handler)
	accepted := invoke(handler, http.MethodGet, "/photos/api/reinit", nil, nil, "127.0.0.1:1234", cookie)
	if accepted.Code != http.StatusAccepted || accepted.Body.String() != "{\"ok\":true,\"started\":true}\n" || index.reinitializeCalls != 1 {
		t.Fatalf("authenticated reinitialize = %d body=%q calls=%d", accepted.Code, accepted.Body.String(), index.reinitializeCalls)
	}
}

func TestReinitializeReturnsConflictWhenAlreadyRunning(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	index := &fakeMediaIndex{reinitializeStarted: false, reinitializeErr: mediaindex.ErrScanInProgress}
	handler.index = index

	response := invoke(handler, http.MethodGet, "/api/reinit", nil, nil, "127.0.0.1:1234", authenticatedCookie(t, handler))
	if response.Code != http.StatusConflict || response.Body.String() == "" {
		t.Fatalf("already-running reinitialize = %d body=%q, want conflict JSON", response.Code, response.Body.String())
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
	handler := New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, media, nil)
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
	staticRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("index"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler.SetWeb(web.New(config.Config{PublicAPIBase: "/api", StaticRoot: staticRoot}))
	blockedStatic := invoke(handler, http.MethodGet, "/", nil, nil, "192.0.2.55:1234")
	if blockedStatic.Code != http.StatusForbidden {
		t.Fatalf("static page ignored IP allowlist: status %d", blockedStatic.Code)
	}
	for _, method := range []string{http.MethodHead, http.MethodPost, http.MethodOptions} {
		response := invoke(handler, method, "/healthz", nil, nil, "127.0.0.1:1234")
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
			t.Errorf("health method %s response = %d Allow=%q, want method not allowed and GET", method, response.Code, response.Header().Get("Allow"))
		}
	}
}

func TestStaticResourcesDoNotRejectDisallowedOrigins(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	staticRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(staticRoot, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":     "<html>home</html>",
		"assets/app.js":  "window.app = true;",
		"assets/app.css": "body { color: black; }",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(staticRoot, filepath.FromSlash(name)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	handler.SetWeb(web.New(config.Config{PublicAPIBase: "/api", StaticRoot: staticRoot}))

	resources := []string{"/", "/assets/app.js", "/assets/app.css", "/env.js"}
	scenarios := []struct {
		name        string
		origin      string
		host        string
		remote      string
		forwarded   string
		allowOrigin string
	}{
		{name: "no origin", host: "nas.example.test", remote: "192.0.2.10:1234"},
		{name: "same-origin HTTP", origin: "http://nas.example.test", host: "nas.example.test", remote: "192.0.2.10:1234", allowOrigin: "http://nas.example.test"},
		{name: "HTTPS origin via untrusted HTTP upstream", origin: "https://nas.example.test", host: "nas.example.test", remote: "192.0.2.10:1234", forwarded: "https"},
		{name: "disallowed cross-origin", origin: "https://attacker.example.test", host: "nas.example.test", remote: "192.0.2.10:1234"},
	}
	for _, resource := range resources {
		for _, scenario := range scenarios {
			t.Run(resource+"/"+scenario.name, func(t *testing.T) {
				headers := make(http.Header)
				if scenario.origin != "" {
					headers.Set("Origin", scenario.origin)
				}
				if scenario.forwarded != "" {
					headers.Set("X-Forwarded-Proto", scenario.forwarded)
				}
				response := invokeWithHost(handler, http.MethodGet, resource, nil, headers, scenario.host, scenario.remote)
				if response.Code != http.StatusOK {
					t.Fatalf("GET %s status = %d, content-type=%q body=%q", resource, response.Code, response.Header().Get("Content-Type"), response.Body.String())
				}
				if strings.Contains(response.Header().Get("Content-Type"), "application/json") {
					t.Fatalf("static response unexpectedly returned JSON: %q", response.Body.String())
				}
				if got := response.Header().Get("Access-Control-Allow-Origin"); got != scenario.allowOrigin {
					t.Fatalf("Access-Control-Allow-Origin = %q, want %q", got, scenario.allowOrigin)
				}
				if strings.TrimSpace(response.Header().Get("Vary")) == "" || !strings.Contains(response.Header().Get("Vary"), "Origin") {
					t.Fatalf("Vary = %q, want Origin", response.Header().Get("Vary"))
				}
			})
		}
	}
}

func TestAPIRetainsStrictOriginChecksWithUntrustedForwardedHeaders(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	invalid := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil,
		http.Header{"Origin": []string{"https://attacker.example.test"}}, "nas.example.test", "192.0.2.10:1234")
	if invalid.Code != http.StatusForbidden || invalid.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("invalid API Origin response = %d headers=%v body=%q", invalid.Code, invalid.Header(), invalid.Body.String())
	}

	forged := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, http.Header{
		"Origin":            []string{"https://nas.example.test"},
		"X-Forwarded-Proto": []string{"https"},
		"X-Forwarded-Host":  []string{"nas.example.test"},
		"X-Forwarded-For":   []string{"127.0.0.1"},
	}, "nas.example.test", "192.0.2.10:1234")
	if forged.Code != http.StatusForbidden || forged.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("untrusted forwarded headers bypassed API Origin check: %d headers=%v body=%q", forged.Code, forged.Header(), forged.Body.String())
	}
}

func TestTrustedHTTPSProxySupportsLoginSessionAndLogout(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.AllowedOrigins = nil
	handler.config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	proxyHeaders := http.Header{
		"Origin":            []string{"https://nas.example.test:63000"},
		"X-Forwarded-Proto": []string{"https"},
	}
	const host = "nas.example.test:63000"
	const remote = "127.0.0.1:1234"

	captchaResponse := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, proxyHeaders, host, remote)
	if captchaResponse.Code != http.StatusOK || captchaResponse.Header().Get("Access-Control-Allow-Origin") != "https://nas.example.test:63000" {
		t.Fatalf("trusted-proxy captcha = %d headers=%v body=%q", captchaResponse.Code, captchaResponse.Header(), captchaResponse.Body.String())
	}
	var challenge captcha.Response
	if err := json.Unmarshal(captchaResponse.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(loginRequest{
		Account: "owner", Password: "correct", CodeID: challenge.ID, Code: answerFromDataURI(t, challenge.Data),
	})
	if err != nil {
		t.Fatal(err)
	}
	loginHeaders := proxyHeaders.Clone()
	loginHeaders.Set("Content-Type", "application/json")
	loginResponse := invokeWithHost(handler, http.MethodPost, "/api/logIn", payload, loginHeaders, host, remote)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("trusted-proxy login = %d body=%q", loginResponse.Code, loginResponse.Body.String())
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != session.CookieName || !cookies[0].HttpOnly || cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Path != "/api" {
		t.Fatalf("trusted-proxy session cookie attributes = %#v", cookies)
	}

	sessionResponse := invokeWithHost(handler, http.MethodGet, "/api/session", nil, proxyHeaders, host, remote, cookies...)
	if sessionResponse.Code != http.StatusOK {
		t.Fatalf("trusted-proxy session = %d body=%q", sessionResponse.Code, sessionResponse.Body.String())
	}
	logoutResponse := invokeWithHost(handler, http.MethodPost, "/api/logout", nil, proxyHeaders, host, remote, cookies...)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("trusted-proxy logout = %d body=%q", logoutResponse.Code, logoutResponse.Body.String())
	}
	revoked := invokeWithHost(handler, http.MethodGet, "/api/session", nil, nil, host, remote, cookies...)
	if revoked.Code != http.StatusUnauthorized {
		t.Fatalf("session after trusted-proxy logout = %d, want unauthorized", revoked.Code)
	}
}

func TestDirectHTTPOriginWithNonStandardPortWithoutCORSOrigins(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.AllowedOrigins = nil
	response := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil,
		http.Header{"Origin": []string{"http://nas.example.test:63000"}},
		"nas.example.test:63000", "192.0.2.10:1234")
	if response.Code != http.StatusOK {
		t.Fatalf("direct HTTP same-origin captcha = %d body=%q", response.Code, response.Body.String())
	}
}

func TestDebugOriginLogRetainsAutomaticFailureWhenCORSAllows(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "debug"
	handler.config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	handler.config.AllowedOrigins = []string{"https://nas.example.test:63000"}
	logs := captureRequestLogs(handler)
	headers := http.Header{
		"Origin":            []string{"https://nas.example.test:63000"},
		"X-Forwarded-Proto": []string{"http"},
	}
	response := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, headers,
		"nas.example.test:63000", "127.0.0.1:1234")
	if response.Code != http.StatusOK {
		t.Fatalf("configured Origin fallback status = %d body=%q", response.Code, response.Body.String())
	}
	for _, want := range []string{
		"same_origin=false", "same_origin_reason=origin_scheme_mismatch",
		"allowed_via=configured_origin", "origin_allowed=true", `origin_port="63000"`,
		`request_port="63000"`, "request_scheme=http",
		"proxy_trusted=true", "x_forwarded_proto_present=true", "x_forwarded_proto_used=true",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("debug Origin log lacks %q: %s", want, logs.String())
		}
	}

	logs.Reset()
	handler.config.AllowedOrigins = nil
	response = invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, headers,
		"nas.example.test:63000", "127.0.0.1:1234")
	if response.Code != http.StatusForbidden {
		t.Fatalf("unconfigured Origin fallback status = %d, want 403", response.Code)
	}
	for _, want := range []string{"same_origin=false", "same_origin_reason=origin_scheme_mismatch", "allowed_via=none", "origin_allowed=false"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("rejected Origin log lacks %q: %s", want, logs.String())
		}
	}
}

func TestDebugOriginLogDistinguishesHostPortLossAndUntrustedForwardedProto(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "debug"
	handler.config.AllowedOrigins = nil
	handler.config.TrustedProxies = []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	logs := captureRequestLogs(handler)

	response := invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, http.Header{
		"Origin":            []string{"https://nas.example.test:63000"},
		"X-Forwarded-Proto": []string{"https"},
	}, "nas.example.test", "127.0.0.1:1234")
	if response.Code != http.StatusForbidden || !strings.Contains(logs.String(), "same_origin_reason=origin_host_mismatch") ||
		!strings.Contains(logs.String(), `origin_port="63000"`) || !strings.Contains(logs.String(), `request_port=""`) {
		t.Fatalf("Host port loss response/log = %d %s", response.Code, logs.String())
	}

	logs.Reset()
	response = invokeWithHost(handler, http.MethodGet, "/api/captcha", nil, http.Header{
		"Origin":            []string{"https://nas.example.test:63000"},
		"X-Forwarded-Proto": []string{"https"},
		"X-Forwarded-Host":  []string{"nas.example.test:63000"},
	}, "nas.example.test:63000", "192.0.2.10:1234")
	if response.Code != http.StatusForbidden {
		t.Fatalf("untrusted forwarded proto status = %d, want 403", response.Code)
	}
	for _, want := range []string{
		"same_origin_reason=origin_scheme_mismatch", "proxy_trusted=false",
		"x_forwarded_proto_present=true", "x_forwarded_proto_used=false",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("untrusted proxy log lacks %q: %s", want, logs.String())
		}
	}
}

func TestInfoLogsOnlyLoginAttemptsAndLogsEachOnce(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "info"
	logs := captureRequestLogs(handler)

	staticRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(staticRoot, "index.html"), []byte("index"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler.SetWeb(web.New(config.Config{StaticRoot: staticRoot, PublicAPIBase: "/api"}))
	if response := invoke(handler, http.MethodGet, "/", nil, nil, "127.0.0.1:1234"); response.Code != http.StatusOK {
		t.Fatalf("static page status = %d", response.Code)
	}
	if response := invoke(handler, http.MethodGet, "/api/captcha", nil, nil, "127.0.0.1:1234"); response.Code != http.StatusOK {
		t.Fatalf("captcha status = %d", response.Code)
	}
	videoPath := filepath.Join(handler.catalog.RootPath(), "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	rangeResponse := invoke(handler, http.MethodGet, "/api/stream/clip.mp4", nil,
		http.Header{"Range": []string{"bytes=0-2"}}, "127.0.0.1:1234", authenticatedCookie(t, handler))
	if rangeResponse.Code != http.StatusPartialContent || logs.Len() != 0 {
		t.Fatalf("Range request = %d, info logs=%q; want 206 and no log", rangeResponse.Code, logs.String())
	}

	success := loginWithFreshCaptcha(t, handler, "127.0.0.1:1234", "owner", "correct")
	if success.Code != http.StatusOK || strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), "event=login_success") {
		t.Fatalf("success login/log = %d %q; want one login_success line", success.Code, logs.String())
	}

	logs.Reset()
	failure := loginWithFreshCaptcha(t, handler, "127.0.0.1:1234", "owner", "password-sentinel")
	if failure.Code != http.StatusUnauthorized || strings.Count(logs.String(), "\n") != 1 ||
		!strings.Contains(logs.String(), "event=login_failed") || !strings.Contains(logs.String(), "reason=credentials_invalid") ||
		strings.Contains(logs.String(), "password-sentinel") {
		t.Fatalf("failed login/log = %d %q", failure.Code, logs.String())
	}

	logs.Reset()
	badCaptcha := loginWithInvalidCaptcha(t, handler, "127.0.0.1:1234", "owner", "correct")
	if badCaptcha.Code != http.StatusBadRequest || strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), "reason=captcha_invalid") {
		t.Fatalf("invalid captcha/log = %d %q", badCaptcha.Code, logs.String())
	}
}

func TestInfoLoginRecognitionUsesConfiguredAPIPath(t *testing.T) {
	for _, test := range []struct {
		name        string
		basePath    string
		requestPath string
		wantStatus  int
		wantLogin   bool
	}{
		{name: "root login", requestPath: "/api/logIn", wantStatus: http.StatusForbidden, wantLogin: true},
		{name: "prefixed login", basePath: "/photos", requestPath: "/photos/api/logIn", wantStatus: http.StatusForbidden, wantLogin: true},
		{name: "missing prefix is not login", basePath: "/photos", requestPath: "/api/logIn", wantStatus: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newLoginTestHandler(t, 10)
			handler.config.LogLevel = "info"
			handler.config.BasePath = test.basePath
			handler.config.AllowedOrigins = nil
			logs := captureRequestLogs(handler)

			response := invokeWithHost(handler, http.MethodPost, test.requestPath, nil,
				http.Header{"Origin": []string{"https://attacker.example.test"}},
				"nas.example.test", "192.0.2.10:1234")
			if response.Code != test.wantStatus {
				t.Fatalf("POST %s status = %d, want %d", test.requestPath, response.Code, test.wantStatus)
			}
			if test.wantLogin {
				if strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), "event=login_failed") {
					t.Fatalf("POST %s logs = %q, want exactly one login_failed event", test.requestPath, logs.String())
				}
			} else if logs.Len() != 0 {
				t.Fatalf("POST %s was not a configured login route but logged %q", test.requestPath, logs.String())
			}
		})
	}
}

func TestInfoLogsEarlyLoginRejectionsAndRateLimitOnce(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "info"
	handler.config.AllowedOrigins = nil
	logs := captureRequestLogs(handler)
	response := invokeWithHost(handler, http.MethodPost, "/api/logIn", []byte("password-sentinel"), http.Header{
		"Origin": []string{"https://attacker.example.test"},
	}, "nas.example.test", "192.0.2.10:1234")
	if response.Code != http.StatusForbidden || strings.Count(logs.String(), "\n") != 1 ||
		!strings.Contains(logs.String(), "reason=origin_host_mismatch") || strings.Contains(logs.String(), "password-sentinel") {
		t.Fatalf("Origin-rejected login/log = %d %q", response.Code, logs.String())
	}

	logs.Reset()
	handler.config.AllowedIPs = []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")}
	response = invokeWithHost(handler, http.MethodPost, "/api/logIn", nil, nil,
		"nas.example.test", "192.0.2.10:1234")
	if response.Code != http.StatusForbidden || strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), "reason=ip_not_allowed") {
		t.Fatalf("IP-rejected login/log = %d %q", response.Code, logs.String())
	}

	rateLimited := newLoginTestHandler(t, 1)
	rateLimited.config.LogLevel = "info"
	rateLogs := captureRequestLogs(rateLimited)
	first := loginWithFreshCaptcha(t, rateLimited, "127.0.0.1:1234", "owner", "wrong-password")
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first login status = %d", first.Code)
	}
	rateLogs.Reset()
	second := loginWithFreshCaptcha(t, rateLimited, "127.0.0.1:1234", "owner", "correct")
	if second.Code != http.StatusTooManyRequests || strings.Count(rateLogs.String(), "\n") != 1 || !strings.Contains(rateLogs.String(), "reason=login_rate_limited") {
		t.Fatalf("rate-limited login/log = %d %q", second.Code, rateLogs.String())
	}
}

func TestDebugLogsUseFixedRoutesAndDoNotLeakRequestData(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "debug"
	handler.config.BasePath = "/photos"
	logs := captureRequestLogs(handler)
	requestBody := []byte("password-sentinel captcha-sentinel")
	response := invokeWithHost(handler, http.MethodPost,
		"/photos/api/stream/private-media-sentinel.mp4?token=query-sentinel", requestBody,
		http.Header{
			"Origin":        []string{"https://attacker.example.test\r\nforged-field"},
			"Cookie":        []string{"session=cookie-sentinel"},
			"Authorization": []string{"Bearer jwt-sentinel"},
		}, "nas.example.test", "192.0.2.10:1234")
	if response.Code != http.StatusForbidden {
		t.Fatalf("malformed Origin status = %d", response.Code)
	}
	output := logs.String()
	for _, forbidden := range []string{
		"private-media-sentinel", "query-sentinel", "password-sentinel", "captcha-sentinel",
		"cookie-sentinel", "jwt-sentinel", "forged-field",
	} {
		if strings.Contains(output, forbidden) {
			t.Errorf("log contains sensitive marker %q: %q", forbidden, output)
		}
	}
	if !strings.Contains(output, `route="/api/stream/*"`) || strings.Count(output, "\n") != 1 || strings.Contains(output, "\r") {
		t.Fatalf("unexpected sanitized route/log line: %q", output)
	}

	for _, test := range []struct{ path, want string }{
		{"/photos/api/logIn", "/api/logIn"},
		{"/photos/assets/app.js", "/assets/*"},
		{"/photos/env.js", "/env.js"},
		{"/photos/healthz", "/healthz"},
		{"/photos/private/unknown", "other"},
	} {
		if got := requestRoute(test.path, "/photos"); got != test.want {
			t.Errorf("requestRoute(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestDebugClassifiesRootAndPartialMediaScanFailures(t *testing.T) {
	t.Run("root scan", func(t *testing.T) {
		handler := newLoginTestHandler(t, 10)
		handler.config.LogLevel = "debug"
		logs := captureRequestLogs(handler)
		cookie := authenticatedCookie(t, handler)
		if err := handler.catalog.Close(); err != nil {
			t.Fatal(err)
		}
		response := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(logs.String(), "reason=media_root_unreadable") {
			t.Fatalf("root scan status/log = %d %q", response.Code, logs.String())
		}
	})

	t.Run("partial child scan", func(t *testing.T) {
		handler := newLoginTestHandler(t, 10)
		handler.config.LogLevel = "debug"
		logs := captureRequestLogs(handler)
		cookie := authenticatedCookie(t, handler)
		root := handler.catalog.RootPath()
		child := filepath.Join(root, "unreadable")
		if err := os.Mkdir(child, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(child, "private-name.jpg"), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(child, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(child, 0o700) })
		if _, err := os.ReadDir(child); err == nil {
			t.Skip("process can still read mode-000 directories")
		}
		response := invoke(handler, http.MethodGet, "/api/list?type=image", nil, nil, "127.0.0.1:1234", cookie)
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(logs.String(), "reason=media_scan_partial") ||
			strings.Contains(logs.String(), "private-name.jpg") || strings.Contains(logs.String(), root) {
			t.Fatalf("partial scan status/log = %d %q", response.Code, logs.String())
		}
	})
}

func TestMediaScanReasonClassifiesPermissionAndUnknownErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "root permission", err: errors.Join(catalog.ErrRootScan, os.ErrPermission), want: "media_root_permission_denied"},
		{name: "root unreadable", err: catalog.ErrRootScan, want: "media_root_unreadable"},
		{name: "partial scan", err: catalog.ErrPartialScan, want: "media_scan_partial"},
		{name: "permission", err: os.ErrPermission, want: "media_permission_denied"},
		{name: "unknown", err: errors.New("opaque failure"), want: "media_scan_failed"},
		{name: "deadline", err: context.DeadlineExceeded, want: "media_scan_timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := mediaScanReason(test.err); got != test.want {
				t.Fatalf("mediaScanReason() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLogLevelOffSuppressesRequestsAndLogin(t *testing.T) {
	handler := newLoginTestHandler(t, 10)
	handler.config.LogLevel = "off"
	logs := captureRequestLogs(handler)
	_ = invoke(handler, http.MethodGet, "/api/captcha", nil, nil, "127.0.0.1:1234")
	_ = invokeWithHost(handler, http.MethodPost, "/api/logIn", []byte("body-sentinel"), http.Header{
		"Origin": []string{"https://attacker.example.test"},
	}, "nas.example.test", "192.0.2.10:1234")
	if logs.Len() != 0 {
		t.Fatalf("LOG_LEVEL=off emitted logs: %q", logs.String())
	}
}

func captureRequestLogs(handler *Handler) *bytes.Buffer {
	var output bytes.Buffer
	handler.logger = log.New(&output, "", log.LstdFlags)
	return &output
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
	handler := New(cfg, auth.New("owner", "pw"), captchas, sessions, media, nil)

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
	handler := New(cfg, auth.New("owner", "pw"), captchas, sessions, media, nil)

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

func invokeWithHost(handler http.Handler, method, target string, body []byte, headers http.Header, host, remote string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = host
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
		SessionTTL: time.Minute, CaptchaTTL: time.Minute, CaptchaLimit: 64, SessionLimit: 4, LogLevel: "off",
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
	return New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, media, nil)
}

type fakeMediaIndex struct {
	paths               []string
	ready               bool
	sampleErr           error
	reinitializeStarted bool
	reinitializeErr     error
	reinitializeCalls   int
}

func (index *fakeMediaIndex) Sample(ctx context.Context, _ string) ([]string, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return append([]string(nil), index.paths...), index.ready, index.sampleErr
}

func (index *fakeMediaIndex) Reinitialize() (bool, error) {
	index.reinitializeCalls++
	return index.reinitializeStarted, index.reinitializeErr
}

func (*fakeMediaIndex) RemoveMissing(string) error {
	return nil
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
