package jobqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

// Store persists worker-facing lifecycle transitions for jobs.
type Store interface {
	Close() error
	ClaimNextQueuedJob(ctx context.Context, workerID string, now time.Time) (ClaimedJob, bool, error)
	UpdateJobProgress(ctx context.Context, jobID string, workerID string, stage string, progress float64, progressDescription *string, now time.Time) (bool, error)
	RefreshHeartbeat(ctx context.Context, jobID string, workerID string, now time.Time) (bool, error)
	FailExpiredProcessingJobs(ctx context.Context, cutoff time.Time, now time.Time) (int64, error)
	MarkJobSucceeded(ctx context.Context, jobID string, workerID string, now time.Time) (bool, error)
	MarkJobPendingUserDedup(ctx context.Context, jobID string, workerID string, now time.Time) (bool, error)
	MarkJobFailed(ctx context.Context, jobID string, workerID string, code string, message string, now time.Time) (bool, error)
}

type sqliteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens a SQLite-backed queue lifecycle store.
func NewSQLiteStore(databaseURL string) (Store, error) {
	normalizedURL := normalizeDatabaseURL(databaseURL)
	db, err := sql.Open("libsql", normalizedURL)
	if err != nil {
		return nil, fmt.Errorf("open libsql db: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if isLocalDatabaseURL(normalizedURL) {
		if err := applySQLiteBusyTimeout(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure sqlite busy timeout: %w", err)
		}
	}

	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func (s *sqliteStore) ClaimNextQueuedJob(ctx context.Context, workerID string, now time.Time) (ClaimedJob, bool, error) {
	if workerID == "" {
		return ClaimedJob{}, false, fmt.Errorf("workerID is required")
	}

	claimTime := normalizeNow(now)
	claimTimestamp := claimTime.Format(time.RFC3339Nano)

	for attempts := 0; attempts < 16; attempts++ {
		job, ok, err := s.claimOnce(ctx, workerID, claimTime, claimTimestamp)
		if err != nil {
			if isSQLiteBusy(err) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return ClaimedJob{}, false, err
		}
		if ok {
			return job, true, nil
		}

		return ClaimedJob{}, false, nil
	}

	return ClaimedJob{}, false, fmt.Errorf("claim queued job: sqlite busy after retries")
}

func (s *sqliteStore) claimOnce(
	ctx context.Context,
	workerID string,
	claimTime time.Time,
	claimTimestamp string,
) (ClaimedJob, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimedJob{}, false, fmt.Errorf("begin claim tx: %w", err)
	}

	var claimed ClaimedJob
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, upload_id, session_id
FROM jobs
WHERE status = ?
ORDER BY created_at ASC
LIMIT 1;`,
		JobStatusQueued,
	).Scan(&claimed.ID, &claimed.UploadID, &claimed.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			return ClaimedJob{}, false, nil
		}
		_ = tx.Rollback()
		return ClaimedJob{}, false, fmt.Errorf("select queued job: %w", err)
	}

	result, err := tx.ExecContext(
		ctx,
		`UPDATE jobs
SET status = ?, worker_id = ?, claimed_at = ?, heartbeat_at = ?, stage = ?, progress = ?, progress_description = NULL, updated_at = ?
WHERE id = ? AND status = ?;`,
		JobStatusProcessing,
		workerID,
		claimTimestamp,
		claimTimestamp,
		DefaultClaimStage,
		DefaultClaimInitialProgress,
		claimTimestamp,
		claimed.ID,
		JobStatusQueued,
	)
	if err != nil {
		_ = tx.Rollback()
		return ClaimedJob{}, false, fmt.Errorf("claim queued job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return ClaimedJob{}, false, fmt.Errorf("claim queued job rows affected: %w", err)
	}

	if rowsAffected == 0 {
		_ = tx.Rollback()
		return ClaimedJob{}, false, nil
	}

	if err := tx.Commit(); err != nil {
		return ClaimedJob{}, false, fmt.Errorf("commit claim tx: %w", err)
	}

	claimed.Status = JobStatusProcessing
	claimed.Progress = DefaultClaimInitialProgress
	claimed.Stage = DefaultClaimStage
	claimed.WorkerID = workerID
	claimed.ClaimedAt = claimTime

	return claimed, true, nil
}

func (s *sqliteStore) UpdateJobProgress(
	ctx context.Context,
	jobID string,
	workerID string,
	stage string,
	progress float64,
	progressDescription *string,
	now time.Time,
) (bool, error) {
	if jobID == "" {
		return false, fmt.Errorf("jobID is required")
	}
	if workerID == "" {
		return false, fmt.Errorf("workerID is required")
	}
	if stage == "" {
		return false, fmt.Errorf("stage is required")
	}
	if progress < 0 || progress > 100 {
		return false, fmt.Errorf("progress must be between 0 and 100")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET stage = ?, progress = ?, progress_description = ?, updated_at = ?
WHERE id = ? AND worker_id = ? AND status = ?;`,
		stage,
		progress,
		nullableString(progressDescription),
		timestamp,
		jobID,
		workerID,
		JobStatusProcessing,
	))
}

