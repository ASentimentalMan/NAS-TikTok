package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/config"
)

func TestRuntimeConfigAndSPAUseDeploymentPrefix(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("app-shell"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := New(config.Config{BasePath: "/photos", PublicAPIBase: "/photos/api", StaticRoot: root})

	env := httptest.NewRecorder()
	handler.ServeHTTP(env, httptest.NewRequest("GET", "/photos/env.js", nil))
	if env.Code != 200 || !strings.Contains(env.Body.String(), `API_BASE: "/photos/api"`) || env.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("runtime config response: %d %q", env.Code, env.Body.String())
	}

	page := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/photos/library/album", nil)
	request.Header.Set("Accept", "text/html")
	handler.ServeHTTP(page, request)
	if page.Code != 200 || page.Body.String() != "app-shell" {
		t.Fatalf("SPA response: %d %q location=%q", page.Code, page.Body.String(), page.Header().Get("Location"))
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest("GET", "/photos/missing.js", nil))
	if asset.Code != 404 {
		t.Fatalf("missing asset returned %d", asset.Code)
	}
}
