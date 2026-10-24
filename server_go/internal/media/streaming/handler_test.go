package streaming

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
	mediaindex "github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/index"
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
	handler := New(media, nil)

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
	handler := New(media, nil)

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

func TestServeContentMultipleRangesFollowGoCompatibilitySetting(t *testing.T) {
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
	New(media, nil).ServeHTTP(response, request, "video.mp4")
	switch response.Code {
	case 200:
		if response.Body.String() != "0123456789" || response.Header().Get("Content-Range") != "" {
			t.Fatalf("multiple range full response = body %q Content-Range=%q, want complete content without Content-Range", response.Body.String(), response.Header().Get("Content-Range"))
		}
		return
	case 206:
		if response.Header().Get("Content-Range") != "" {
			t.Fatalf("multiple range response has top-level Content-Range=%q", response.Header().Get("Content-Range"))
		}
	default:
		t.Fatalf("multiple range response status = %d, want 200 or 206", response.Code)
	}
	mediaType, params, err := mime.ParseMediaType(response.Header().Get("Content-Type"))
	if err != nil || mediaType != "multipart/byteranges" || params["boundary"] == "" {
		t.Fatalf("multiple range Content-Type = %q, %v; want multipart/byteranges with boundary", response.Header().Get("Content-Type"), err)
	}

	reader := multipart.NewReader(response.Body, params["boundary"])
	wantRanges := []struct {
		contentRange string
		body         string
	}{
		{contentRange: "bytes 0-1/10", body: "01"},
		{contentRange: "bytes 4-5/10", body: "45"},
	}
	for index, want := range wantRanges {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatalf("multipart range %d: %v", index, err)
		}
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read multipart range %d: %v", index, err)
		}
		if got := part.Header.Get("Content-Range"); got != want.contentRange || string(body) != want.body {
			t.Fatalf("multipart range %d = Content-Range %q body %q, want %q body %q", index, got, body, want.contentRange, want.body)
		}
	}
	if part, err := reader.NextPart(); err != io.EOF {
		if err != nil {
			t.Fatalf("read extra multipart range: %v", err)
		}
		body, readErr := io.ReadAll(part)
		t.Fatalf("unexpected extra multipart range Content-Range=%q body=%q read error=%v", part.Header.Get("Content-Range"), body, readErr)
	}
}

func TestContentTypeMatchesMediaWhitelist(t *testing.T) {
	cases := map[string]string{
		".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".jfif": "image/jpeg",
		".png": "image/png", ".webp": "image/webp", ".avif": "image/avif",
		".heic": "image/heic", ".heif": "image/heif", ".gif": "image/gif",
		".apng": "image/apng", ".bmp": "image/bmp", ".jxl": "image/jxl",
		".mp4": "video/mp4", ".mov": "video/quicktime", ".m4v": "video/mp4",
		".webm": "video/webm",
		".ogv":  "application/octet-stream", ".ogg": "application/octet-stream",
		".xml": "application/octet-stream",
	}
	for extension, want := range cases {
		t.Run(extension, func(t *testing.T) {
			if got := contentType(extension); got != want {
				t.Fatalf("contentType(%q) = %q, want %q", extension, got, want)
			}
			if got := contentType(strings.ToUpper(extension)); got != want {
				t.Fatalf("contentType(%q) = %q, want %q", strings.ToUpper(extension), got, want)
			}
		})
	}
}

type failingOpener struct {
	err error
}

func (opener failingOpener) Open(string) (*os.File, fs.FileInfo, error) {
	return nil, nil, opener.err
}

