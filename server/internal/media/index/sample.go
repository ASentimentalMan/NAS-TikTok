package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"time"
)

// Sample returns up to ten paths selected uniformly without replacement from
// the in-memory ID snapshot. A false ready result asks the caller to use its
// HDD-backed catalog.
func (m *Manager) Sample(ctx context.Context, mediaType string) ([]string, bool, error) {
	if mediaType != "image" && mediaType != "video" {
		return nil, false, ErrInvalidType
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.ready || m.db == nil || m.closed {
		return nil, false, nil
	}
	ids := m.ids[mediaType]
	positions := sampleIndexes(len(ids), min(sampleLimit, len(ids)))
	paths := make([]string, 0, len(positions))
	for _, position := range positions {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var relativePath string
		err := m.db.QueryRowContext(ctx, `SELECT relative_path FROM media_files
			WHERE id = ? AND media_type = ?`, ids[position], mediaType).Scan(&relativePath)
		if err != nil {
			return nil, false, fmt.Errorf("load media path for index ID %d: %w", ids[position], err)
		}
		paths = append(paths, relativePath)
	}
	return paths, true, nil
}

func sampleIndexes(n, k int) []int {
	if k > n {
		k = n
	}
	if k <= 0 {
		return nil
	}
	selected := make([]int, k)
	swaps := make(map[int]int, k)
	for i := 0; i < k; i++ {
		j := i + rand.IntN(n-i)
		valueAtJ := j
		if value, ok := swaps[j]; ok {
			valueAtJ = value
		}
		valueAtI := i
		if value, ok := swaps[i]; ok {
			valueAtI = value
		}
		swaps[j] = valueAtI
		selected[i] = valueAtJ
	}
	return selected
}

// RemoveMissing deletes a stale path only while the in-memory snapshot is
// ready. The ID slice is replaced only after the database transaction commits.
func (m *Manager) RemoveMissing(relativePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if !m.ready || m.db == nil {
		return ErrNotReady
	}

	tx, err := m.db.BeginTx(context.Background(), nil)
	if err != nil {
		m.logger.Errorf("media index: remove missing path %q: begin transaction: %v", relativePath, err)
		return err
	}
	defer tx.Rollback()
	var id int64
	var mediaType string
	err = tx.QueryRow("SELECT id, media_type FROM media_files WHERE relative_path = ?", relativePath).Scan(&id, &mediaType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		m.logger.Errorf("media index: remove missing path %q: read row: %v", relativePath, err)
		return err
	}
	previous := m.ids[mediaType]
	found := false
	for _, candidate := range previous {
		if candidate == id {
			found = true
			break
		}
	}
	if !found {
		m.logger.Errorf("media index: remove missing path %q: database ID %d is absent from the ready snapshot", relativePath, id)
		return ErrSnapshotMismatch
	}
	result, err := tx.Exec("DELETE FROM media_files WHERE id = ?", id)
	if err != nil {
		m.logger.Errorf("media index: remove missing path %q: delete row: %v", relativePath, err)
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		m.logger.Errorf("media index: remove missing path %q: count deleted rows: %v", relativePath, err)
		return err
	}
	if deleted != 1 {
		m.logger.Errorf("media index: remove missing path %q: expected one database row, deleted %d", relativePath, deleted)
		return ErrSnapshotMismatch
	}
	if err := tx.Commit(); err != nil {
		m.logger.Errorf("media index: remove missing path %q: commit deletion: %v", relativePath, err)
		return err
	}
	next := make([]int64, 0, len(previous))
	for _, candidate := range previous {
		if candidate != id {
			next = append(next, candidate)
		}
	}
	m.ids[mediaType] = next
	return nil
}

// Reinitialize closes the current database before removing its database and
// SQLite journal files, then starts a fresh build independently of any request
// context. It returns ErrScanInProgress when a build is already running.
func (m *Manager) Reinitialize() (bool, error) {
	m.lifecycle.Lock()
	defer m.lifecycle.Unlock()

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return false, ErrClosed
	}
	if m.scanning {
		m.mu.Unlock()
		return false, ErrScanInProgress
	}
	// Switch to HDD mode before cancellation, close, or file removal.
	m.ready = false
	m.ids = map[string][]int64{"image": nil, "video": nil}
	m.generation++
	cancel := m.workerCancel
	m.mu.Unlock()
	m.logger.Infof("media index: manual reinitialization switched to HDD mode at=%s media_root=%q",
		time.Now().UTC().Format(time.RFC3339), m.mediaRoot)

	if cancel != nil {
		cancel()
	}
	m.workerWG.Wait()

	m.mu.Lock()
	db := m.db
	m.db = nil
	m.opened = false
	m.workerCancel = nil
	m.scanning = false
	m.mu.Unlock()
	if db != nil {
		if err := db.Close(); err != nil {
			m.logger.Errorf("media index: close database for reinitialization: %v", err)
			return false, err
		}
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(m.dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			m.logger.Errorf("media index: remove database file %q: %v", m.dbPath+suffix, err)
			return false, err
		}
	}

	ctx := context.Background()
	if err := ensureDatabaseParent(m.dbPath); err != nil {
		m.logger.Errorf("media index: create database directory for reinitialization: %v", err)
		return false, err
	}
	newDB, err := openDatabase(ctx, m.dbPath)
	if err != nil {
		m.logger.Errorf("media index: create fresh database: %v", err)
		return false, err
	}
	if err := ensureSchema(ctx, newDB); err != nil {
		_ = newDB.Close()
		m.logger.Errorf("media index: prepare fresh schema: %v", err)
		return false, err
	}
	if err := initializeFreshIndex(ctx, newDB, m.mediaRoot); err != nil {
		_ = newDB.Close()
		m.logger.Errorf("media index: initialize fresh index: %v", err)
		return false, err
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = newDB.Close()
		return false, ErrClosed
	}
	m.db = newDB
	m.opened = true
	m.startWorkerLocked(context.Background(), m.generation)
	m.mu.Unlock()
	m.logger.Infof("media index: manual reinitialization started fresh DB at=%s media_root=%q mode=HDD",
		time.Now().UTC().Format(time.RFC3339), m.mediaRoot)
	return true, nil
}