func (s *sqliteStore) RefreshHeartbeat(
	ctx context.Context,
	jobID string,
	workerID string,
	now time.Time,
) (bool, error) {
	if jobID == "" {
		return false, fmt.Errorf("jobID is required")
	}
	if workerID == "" {
		return false, fmt.Errorf("workerID is required")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET heartbeat_at = ?, updated_at = ?
WHERE id = ? AND worker_id = ? AND status = ?;`,
		timestamp,
		timestamp,
		jobID,
		workerID,
		JobStatusProcessing,
	))
}

func (s *sqliteStore) FailExpiredProcessingJobs(ctx context.Context, cutoff time.Time, now time.Time) (int64, error) {
	nowTimestamp := normalizeNow(now).Format(time.RFC3339Nano)
	cutoffTimestamp := cutoff.UTC().Format(time.RFC3339Nano)

	result, err := s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET status = ?, progress = 100, progress_description = NULL, error_code = ?, error_message = ?, finished_at = ?, updated_at = ?
WHERE status = ? AND heartbeat_at IS NOT NULL AND heartbeat_at < ?;`,
		JobStatusFailed,
		ErrorCodeWorkerTimeout,
		ErrorMessageWorkerTimeout,
		nowTimestamp,
		nowTimestamp,
		JobStatusProcessing,
		cutoffTimestamp,
	)
	if err != nil {
		return 0, fmt.Errorf("fail expired jobs: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("fail expired jobs rows affected: %w", err)
	}

	return rowsAffected, nil
}

func (s *sqliteStore) MarkJobSucceeded(ctx context.Context, jobID string, workerID string, now time.Time) (bool, error) {
	if jobID == "" {
		return false, fmt.Errorf("jobID is required")
	}
	if workerID == "" {
		return false, fmt.Errorf("workerID is required")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET status = ?, progress = 100, stage = NULL, progress_description = NULL, error_code = NULL, error_message = NULL, finished_at = ?, updated_at = ?
WHERE id = ? AND worker_id = ? AND status = ?;`,
		JobStatusSucceeded,
		timestamp,
		timestamp,
		jobID,
		workerID,
		JobStatusProcessing,
	))
}

func (s *sqliteStore) MarkJobPendingUserDedup(
	ctx context.Context,
	jobID string,
	workerID string,
	now time.Time,
) (bool, error) {
	if jobID == "" {
		return false, fmt.Errorf("jobID is required")
	}
	if workerID == "" {
		return false, fmt.Errorf("workerID is required")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET status = ?, progress = 100, stage = NULL, progress_description = NULL, error_code = NULL, error_message = NULL, finished_at = ?, updated_at = ?
WHERE id = ? AND worker_id = ? AND status = ?;`,
		JobStatusPendingUserDedup,
		timestamp,
		timestamp,
		jobID,
		workerID,
		JobStatusProcessing,
	))
}

func (s *sqliteStore) MarkJobFailed(
	ctx context.Context,
	jobID string,
	workerID string,
	code string,
	message string,
	now time.Time,
) (bool, error) {
	if jobID == "" {
		return false, fmt.Errorf("jobID is required")
	}
	if workerID == "" {
		return false, fmt.Errorf("workerID is required")
	}
	if code == "" {
		return false, fmt.Errorf("code is required")
	}
	if message == "" {
		return false, fmt.Errorf("message is required")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE jobs
SET status = ?, progress = 100, progress_description = NULL, error_code = ?, error_message = ?, finished_at = ?, updated_at = ?
WHERE id = ? AND worker_id = ? AND status = ?;`,
		JobStatusFailed,
		code,
		message,
		timestamp,
		timestamp,
		jobID,
		workerID,
		JobStatusProcessing,
	))
}

func rowsAffectedBool(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

func normalizeNow(now time.Time) time.Time {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return now
}

func isSQLiteBusy(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}

func isLocalDatabaseURL(databaseURL string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(databaseURL))
	if trimmed == "" {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "libsql://"),
		strings.HasPrefix(trimmed, "https://"),
		strings.HasPrefix(trimmed, "http://"),
		strings.HasPrefix(trimmed, "wss://"),
		strings.HasPrefix(trimmed, "ws://"):
		return false
	default:
		return true
	}
}

func normalizeDatabaseURL(databaseURL string) string {
	normalized := strings.TrimSpace(databaseURL)
	if normalized == "" || strings.HasPrefix(strings.ToLower(normalized), "file:") || strings.HasPrefix(normalized, ":memory:") {
		return normalized
	}
	if isLocalDatabaseURL(normalized) {
		return "file:" + normalized
	}
	return normalized
}

func applySQLiteBusyTimeout(db *sql.DB) error {
	rows, err := db.Query("PRAGMA busy_timeout = 5000;")
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.Err()
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}
