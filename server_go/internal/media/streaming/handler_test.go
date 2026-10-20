package streaming

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
)

func TestServeContentSupportsRangeHeadAndPrivateCaching(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := catalog.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	handler := New(media)

	request := httptest.NewRequest("GET", "/api/stream/video.mp4", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request, "video.mp4")
	if response.Code != 206 || response.Body.String() != "2345" || response.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range response = status %d, body %q, range %q", response.Code, response.Body.String(), response.Header().Get("Content-Range"))
	}
	if response.Header().Get("Content-Type") != "video/mp4" || response.Header().Get("Accept-Ranges") != "bytes" || response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("missing media response headers: %v", response.Header())
	}

	head := httptest.NewRequest("HEAD", "/api/stream/video.mp4", nil)
	headResponse := httptest.NewRecorder()
	handler.ServeHTTP(headResponse, head, "video.mp4")
	if headResponse.Code != 200 || headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD response = status %d, body size %d", headResponse.Code, headResponse.Body.Len())
	}
	headRange := httptest.NewRequest("HEAD", "/api/stream/video.mp4", nil)
	headRange.Header.Set("Range", "bytes=2-5")
	headRangeResponse := httptest.NewRecorder()
	handler.ServeHTTP(headRangeResponse, headRange, "video.mp4")
	if headRangeResponse.Code != 206 || headRangeResponse.Body.Len() != 0 || headRangeResponse.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("HEAD Range response = status %d body size %d Content-Range=%q", headRangeResponse.Code, headRangeResponse.Body.Len(), headRangeResponse.Header().Get("Content-Range"))
	}

	invalid := httptest.NewRequest("GET", "/api/stream/video.mp4", nil)
	invalid.Header.Set("Range", "bytes=20-")
	invalidResponse := httptest.NewRecorder()
	handler.ServeHTTP(invalidResponse, invalid, "video.mp4")
	if invalidResponse.Code != 416 {
		t.Fatalf("unsatisfiable range status = %d, want 416", invalidResponse.Code)
	}
}

func TestServeContentHandlesRangeVariants(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "empty.mp4"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := catalog.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	handler := New(media)

	cases := []struct {
		name         string
		file         string
		rangeHeader  string
		status       int
		body         string
		checkBody    bool
		contentRange string
	}{
		{name: "no range", file: "video.mp4", status: 200, body: "0123456789", checkBody: true},
		{name: "open ended", file: "video.mp4", rangeHeader: "bytes=0-", status: 206, body: "0123456789", checkBody: true, contentRange: "bytes 0-9/10"},
		{name: "bounded", file: "video.mp4", rangeHeader: "bytes=0-4", status: 206, body: "01234", checkBody: true, contentRange: "bytes 0-4/10"},
		{name: "suffix", file: "video.mp4", rangeHeader: "bytes=-4", status: 206, body: "6789", checkBody: true, contentRange: "bytes 6-9/10"},
		{name: "suffix larger than file", file: "video.mp4", rangeHeader: "bytes=-1024", status: 206, body: "0123456789", checkBody: true, contentRange: "bytes 0-9/10"},
		{name: "empty file with range", file: "empty.mp4", rangeHeader: "bytes=0-", status: 200, checkBody: true},
		{name: "reversed range", file: "video.mp4", rangeHeader: "bytes=5-3", status: 416},
		{name: "nonnumeric range", file: "video.mp4", rangeHeader: "bytes=NaN-", status: 416},
		{name: "unsupported unit", file: "video.mp4", rangeHeader: "items=0-2", status: 416},
		{name: "malformed delimiters", file: "video.mp4", rangeHeader: "bytes=--1", status: 416},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/stream/"+test.file, nil)
			if test.rangeHeader != "" {
				request.Header.Set("Range", test.rangeHeader)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request, test.file)
			if response.Code != test.status || (test.checkBody && response.Body.String() != test.body) {
				t.Fatalf("range response = status %d body %q, want %d body %q", response.Code, response.Body.String(), test.status, test.body)
			}
			if got := response.Header().Get("Content-Range"); got != test.contentRange {
				t.Fatalf("Content-Range = %q, want %q", got, test.contentRange)
			}
		})
	}
}

func TestServeContentIgnoresMultipleRanges(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "video.mp4"), []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := catalog.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()

	request := httptest.NewRequest("GET", "/api/stream/video.mp4", nil)
	request.Header.Set("Range", "bytes=0-1,4-5")
	response := httptest.NewRecorder()
	New(media).ServeHTTP(response, request, "video.mp4")
	if response.Code != 200 || response.Body.String() != "0123456789" || response.Header().Get("Content-Range") != "" {
		t.Fatalf("unsupported multiple range response = %d body=%q Content-Range=%q", response.Code, response.Body.String(), response.Header().Get("Content-Range"))
	}
}