func TestMissingFileCallbackOnlyRunsForConfirmedMissingPath(t *testing.T) {
	cases := []struct {
		name         string
		err          error
		wantStatus   int
		wantBody     string
		wantCallback bool
	}{
		{
			name:       "wrapped missing",
			err:        fmt.Errorf("catalog open: %w", os.ErrNotExist),
			wantStatus: 404, wantBody: "file not found\n", wantCallback: true,
		},
		{
			name:       "unsafe path",
			err:        fmt.Errorf("catalog open: %w", catalog.ErrUnsafePath),
			wantStatus: 404, wantBody: "file not found\n",
		},
		{
			name:       "unsupported extension",
			err:        catalog.ErrUnsupported,
			wantStatus: 404, wantBody: "file not found\n",
		},
		{
			name:       "permission denied",
			err:        os.ErrPermission,
			wantStatus: 500, wantBody: "file unavailable\n",
		},
		{
			name:       "other I/O error",
			err:        errors.New("disk read failed"),
			wantStatus: 500, wantBody: "file unavailable\n",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var callbackPaths []string
			handler := New(failingOpener{err: test.err}, func(relativePath string) error {
				callbackPaths = append(callbackPaths, relativePath)
				return errors.New("index update failed")
			})
			request := httptest.NewRequest("GET", "/api/stream/missing.jpg", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request, "missing.jpg")

			if response.Code != test.wantStatus || response.Body.String() != test.wantBody {
				t.Fatalf("response = status %d body %q, want status %d body %q", response.Code, response.Body.String(), test.wantStatus, test.wantBody)
			}
			if test.wantCallback {
				if len(callbackPaths) != 1 || callbackPaths[0] != "missing.jpg" {
					t.Fatalf("callback paths = %v, want [missing.jpg]", callbackPaths)
				}
			} else if len(callbackPaths) != 0 {
				t.Fatalf("callback paths = %v, want no callback", callbackPaths)
			}
		})
	}
}

type indexScannerFunc func(context.Context, string) ([]mediaindex.MediaFile, []string, error)

func (scanner indexScannerFunc) ScanDirectory(ctx context.Context, relativeDir string) ([]mediaindex.MediaFile, []string, error) {
	return scanner(ctx, relativeDir)
}

func TestConcurrentMissingStreamsRemoveStaleReadyIndexPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "live.jpg"), []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	media, err := catalog.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()

	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	manager, err := mediaindex.New(dbPath, root, indexScannerFunc(func(_ context.Context, relativeDir string) ([]mediaindex.MediaFile, []string, error) {
		if relativeDir != "." {
			return nil, nil, fmt.Errorf("unexpected directory %q", relativeDir)
		}
		return []mediaindex.MediaFile{
			{RelativePath: "gone.jpg", MediaType: "image"},
			{RelativePath: "live.jpg", MediaType: "image"},
		}, nil, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitForIndexReady(t, manager)

	handler := New(media, manager.RemoveMissing)
	const requestCount = 12
	startRequests := make(chan struct{})
	responses := make(chan int, requestCount)
	var requests sync.WaitGroup
	requests.Add(requestCount)
	for range requestCount {
		go func() {
			defer requests.Done()
			<-startRequests
			request := httptest.NewRequest(http.MethodGet, "/api/stream/gone.jpg", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request, "gone.jpg")
			responses <- response.Code
		}()
	}
	close(startRequests)
	finished := make(chan struct{})
	go func() {
		requests.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent missing stream requests did not finish")
	}
	close(responses)
	for status := range responses {
		if status != http.StatusNotFound {
			t.Errorf("missing stream status = %d, want 404", status)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var staleRows int
	if err := db.QueryRow("SELECT count(*) FROM media_files WHERE relative_path = ?", "gone.jpg").Scan(&staleRows); err != nil {
		t.Fatal(err)
	}
	if staleRows != 0 {
		t.Fatalf("database still contains %d rows for missing path", staleRows)
	}
	paths, ready, err := manager.Sample(context.Background(), "image")
	if err != nil || !ready || len(paths) != 1 || paths[0] != "live.jpg" {
		t.Fatalf("sample after missing streams = %v, ready=%t err=%v; want [live.jpg]", paths, ready, err)
	}
}

func waitForIndexReady(t *testing.T, manager *mediaindex.Manager) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, ready, err := manager.Sample(context.Background(), "image")
		if err == nil && ready {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for ready media index")
}
