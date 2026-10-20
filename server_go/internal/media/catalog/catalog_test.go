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

func TestListRecursesFiltersAndSkipsNASMetadata(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"一级/假期 你好.JPG":      "image",
		"second/photo.webp": "image",
		"@eaDir/hidden.jpg": "hidden",
		"ignored.txt":       "ignored",
		"movie.MP4":         "video",
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
	if len(images) != 2 {
		t.Fatalf("image sample = %v", images)
	}
	for _, item := range images {
		if strings.Contains(item, "@eaDir") || strings.HasSuffix(strings.ToLower(item), ".txt") {
			t.Errorf("unexpected catalog item %q", item)
		}
	}
	videos, err := catalog.List(context.Background(), "video")
	if err != nil || len(videos) != 1 || videos[0] != "movie.MP4" {
		t.Fatalf("video sample = %v, %v", videos, err)
	}
	if _, err := catalog.List(context.Background(), "audio"); err != ErrInvalidType {
		t.Fatalf("invalid media type error = %v", err)
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
