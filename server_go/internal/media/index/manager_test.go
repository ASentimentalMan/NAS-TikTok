package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type scannerFunc func(context.Context, string) ([]MediaFile, []string, error)

func (f scannerFunc) ScanDirectory(ctx context.Context, relativeDir string) ([]MediaFile, []string, error) {
	return f(ctx, relativeDir)
}

func quietLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func newTestManager(t *testing.T, dbPath string, scanner Scanner) *Manager {
	t.Helper()
	manager, err := New(dbPath, filepath.Join(t.TempDir(), "media"), scanner, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func waitFor(t *testing.T, description string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func openTestDatabase(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	if err := ensureDatabaseParent(dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := openDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureSchema(context.Background(), db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

func createReadyDatabase(t *testing.T, dbPath, mediaRoot string, files []MediaFile) {
	t.Helper()
	db := openTestDatabase(t, dbPath)
	tx, err := db.Begin()
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_, err = tx.Exec(`INSERT INTO index_state
		(id, status, started_at, finished_at, processed_dirs, last_error, media_root)
		VALUES (1, 'ready', ?, ?, 1, NULL, ?)`, timestampNow(), timestampNow(), filepath.Clean(mediaRoot))
	if err != nil {
		_ = tx.Rollback()
		_ = db.Close()
		t.Fatal(err)
	}
	for i, file := range files {
		_, err := tx.Exec("INSERT INTO media_files(id, relative_path, media_type) VALUES (?, ?, ?)", (i+1)*7, file.RelativePath, file.MediaType)
		if err != nil {
			_ = tx.Rollback()
			_ = db.Close()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaConstraintsAndTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	manager := newTestManager(t, dbPath, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		return nil, nil, nil
	}))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "initial index build", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})
	defer manager.Close()

	m := map[string]bool{}
	var startedAtType string
	if err := manager.db.QueryRow("SELECT typeof(started_at) FROM index_state WHERE id = 1").Scan(&startedAtType); err != nil {
		t.Fatal(err)
	}
	if startedAtType != "integer" {
		t.Fatalf("started_at has SQLite type %q, want integer", startedAtType)
	}
	var finishedAtType string
	if err := manager.db.QueryRow("SELECT typeof(finished_at) FROM index_state WHERE id = 1").Scan(&finishedAtType); err != nil {
		t.Fatal(err)
	}
	if finishedAtType != "integer" {
		t.Fatalf("finished_at has SQLite type %q, want integer", finishedAtType)
	}
	var tables int
	if err := manager.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 3 {
		t.Fatalf("expected exactly 3 index tables, found %d", tables)
	}
	rows, err := manager.db.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		m[name] = true
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, map[string]bool{"media_files": true, "index_state": true, "scan_queue": true}) {
		t.Fatalf("unexpected tables: %#v", m)
	}
	if _, err := manager.db.Exec("INSERT INTO media_files(relative_path, media_type) VALUES ('bad.bin', 'audio')"); err == nil {
		t.Fatal("invalid media type passed the schema constraint")
	}
	if _, err := manager.db.Exec("UPDATE index_state SET status = 'unknown' WHERE id = 1"); err == nil {
		t.Fatal("invalid status passed the schema constraint")
	}
	if _, err := manager.db.Exec(`INSERT INTO index_state
		(id, status, started_at, finished_at, processed_dirs, last_error, media_root)
		VALUES (2, 'building', 0, NULL, 0, NULL, 'root')`); err == nil {
		t.Fatal("index_state accepted an id other than 1")
	}
}

func TestBuildingIndexResumesFromQueue(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	started := make(chan struct{})
	first, err := New(dbPath, root, scannerFunc(func(ctx context.Context, relativeDir string) ([]MediaFile, []string, error) {
		if relativeDir == "." {
			return []MediaFile{{RelativePath: "root.jpg", MediaType: "image"}}, []string{"sub"}, nil
		}
		close(started)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach the child directory")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	db := openTestDatabase(t, dbPath)
	var status string
	var processed int
	if err := db.QueryRow("SELECT status, processed_dirs FROM index_state WHERE id = 1").Scan(&status, &processed); err != nil {
		t.Fatal(err)
	}
	var queued string
	if err := db.QueryRow("SELECT relative_dir FROM scan_queue").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if status != "building" || processed != 1 || queued != "sub" {
		t.Fatalf("unexpected checkpoint: status=%q processed=%d queued=%q", status, processed, queued)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var callsMu sync.Mutex
	var calls []string
	second, err := New(dbPath, root, scannerFunc(func(_ context.Context, relativeDir string) ([]MediaFile, []string, error) {
		callsMu.Lock()
		calls = append(calls, relativeDir)
		callsMu.Unlock()
		if relativeDir != "sub" {
			return nil, nil, fmt.Errorf("unexpected directory %q", relativeDir)
		}
		return []MediaFile{{RelativePath: "sub/child.jpg", MediaType: "image"}}, nil, nil
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "resumed index build", func() bool {
		_, ready, err := second.Sample(context.Background(), "image")
		return err == nil && ready
	})
	defer second.Close()
	callsMu.Lock()
	defer callsMu.Unlock()
	if !reflect.DeepEqual(calls, []string{"sub"}) {
		t.Fatalf("resume rescanned completed directories: %v", calls)
	}
	var count int
	if err := second.db.QueryRow("SELECT count(*) FROM media_files").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected root and child file rows, found %d", count)
	}
}

func TestFailedDirectoryPreservesQueueAndDoesNotAutoRetry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	var calls atomic.Int32
	manager, err := New(dbPath, root, scannerFunc(func(_ context.Context, relativeDir string) ([]MediaFile, []string, error) {
		calls.Add(1)
		if relativeDir == "." {
			return []MediaFile{{RelativePath: "root.jpg", MediaType: "image"}}, []string{"broken"}, nil
		}
		return nil, nil, errors.New("directory read failed")
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "failed index state", func() bool {
		var status string
		return manager.db.QueryRow("SELECT status FROM index_state WHERE id = 1").Scan(&status) == nil && status == "failed"
	})
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	var resumedCalls atomic.Int32
	resumed, err := New(dbPath, root, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		resumedCalls.Add(1)
		return nil, nil, nil
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumedCalls.Load() != 0 {
		t.Fatalf("failed scan state retried automatically %d times", resumedCalls.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("expected root and failing directory calls, found %d", calls.Load())
	}
	var processed int
	var queued, lastError string
	if err := resumed.db.QueryRow("SELECT processed_dirs FROM index_state WHERE id = 1").Scan(&processed); err != nil {
		t.Fatal(err)
	}
	if err := resumed.db.QueryRow("SELECT relative_dir FROM scan_queue").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if err := resumed.db.QueryRow("SELECT last_error FROM index_state WHERE id = 1").Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	if processed != 1 || queued != "broken" || !strings.Contains(lastError, "directory read failed") {
		t.Fatalf("failure checkpoint not preserved: processed=%d queued=%q error=%q", processed, queued, lastError)
	}
}

func TestReadyReloadDoesNotCallScanner(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	first, err := New(dbPath, root, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		return []MediaFile{{RelativePath: "a.jpg", MediaType: "image"}}, nil, nil
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "ready initial build", func() bool {
		_, ready, err := first.Sample(context.Background(), "image")
		return err == nil && ready
	})
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	second, err := New(dbPath, root, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		calls.Add(1)
		return nil, nil, errors.New("ready index should not scan")
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if calls.Load() != 0 {
		t.Fatalf("ready index called scanner %d times", calls.Load())
	}
	paths, ready, err := second.Sample(context.Background(), "image")
	if err != nil || !ready || !reflect.DeepEqual(paths, []string{"a.jpg"}) {
		t.Fatalf("unexpected ready sample: paths=%v ready=%t err=%v", paths, ready, err)
	}
}

func TestChangedMediaRootBuildsFreshIndex(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	oldRoot := filepath.Join(t.TempDir(), "old-media")
	newRoot := filepath.Join(t.TempDir(), "new-media")
	createReadyDatabase(t, dbPath, oldRoot, []MediaFile{
		{RelativePath: "stale.jpg", MediaType: "image"},
	})
	var calls atomic.Int32
	manager, err := New(dbPath, newRoot, scannerFunc(func(_ context.Context, relativeDir string) ([]MediaFile, []string, error) {
		calls.Add(1)
		if relativeDir != "." {
			return nil, nil, fmt.Errorf("unexpected directory %q", relativeDir)
		}
		return []MediaFile{{RelativePath: "fresh.jpg", MediaType: "image"}}, nil, nil
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	waitFor(t, "fresh index for changed media root", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})
	paths, ready, err := manager.Sample(context.Background(), "image")
	if err != nil || !ready || !reflect.DeepEqual(paths, []string{"fresh.jpg"}) {
		t.Fatalf("stale index was served after root change: paths=%v ready=%t err=%v", paths, ready, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one fresh root scan, got %d", calls.Load())
	}
	var storedRoot string
	if err := manager.db.QueryRow("SELECT media_root FROM index_state WHERE id = 1").Scan(&storedRoot); err != nil {
		t.Fatal(err)
	}
	if storedRoot != filepath.Clean(newRoot) {
		t.Fatalf("stored media root = %q, want %q", storedRoot, filepath.Clean(newRoot))
	}
}

func TestSampleUsesUniqueNoncontiguousIDsAndIndexedLookup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	files := make([]MediaFile, 20)
	wantPaths := make(map[string]bool, len(files))
	for i := range files {
		files[i] = MediaFile{RelativePath: fmt.Sprintf("image-%02d.jpg", i), MediaType: "image"}
		wantPaths[files[i].RelativePath] = true
	}
	createReadyDatabase(t, dbPath, root, files)
	manager, err := New(dbPath, root, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		return nil, nil, errors.New("should not scan")
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	paths, ready, err := manager.Sample(context.Background(), "image")
	if err != nil || !ready {
		t.Fatalf("sample failed: ready=%t err=%v", ready, err)
	}
	if len(paths) != sampleLimit {
		t.Fatalf("expected %d paths, found %d", sampleLimit, len(paths))
	}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !wantPaths[path] {
			t.Fatalf("sample returned unknown path %q", path)
		}
		if seen[path] {
			t.Fatalf("sample returned duplicate path %q", path)
		}
		seen[path] = true
	}

	var seq, parent, notUsed int
	var detail string
	if err := manager.db.QueryRow(`EXPLAIN QUERY PLAN SELECT relative_path FROM media_files
		WHERE id = ? AND media_type = ?`, 7, "image").Scan(&seq, &parent, &notUsed, &detail); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "SEARCH media_files USING INTEGER PRIMARY KEY") || strings.Contains(detail, "SCAN media_files") {
		t.Fatalf("path lookup is not using the primary-key index: %q", detail)
	}
}

func TestRemoveMissingKeepsSnapshotOnDatabaseError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	files := make([]MediaFile, 11)
	for i := range files {
		files[i] = MediaFile{RelativePath: fmt.Sprintf("image-%02d.jpg", i), MediaType: "image"}
	}
	createReadyDatabase(t, dbPath, root, files)
	manager, err := New(dbPath, root, scannerFunc(func(context.Context, string) ([]MediaFile, []string, error) {
		return nil, nil, nil
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err := manager.db.Exec(`CREATE TRIGGER deny_media_delete BEFORE DELETE ON media_files
		BEGIN SELECT RAISE(ABORT, 'delete denied'); END`); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveMissing("image-00.jpg"); err == nil {
		t.Fatal("expected the database trigger to reject deletion")
	}
	manager.mu.RLock()
	if len(manager.ids["image"]) != 11 {
		manager.mu.RUnlock()
		t.Fatalf("ID snapshot changed after database error: %d IDs", len(manager.ids["image"]))
	}
	manager.mu.RUnlock()
	if _, err := manager.db.Exec("DROP TRIGGER deny_media_delete"); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveMissing("image-00.jpg"); err != nil {
		t.Fatal(err)
	}
	paths, ready, err := manager.Sample(context.Background(), "image")
	if err != nil || !ready || len(paths) != sampleLimit {
		t.Fatalf("unexpected sample after deletion: paths=%d ready=%t err=%v", len(paths), ready, err)
	}
	for _, path := range paths {
		if path == "image-00.jpg" {
			t.Fatal("deleted path remained in the in-memory sample")
		}
	}
}

func TestSampleRemoveMissingAndReinitializeConcurrent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	root := filepath.Join(t.TempDir(), "media")
	createReadyDatabase(t, dbPath, root, []MediaFile{
		{RelativePath: "old-1.jpg", MediaType: "image"},
		{RelativePath: "gone.jpg", MediaType: "image"},
		{RelativePath: "old-1.mp4", MediaType: "video"},
	})

	scanStarted := make(chan struct{})
	releaseScan := make(chan struct{})
	var releaseOnce sync.Once
	manager, err := New(dbPath, root, scannerFunc(func(ctx context.Context, relativeDir string) ([]MediaFile, []string, error) {
		if relativeDir != "." {
			return nil, nil, fmt.Errorf("unexpected directory %q", relativeDir)
		}
		close(scanStarted)
		select {
		case <-releaseScan:
			return []MediaFile{
				{RelativePath: "fresh-1.jpg", MediaType: "image"},
				{RelativePath: "fresh-2.jpg", MediaType: "image"},
				{RelativePath: "fresh.mp4", MediaType: "video"},
			}, nil, nil
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}), quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		releaseOnce.Do(func() { close(releaseScan) })
		if err := manager.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	}()

	waitFor(t, "initial ready index", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})

	knownPaths := map[string]bool{
		"old-1.jpg": true, "gone.jpg": true, "old-1.mp4": true,
		"fresh-1.jpg": true, "fresh-2.jpg": true, "fresh.mp4": true,
	}
	connectionHold, err := manager.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connectionHold.Rollback()
	waitCount := manager.db.Stats().WaitCount
	stopSamples := make(chan struct{})
	var stopSamplesOnce sync.Once
	samplerDone := make(chan struct{})
	sampleErrors := make(chan error, 1)
	firstSample := make(chan struct {
		paths []string
		ready bool
		err   error
	}, 1)
	var sampleCalls atomic.Int64
	go func() {
		defer close(samplerDone)
		for {
			select {
			case <-stopSamples:
				return
			default:
			}
			paths, ready, err := manager.Sample(context.Background(), "image")
			if sampleCalls.Add(1) == 1 {
				firstSample <- struct {
					paths []string
					ready bool
					err   error
				}{paths: paths, ready: ready, err: err}
			}
			if err != nil {
				select {
				case sampleErrors <- fmt.Errorf("sample during mutation: %w", err):
				default:
				}
				return
			}
			if ready {
				for _, path := range paths {
					if !knownPaths[path] {
						select {
						case sampleErrors <- fmt.Errorf("sample returned unknown path %q", path):
						default:
						}
						return
					}
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	defer stopSamplesOnce.Do(func() { close(stopSamples) })
	waitFor(t, "sample waiting for the held database connection", func() bool {
		return manager.db.Stats().WaitCount > waitCount
	})

	type reinitializeResult struct {
		started bool
		err     error
	}
	reinitializeDone := make(chan reinitializeResult, 1)
	removeDone := make(chan error, 1)
	go func() {
		removeDone <- manager.RemoveMissing("gone.jpg")
	}()
	waitFor(t, "RemoveMissing waiting for the active sample", func() bool {
		if manager.mu.TryRLock() {
			manager.mu.RUnlock()
			return false
		}
		return true
	})
	go func() {
		started, err := manager.Reinitialize()
		reinitializeDone <- reinitializeResult{started: started, err: err}
	}()
	waitFor(t, "Reinitialize waiting for the active sample", func() bool {
		if manager.lifecycle.TryLock() {
			manager.lifecycle.Unlock()
			return false
		}
		return true
	})
	if err := connectionHold.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-firstSample:
		initialImageSample := map[string]bool{}
		for _, path := range result.paths {
			initialImageSample[path] = true
		}
		if result.err != nil || !result.ready || len(initialImageSample) != 2 ||
			!initialImageSample["old-1.jpg"] || !initialImageSample["gone.jpg"] {
			t.Fatalf("sample overlapping queued mutations = %v, ready=%t err=%v", result.paths, result.ready, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sample did not finish after releasing the database connection")
	}

	select {
	case <-scanStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("reinitialized scan did not start")
	}
	select {
	case result := <-reinitializeDone:
		if result.err != nil || !result.started {
			t.Fatalf("reinitialize result = (%t, %v), want started", result.started, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reinitialize did not return while its scan was blocked")
	}
	select {
	case err := <-removeDone:
		if err != nil && !errors.Is(err, ErrNotReady) {
			t.Fatalf("concurrent RemoveMissing error = %v, want nil or ErrNotReady", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RemoveMissing did not complete")
	}

	callsBeforeBlockedScan := sampleCalls.Load()
	waitFor(t, "samples during reinitialized scan", func() bool {
		return sampleCalls.Load() > callsBeforeBlockedScan
	})
	releaseOnce.Do(func() { close(releaseScan) })
	waitFor(t, "fresh ready index", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})

	stopSamplesOnce.Do(func() { close(stopSamples) })
	select {
	case <-samplerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("sampling loop did not stop")
	}
	select {
	case err := <-sampleErrors:
		t.Fatal(err)
	default:
	}

	manager.mu.RLock()
	var databaseRows int
	if err := manager.db.QueryRow("SELECT count(*) FROM media_files").Scan(&databaseRows); err != nil {
		manager.mu.RUnlock()
		t.Fatal(err)
	}
	imageIDs, videoIDs := len(manager.ids["image"]), len(manager.ids["video"])
	manager.mu.RUnlock()
	if databaseRows != imageIDs+videoIDs || databaseRows != 3 {
		t.Fatalf("fresh DB rows = %d, snapshot IDs = %d images + %d videos", databaseRows, imageIDs, videoIDs)
	}

	images, ready, err := manager.Sample(context.Background(), "image")
	imageSet := map[string]bool{}
	for _, image := range images {
		imageSet[image] = true
	}
	if err != nil || !ready || len(imageSet) != 2 || !imageSet["fresh-1.jpg"] || !imageSet["fresh-2.jpg"] {
		t.Fatalf("fresh image sample = %v, ready=%t err=%v", images, ready, err)
	}
	videos, ready, err := manager.Sample(context.Background(), "video")
	if err != nil || !ready || !reflect.DeepEqual(videos, []string{"fresh.mp4"}) {
		t.Fatalf("fresh video sample = %v, ready=%t err=%v", videos, ready, err)
	}
}

func TestReinitializeRejectsConcurrentScanAndStartsFreshBuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var calls atomic.Int32
	manager := newTestManager(t, dbPath, scannerFunc(func(ctx context.Context, relativeDir string) ([]MediaFile, []string, error) {
		calls.Add(1)
		started <- struct{}{}
		if calls.Load() == 1 {
			<-release
		} else if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		return nil, nil, nil
	}))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial scan did not start")
	}
	if started, err := manager.Reinitialize(); started || !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("reinitialize during scan = (%t, %v), want conflict", started, err)
	}
	if _, ready, err := manager.Sample(context.Background(), "image"); err != nil || ready {
		t.Fatalf("building manager should use HDD fallback: ready=%t err=%v", ready, err)
	}
	release <- struct{}{}
	waitFor(t, "initial build completion", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})
	startedFresh, err := manager.Reinitialize()
	if err != nil || !startedFresh {
		t.Fatalf("reinitialize ready index = (%t, %v), want started", startedFresh, err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("fresh scan did not start")
	}
	waitFor(t, "fresh build completion", func() bool {
		_, ready, err := manager.Sample(context.Background(), "image")
		return err == nil && ready
	})
	if calls.Load() != 2 {
		t.Fatalf("expected two root scans, found %d", calls.Load())
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseCancelsAndWaitsForScanner(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "index.sqlite")
	started := make(chan struct{})
	var returned atomic.Bool
	manager := newTestManager(t, dbPath, scannerFunc(func(ctx context.Context, _ string) ([]MediaFile, []string, error) {
		close(started)
		<-ctx.Done()
		returned.Store(true)
		return nil, nil, ctx.Err()
	}))
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scanner did not start")
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if !returned.Load() {
		t.Fatal("Close returned before scanner exited")
	}
	if _, ready, err := manager.Sample(context.Background(), "image"); err != nil || ready {
		t.Fatalf("closed manager served the index: ready=%t err=%v", ready, err)
	}
}
