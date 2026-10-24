package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type cancelAfterContextChecks struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (ctx *cancelAfterContextChecks) Err() error {
	ctx.checks++
	if ctx.checks == 2 {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func TestListRecursesFiltersAndSkipsNASMetadata(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"一级/假期 你好.JPG":       "image",
		"second/photo.webp":  "image",
		"second/scan.JFIF":   "image",
		"second/large.jxl":   "image",
		"@eaDir/hidden.jpg":  "hidden",
		"ignored.txt":        "ignored",
		"movie.MP4":          "video",
		"second/trailer.m4v": "video",
		"legacy.ogv":         "unsupported",
		"legacy.ogg":         "unsupported",
	} {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	images, err := catalog.List(context.Background(), "image")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 4 {
		t.Fatalf("image sample = %v", images)
	}
	imageSet := make(map[string]bool, len(images))
	for _, item := range images {
		if strings.Contains(item, "@eaDir") || strings.HasSuffix(strings.ToLower(item), ".txt") {
			t.Errorf("unexpected catalog item %q", item)
		}
		imageSet[item] = true
	}
	for _, want := range []string{"一级/假期 你好.JPG", "second/photo.webp", "second/scan.JFIF", "second/large.jxl"} {
		if !imageSet[want] {
			t.Errorf("image sample missing %q: %v", want, images)
		}
	}
	videos, err := catalog.List(context.Background(), "video")
	if err != nil || len(videos) != 2 {
		t.Fatalf("video sample = %v, %v", videos, err)
	}
	videoSet := make(map[string]bool, len(videos))
	for _, item := range videos {
		videoSet[item] = true
	}
	for _, want := range []string{"movie.MP4", "second/trailer.m4v"} {
		if !videoSet[want] {
			t.Errorf("video sample missing %q: %v", want, videos)
		}
	}
	if _, err := catalog.List(context.Background(), "audio"); err != ErrInvalidType {
		t.Fatalf("invalid media type error = %v", err)
	}
}

func TestScanDirectoryReturnsOnlyCurrentDirectoryMediaAndChildDirectories(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{
		"root.JPG",
		"sub/child.JfIf",
		"sub/deeper/photo.png",
		"@eaDir/hidden.jpg",
		"sub/@eaDir/also-hidden.webp",
		"sub/legacy.ogv",
	} {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "root.JPG"), filepath.Join(root, "alias.JPEG")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "sub"), filepath.Join(root, "linked-sub")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	rootFiles, rootDirs, err := catalog.ScanDirectory(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(rootFiles) != 1 || rootFiles[0] != (MediaFile{RelativePath: "root.JPG", MediaType: "image"}) {
		t.Fatalf("root scan files = %v, want root.JPG image", rootFiles)
	}
	if len(rootDirs) != 1 || rootDirs[0] != "sub" {
		t.Fatalf("root scan child directories = %v, want [sub]", rootDirs)
	}

	childFiles, childDirs, err := catalog.ScanDirectory(context.Background(), "sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(childFiles) != 1 || childFiles[0] != (MediaFile{RelativePath: "sub/child.JfIf", MediaType: "image"}) {
		t.Fatalf("sub scan files = %v, want sub/child.JfIf image", childFiles)
	}
	if len(childDirs) != 1 || childDirs[0] != "sub/deeper" {
		t.Fatalf("sub scan child directories = %v, want [sub/deeper]", childDirs)
	}

	if files, dirs, err := catalog.ScanDirectory(context.Background(), "../"); !errors.Is(err, ErrUnsafePath) || files != nil || dirs != nil {
		t.Fatalf("unsafe scan = %v, %v, %v; want nil results and ErrUnsafePath", files, dirs, err)
	}
}

func TestWhitelistIsCaseInsensitiveAndExcludesLegacyVideoExtensions(t *testing.T) {
	root := t.TempDir()
	imageExtensions := []string{".jpg", ".jpeg", ".jfif", ".png", ".webp", ".avif", ".heic", ".heif", ".gif", ".apng", ".bmp", ".jxl"}
	videoExtensions := []string{".mp4", ".mov", ".m4v", ".webm"}
	for index, extension := range imageExtensions {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("image-%02d%s", index, strings.ToUpper(extension))), []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for index, extension := range videoExtensions {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("video-%02d%s", index, strings.ToUpper(extension))), []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"legacy.ogv", "legacy.ogg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("legacy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	files, _, err := catalog.ScanDirectory(context.Background(), ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(imageExtensions)+len(videoExtensions) {
		t.Fatalf("whitelist scan returned %d files, want %d: %v", len(files), len(imageExtensions)+len(videoExtensions), files)
	}
	seen := make(map[string]string, len(files))
	for _, file := range files {
		seen[file.RelativePath] = file.MediaType
	}
	for index, extension := range imageExtensions {
		name := fmt.Sprintf("image-%02d%s", index, strings.ToUpper(extension))
		if seen[name] != "image" {
			t.Errorf("whitelisted image %q has media type %q", name, seen[name])
		}
	}
	for index, extension := range videoExtensions {
		name := fmt.Sprintf("video-%02d%s", index, strings.ToUpper(extension))
		if seen[name] != "video" {
			t.Errorf("whitelisted video %q has media type %q", name, seen[name])
		}
	}
	for _, name := range []string{"legacy.ogv", "legacy.ogg"} {
		if _, ok := seen[name]; ok {
			t.Errorf("legacy extension %q was returned", name)
		}
	}
}

func TestOpenAcceptsOnlyCurrentMediaWhitelist(t *testing.T) {
	root := t.TempDir()
	imageExtensions := []string{".jpg", ".jpeg", ".jfif", ".png", ".webp", ".avif", ".heic", ".heif", ".gif", ".apng", ".bmp", ".jxl"}
	videoExtensions := []string{".mp4", ".mov", ".m4v", ".webm"}
	var names []string
	for index, extension := range append(imageExtensions, videoExtensions...) {
		name := fmt.Sprintf("valid-%02d%s", index, strings.ToUpper(extension))
		if err := os.WriteFile(filepath.Join(root, name), []byte("media"), 0o600); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	for _, name := range []string{"legacy.ogv", "legacy.ogg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("legacy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	for _, name := range names {
		file, _, err := catalog.Open(name)
		if err != nil {
			t.Errorf("Open(%q) error = %v", name, err)
			continue
		}
		_ = file.Close()
	}
	for _, name := range []string{"legacy.ogv", "legacy.ogg"} {
		if file, _, err := catalog.Open(name); !errors.Is(err, ErrUnsupported) {
			if file != nil {
				_ = file.Close()
			}
			t.Errorf("Open(%q) error = %v, want ErrUnsupported", name, err)
		}
	}
}

func TestOpenRejectsTraversalUnsupportedAndSymlinks(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "media")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "outside.jpg")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.jpg")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("not media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("PRIVATE=secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "privkey.pem"), []byte("private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideDirectory := filepath.Join(parent, "outside-directory")
	if err := os.Mkdir(outsideDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDirectory, "secret.jpg"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	directorySymlink := filepath.Join(root, "linked-directory")
	directorySymlinkCreated := os.Symlink(outsideDirectory, directorySymlink) == nil
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	values := []string{"../outside.jpg", "/outside.jpg", "link.jpg", "directory", "note.txt", ".env", "privkey.pem", "a\\..\\outside.jpg"}
	if directorySymlinkCreated {
		values = append(values, "linked-directory/secret.jpg")
	}
	for _, value := range values {
		file, _, err := catalog.Open(value)
		if err == nil {
			_ = file.Close()
			t.Errorf("Open(%q) unexpectedly succeeded", value)
		}
	}
}

func TestOpenFailsIfMediaIsRemoved(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(name, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if file, _, err := catalog.Open("photo.jpg"); !errors.Is(err, os.ErrNotExist) {
		if file != nil {
			_ = file.Close()
		}
		t.Fatalf("Open after removal error = %v, want file not found", err)
	}
}

func TestListHandlesEmptyCanceledAndConcurrentScans(t *testing.T) {
	root := t.TempDir()
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	empty, err := catalog.List(context.Background(), "image")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty directory list = %v, %v; want empty, nil", empty, err)
	}

	for index := 0; index < 20; index++ {
		name := filepath.Join(root, fmt.Sprintf("%02d.jpg", index))
		if err := os.WriteFile(name, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	catalog.scanGate <- struct{}{}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := catalog.List(canceled, "image"); !errors.Is(err, context.Canceled) {
		t.Fatalf("list with canceled context error = %v, want context canceled", err)
	}
	<-catalog.scanGate

	var group sync.WaitGroup
	for index := 0; index < 8; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			items, err := catalog.List(context.Background(), "image")
			if err != nil || len(items) != SampleLimit {
				t.Errorf("concurrent list = %d items, err=%v; want %d", len(items), err, SampleLimit)
			}
		}()
	}
	group.Wait()
}

func TestListReportsRootScanFailure(t *testing.T) {
	catalog, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := catalog.List(context.Background(), "image"); !errors.Is(err, ErrRootScan) {
		t.Fatalf("list after root scan failure error = %v, want ErrRootScan", err)
	}
}

func TestListReportsPartialChildDirectoryFailure(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "unreadable")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "root.jpg"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "child.jpg"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(child, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(child, 0o700) })
	if _, err := os.ReadDir(child); err == nil {
		t.Skip("process can still read mode-000 directories")
	}

	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	files, dirs, err := catalog.ScanDirectory(context.Background(), "unreadable")
	if err == nil || files != nil || dirs != nil {
		t.Fatalf("unreadable directory scan = %v, %v, %v; want nil results and an error", files, dirs, err)
	}

	items, err := catalog.List(context.Background(), "image")
	if !errors.Is(err, ErrPartialScan) {
		t.Fatalf("list with unreadable child error = %v, want ErrPartialScan", err)
	}
	if len(items) != 1 || items[0] != "root.jpg" {
		t.Fatalf("partial scan items = %v, want root.jpg", items)
	}
}

func TestListPreservesCancellationDuringDirectoryRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()

	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelAfterContextChecks{Context: base, cancel: cancel}
	_, err = catalog.List(ctx, "image")
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrRootScan) || errors.Is(err, ErrPartialScan) {
		t.Fatalf("list cancellation error = %v, want context.Canceled without scan failure classification", err)
	}
}

func TestListSkipsSymlinkDirectories(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "media")
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.jpg"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	items, err := catalog.List(context.Background(), "image")
	if err != nil || len(items) != 0 {
		t.Fatalf("list followed symlink directory: items=%v err=%v", items, err)
	}
}

func TestListIsBounded(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 25; index++ {
		name := filepath.Join(root, fmt.Sprintf("%02d.jpg", index))
		if err := os.WriteFile(name, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	items, err := catalog.List(context.Background(), "image")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != SampleLimit {
		t.Fatalf("sample size = %d, want %d", len(items), SampleLimit)
	}
}

func TestListScansPastDirectoryReadBatches(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 300; index++ {
		name := filepath.Join(root, fmt.Sprintf("%03d.jpg", index))
		if err := os.WriteFile(name, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Close()
	catalog.max = 300

	items, err := catalog.List(context.Background(), "image")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 300 {
		t.Fatalf("sample size across directory batches = %d, want 300", len(items))
	}
}
