package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (m *Manager) runWorker(ctx context.Context, db *sql.DB, generation uint64) {
	for {
		if ctx.Err() != nil {
			return
		}
		relativeDir, found, err := nextDirectory(ctx, db)
		if err != nil {
			m.failScan(ctx, db, generation, fmt.Errorf("read scan queue: %w", err))
			return
		}
		if !found {
			if err := m.publishReady(ctx, db, generation); err != nil {
				m.failScan(ctx, db, generation, fmt.Errorf("finalize index: %w", err))
			}
			return
		}

		files, childDirs, err := m.scanner.ScanDirectory(ctx, relativeDir)
		if err != nil {
			if ctx.Err() == nil {
				m.failScan(ctx, db, generation, fmt.Errorf("scan directory %q: %w", relativeDir, err))
			}
			return
		}
		if ctx.Err() != nil {
			return
		}
		if err := validateScanResult(files, childDirs); err != nil {
			m.failScan(ctx, db, generation, fmt.Errorf("scan directory %q: %w", relativeDir, err))
			return
		}
		processed, err := commitDirectory(ctx, db, relativeDir, files, childDirs)
		if err != nil {
			if ctx.Err() == nil {
				m.failScan(ctx, db, generation, fmt.Errorf("store directory %q: %w", relativeDir, err))
			}
			return
		}
		m.logger.Debugf("media index: processed directory %q (processed=%d, files=%d, child_dirs=%d)", relativeDir, processed, len(files), len(childDirs))
	}
}

func nextDirectory(ctx context.Context, db *sql.DB) (string, bool, error) {
	var relativeDir string
	err := db.QueryRowContext(ctx, "SELECT relative_dir FROM scan_queue ORDER BY relative_dir LIMIT 1").Scan(&relativeDir)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return relativeDir, true, nil
}

func validateScanResult(files []MediaFile, childDirs []string) error {
	for _, file := range files {
		if strings.TrimSpace(file.RelativePath) == "" {
			return errors.New("scanner returned an empty relative path")
		}
		if file.MediaType != "image" && file.MediaType != "video" {
			return fmt.Errorf("scanner returned invalid media type %q", file.MediaType)
		}
	}
	for _, childDir := range childDirs {
		if strings.TrimSpace(childDir) == "" {
			return errors.New("scanner returned an empty child directory")
		}
	}
	return nil
}

func commitDirectory(ctx context.Context, db *sql.DB, relativeDir string, files []MediaFile, childDirs []string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, file := range files {
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_files(relative_path, media_type)
			VALUES (?, ?) ON CONFLICT(relative_path) DO UPDATE SET media_type = excluded.media_type`,
			file.RelativePath, file.MediaType); err != nil {
			return 0, err
		}
	}
	for _, childDir := range childDirs {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO scan_queue(relative_dir) VALUES (?)", childDir); err != nil {
			return 0, err
		}
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM scan_queue WHERE relative_dir = ?", relativeDir)
	if err != nil {
		return 0, err
	}
	removed, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if removed != 1 {
		return 0, fmt.Errorf("scan queue entry %q disappeared", relativeDir)
	}
	result, err = tx.ExecContext(ctx, `UPDATE index_state SET processed_dirs = processed_dirs + 1
		WHERE id = 1 AND status = 'building'`)
	if err != nil {
		return 0, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if updated != 1 {
		return 0, errors.New("index state is not building")
	}
	var processed int64
	if err := tx.QueryRowContext(ctx, "SELECT processed_dirs FROM index_state WHERE id = 1").Scan(&processed); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return processed, nil
}

func (m *Manager) publishReady(ctx context.Context, db *sql.DB, generation uint64) error {
	ids, err := loadIDs(ctx, db)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var processedDirs, startedAt int64
	if err := tx.QueryRowContext(ctx, "SELECT processed_dirs, started_at FROM index_state WHERE id = 1").Scan(&processedDirs, &startedAt); err != nil {
		return err
	}
	var pending int
	err = tx.QueryRowContext(ctx, "SELECT 1 FROM scan_queue LIMIT 1").Scan(&pending)
	if err == nil {
		return errors.New("scan queue gained work before index finalization")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE index_state SET status = 'ready', finished_at = ?, last_error = NULL
		WHERE id = 1 AND status = 'building'`, timestampNow())
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("index state is not building during finalization")
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	m.mu.Lock()
	published := m.generation == generation && !m.closed
	if published {
		m.ids = ids
		m.ready = true
		m.scanning = false
		m.workerCancel = nil
	}
	m.mu.Unlock()
	if published {
		images, videos := len(ids["image"]), len(ids["video"])
		elapsed := time.Since(time.Unix(startedAt, 0)).Round(time.Millisecond)
		m.logger.Infof("media index: success images=%d videos=%d total=%d processed_dirs=%d elapsed=%s mode=DB switched",
			images, videos, images+videos, processedDirs, elapsed)
	}
	return nil
}

func (m *Manager) failScan(ctx context.Context, db *sql.DB, generation uint64, cause error) {
	if ctx.Err() != nil {
		return
	}
	tx, txErr := db.BeginTx(context.Background(), nil)
	if txErr == nil {
		result, updateErr := tx.ExecContext(context.Background(), `UPDATE index_state SET status = 'failed',
			finished_at = ?, last_error = ? WHERE id = 1 AND status = 'building'`, timestampNow(), cause.Error())
		if updateErr == nil {
			updated, rowsErr := result.RowsAffected()
			updateErr = rowsErr
			if updateErr == nil && updated != 1 {
				updateErr = errors.New("index state was not building when recording failure")
			}
		}
		if updateErr == nil {
			updateErr = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if updateErr != nil {
			m.logger.Errorf("media index: persist failed state: %v", updateErr)
		}
	} else {
		m.logger.Errorf("media index: begin failed state transaction: %v", txErr)
	}
	images, videos, processedDirs, _, startedAt, summaryErr := indexCounts(context.Background(), db)
	if summaryErr != nil {
		m.logger.Errorf("media index: summarize failed build: %v", summaryErr)
		images, videos, processedDirs, startedAt = 0, 0, 0, timestampNow()
	}
	m.mu.Lock()
	failed := m.generation == generation
	if failed {
		m.ready = false
	}
	m.mu.Unlock()
	if failed {
		elapsed := time.Since(time.Unix(startedAt, 0)).Round(time.Millisecond)
		m.logger.Errorf("media index: failure reason=%q images=%d videos=%d total=%d processed_dirs=%d elapsed=%s mode=HDD retained",
			cause.Error(), images, videos, images+videos, processedDirs, elapsed)
	}
}
