package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"
)

const sampleLimit = 10

var (
	ErrNotReady         = errors.New("media index is not ready")
	ErrScanInProgress   = errors.New("media index scan is already in progress")
	ErrClosed           = errors.New("media index manager is closed")
	ErrInvalidType      = errors.New("media type must be image or video")
	ErrSnapshotMismatch = errors.New("media index database and ID snapshot disagree")
)

// MediaFile describes an image or video file found under the media root.
// RelativePath uses slash separators and is relative to that root.
type MediaFile struct {
	RelativePath string
	MediaType    string
}

// Scanner reads one directory and returns its files and immediate child
// directories. The root directory is represented by ".".
type Scanner interface {
	ScanDirectory(ctx context.Context, relativeDir string) (files []MediaFile, childDirs []string, err error)
}

// Manager owns the persistent media index and its background directory scan.
type Manager struct {
	dbPath    string
	mediaRoot string
	scanner   Scanner
	logger    *log.Logger

	lifecycle    sync.Mutex
	mu           sync.RWMutex
	db           *sql.DB
	ready        bool
	ids          map[string][]int64
	opened       bool
	closed       bool
	scanning     bool
	generation   uint64
	workerCancel context.CancelFunc
	workerWG     sync.WaitGroup
}

func New(dbPath, mediaRoot string, scanner Scanner, logger *log.Logger) (*Manager, error) {
	if dbPath == "" {
		return nil, errors.New("media index database path is required")
	}
	if mediaRoot == "" {
		return nil, errors.New("media root is required")
	}
	if scanner == nil {
		return nil, errors.New("media index scanner is required")
	}
	if logger == nil {
		logger = log.Default()
	}
	absoluteRoot, err := filepath.Abs(mediaRoot)
	if err != nil {
		return nil, fmt.Errorf("media root path is invalid: %w", err)
	}
	return &Manager{
		dbPath:    dbPath,
		mediaRoot: filepath.Clean(absoluteRoot),
		scanner:   scanner,
		logger:    logger,
		ids:       map[string][]int64{"image": nil, "video": nil},
	}, nil
}

// Start opens or resumes the index. A usable ready index is loaded without a
// media-root scan; a missing or building index is scanned in the background.
// On an initialization error the manager stays in HDD mode and the error is
// returned for logging by the caller.
func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()

	m.mu.RLock()
	closed, opened := m.closed, m.opened
	m.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	if opened {
		return nil
	}

	if err := ensureDatabaseParent(m.dbPath); err != nil {
		m.logger.Printf("media index: create database directory: %v", err)
		return err
	}
	db, err := openDatabase(ctx, m.dbPath)
	if err != nil {
		m.logger.Printf("media index: open database: %v", err)
		return err
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = db.Close()
		}
	}()

	if err := ensureSchema(ctx, db); err != nil {
		m.logger.Printf("media index: prepare schema: %v", err)
		return err
	}
	state, err := readState(ctx, db)
	freshBuild := false
	if errors.Is(err, sql.ErrNoRows) {
		if err := initializeFreshIndex(ctx, db, m.mediaRoot); err != nil {
			m.logger.Printf("media index: initialize index: %v", err)
			return err
		}
		state, err = readState(ctx, db)
		if err != nil {
			return fmt.Errorf("read initialized index state: %w", err)
		}
		freshBuild = true
	} else if err != nil {
		m.logger.Printf("media index: read state: %v", err)
		return err
	} else if state.mediaRoot != m.mediaRoot {
		if err := resetForRoot(ctx, db, m.mediaRoot); err != nil {
			m.logger.Printf("media index: reset changed media root: %v", err)
			return err
		}
		state, err = readState(ctx, db)
		if err != nil {
			return fmt.Errorf("read reset index state: %w", err)
		}
		freshBuild = true
		m.logger.Printf("media index: media root changed; starting a fresh build")
	}

	var loaded map[string][]int64
	if state.status == "ready" {
		pending, err := queueHasEntries(ctx, db)
		if err != nil {
			m.logger.Printf("media index: check ready queue: %v", err)
			return err
		}
		if pending {
			return errors.New("media index is marked ready with an unfinished scan queue")
		}
		loaded, err = loadIDs(ctx, db)
		if err != nil {
			m.logger.Printf("media index: load ready IDs: %v", err)
			return err
		}
	} else if state.status != "building" && state.status != "failed" {
		return fmt.Errorf("media index has invalid status %q", state.status)
	}
	var images, videos, queuedDirs int64
	if state.status == "building" {
		images, videos, _, queuedDirs, _, err = indexCounts(ctx, db)
		if err != nil {
			m.logger.Printf("media index: summarize building state: %v", err)
			return err
		}
	}

	m.mu.Lock()
	m.db = db
	m.opened = true
	m.generation++
	generation := m.generation
	if state.status == "ready" {
		m.ids = loaded
		m.ready = true
		m.mu.Unlock()
		closeOnError = false
		m.logger.Printf("media index: loaded ready index (images=%d videos=%d)", len(loaded["image"]), len(loaded["video"]))
		return nil
	}
	m.ready = false
	m.ids = map[string][]int64{"image": nil, "video": nil}
	if state.status == "building" {
		m.startWorkerLocked(ctx, generation)
		m.mu.Unlock()
		closeOnError = false
		if freshBuild {
			m.logger.Printf("media index: initial-start timestamp=%s media_root=%q", time.Unix(state.startedAt, 0).UTC().Format(time.RFC3339), m.mediaRoot)
		} else {
			m.logger.Printf("media index: building recovery recovered_at=%s build_started_at=%s images=%d videos=%d queued_dirs=%d processed_dirs=%d media_root=%q",
				time.Now().UTC().Format(time.RFC3339), time.Unix(state.startedAt, 0).UTC().Format(time.RFC3339), images, videos, queuedDirs, state.processedDirs, m.mediaRoot)
		}
		return nil
	}
	m.mu.Unlock()
	closeOnError = false
	m.logger.Printf("media index: persisted state is failed; remaining in HDD mode")
	return nil
}

func (m *Manager) startWorkerLocked(parent context.Context, generation uint64) {
	ctx, cancel := context.WithCancel(parent)
	m.workerCancel = cancel
	m.scanning = true
	db := m.db
	m.workerWG.Add(1)
	go func() {
		defer m.workerWG.Done()
		defer cancel()
		m.runWorker(ctx, db, generation)
		m.mu.Lock()
		if m.generation == generation {
			m.scanning = false
			m.workerCancel = nil
		}
		m.mu.Unlock()
	}()
}

// Close cancels the background worker, waits for it to stop, and then closes
// the database connection.
func (m *Manager) Close() error {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.ready = false
	m.ids = map[string][]int64{"image": nil, "video": nil}
	m.generation++
	cancel := m.workerCancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.workerWG.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.scanning = false
	m.workerCancel = nil
	if m.db == nil {
		return nil
	}
	err := m.db.Close()
	m.db = nil
	m.opened = false
	if err != nil {
		m.logger.Printf("media index: close database: %v", err)
	}
	return err
}
