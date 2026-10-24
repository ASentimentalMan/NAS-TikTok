package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type indexState struct {
	status        string
	mediaRoot     string
	startedAt     int64
	processedDirs int64
}

func ensureDatabaseParent(dbPath string) error {
	parent := filepath.Dir(dbPath)
	if parent == "." || parent == string(filepath.Separator) {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	return nil
}

func openDatabase(ctx context.Context, dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// Keep PRAGMA settings, especially busy_timeout, consistent on every use
	// of the SQLite connection. Manager operations serialize all writes.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set journal mode: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	return db, nil
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS media_files (
			id INTEGER PRIMARY KEY,
			relative_path TEXT NOT NULL UNIQUE,
			media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video'))
		)`,
		`CREATE TABLE IF NOT EXISTS index_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			status TEXT NOT NULL CHECK (status IN ('building', 'ready', 'failed')),
			started_at INTEGER NOT NULL,
			finished_at INTEGER,
			processed_dirs INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			media_root TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scan_queue (
			relative_dir TEXT PRIMARY KEY
		)`,
		`CREATE INDEX IF NOT EXISTS media_files_media_type_id ON media_files(media_type, id)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("create media index schema: %w", err)
		}
	}
	return tx.Commit()
}

func readState(ctx context.Context, db *sql.DB) (indexState, error) {
	var state indexState
	var startedAt int64
	var finishedAt sql.NullInt64
	var processedDirs int64
	var lastError sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT status, started_at, finished_at, processed_dirs, last_error, media_root
		FROM index_state WHERE id = 1`).Scan(
		&state.status, &startedAt, &finishedAt, &processedDirs, &lastError, &state.mediaRoot,
	)
	if err != nil {
		return indexState{}, err
	}
	state.startedAt = startedAt
	state.processedDirs = processedDirs
	return state, nil
}

func indexCounts(ctx context.Context, db *sql.DB) (images, videos, processedDirs, queuedDirs int64, startedAt int64, err error) {
	err = db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN media_type = 'video' THEN 1 ELSE 0 END), 0)
		FROM media_files`).Scan(&images, &videos)
	if err != nil {
		return
	}
	err = db.QueryRowContext(ctx, "SELECT processed_dirs, started_at FROM index_state WHERE id = 1").Scan(&processedDirs, &startedAt)
	if err != nil {
		return
	}
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM scan_queue").Scan(&queuedDirs)
	return
}

func initializeFreshIndex(ctx context.Context, db *sql.DB, mediaRoot string) error {
	var files, queue int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM media_files").Scan(&files); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM scan_queue").Scan(&queue); err != nil {
		return err
	}
	if files != 0 || queue != 0 {
		return errors.New("media index data exists without an index state row")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO index_state
		(id, status, started_at, finished_at, processed_dirs, last_error, media_root)
		VALUES (1, 'building', ?, NULL, 0, NULL, ?)`, timestampNow(), mediaRoot); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO scan_queue(relative_dir) VALUES (?)", "."); err != nil {
		return err
	}
	return tx.Commit()
}

func resetForRoot(ctx context.Context, db *sql.DB, mediaRoot string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_files"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM scan_queue"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE index_state SET
		status = 'building', started_at = ?, finished_at = NULL,
		processed_dirs = 0, last_error = NULL, media_root = ? WHERE id = 1`,
		timestampNow(), mediaRoot); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "INSERT INTO scan_queue(relative_dir) VALUES (?)", ".")
	if err != nil {
		return err
	}
	if _, err := result.RowsAffected(); err != nil {
		return err
	}
	return tx.Commit()
}

func queueHasEntries(ctx context.Context, db *sql.DB) (bool, error) {
	var marker int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM scan_queue LIMIT 1").Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func loadIDs(ctx context.Context, db *sql.DB) (map[string][]int64, error) {
	ids := map[string][]int64{"image": {}, "video": {}}
	for _, mediaType := range []string{"image", "video"} {
		rows, err := db.QueryContext(ctx, "SELECT id FROM media_files WHERE media_type = ? ORDER BY id", mediaType)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return nil, err
			}
			ids[mediaType] = append(ids[mediaType], id)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func timestampNow() int64 {
	return time.Now().UTC().Unix()
}
