package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClaimNextQueuedJobAllowsSingleWinnerUnderContention(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	baseNow := time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC)

	seedJob(t, store, seededJob{
		id:        "job-claim-1",
		uploadID:  "upload-claim-1",
		sessionID: "session-claim-1",
		status:    JobStatusQueued,
		progress:  0,
		createdAt: baseNow.Add(-5 * time.Second),
		updatedAt: baseNow.Add(-5 * time.Second),
	})

	type result struct {
		ok  bool
		job ClaimedJob
		err error
	}

	results := make([]result, 2)
	workerIDs := []string{"worker-a", "worker-b"}

	var wg sync.WaitGroup
	for i := 0; i < len(workerIDs); i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			job, ok, err := store.ClaimNextQueuedJob(ctx, workerIDs[idx], baseNow)
			results[idx] = result{ok: ok, job: job, err: err}
		}(i)
	}
	wg.Wait()

	successes := 0
	var winner result
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("expected claim without error, got: %v", r.err)
		}
		if r.ok {
			successes++
			winner = r
		}
	}

	if successes != 1 {
		t.Fatalf("expected exactly one successful claim, got %d", successes)
	}
	if winner.job.ID != "job-claim-1" {
		t.Fatalf("expected claimed job id job-claim-1, got %q", winner.job.ID)
	}

	const query = `
SELECT status, worker_id, claimed_at, heartbeat_at, stage, progress
FROM jobs
WHERE id = ?;`

	var status string
	var workerID sql.NullString
	var claimedAt sql.NullString
	var heartbeatAt sql.NullString
	var stage sql.NullString
	var progress int
	if err := store.db.QueryRowContext(ctx, query, "job-claim-1").Scan(
		&status,
		&workerID,
		&claimedAt,
		&heartbeatAt,
		&stage,
		&progress,
	); err != nil {
		t.Fatalf("expected claimed job row, got: %v", err)
	}

	if status != JobStatusProcessing {
		t.Fatalf("expected status %q, got %q", JobStatusProcessing, status)
	}
	if !workerID.Valid || workerID.String == "" {
		t.Fatal("expected worker_id to be populated")
	}
	if !claimedAt.Valid || !heartbeatAt.Valid {
		t.Fatal("expected claimed_at and heartbeat_at to be populated")
	}
	if !stage.Valid || stage.String != DefaultClaimStage {
		t.Fatalf("expected stage %q, got %#v", DefaultClaimStage, stage)
	}
	if progress != DefaultClaimInitialProgress {
		t.Fatalf("expected progress %d, got %d", DefaultClaimInitialProgress, progress)
	}
}

func TestProgressAndHeartbeatUpdatesRespectWorkerOwnership(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 12, 10, 0, 0, time.UTC)

	seedJob(t, store, seededJob{
		id:          "job-owner-1",
		uploadID:    "upload-owner-1",
		sessionID:   "session-owner-1",
		status:      JobStatusProcessing,
		stage:       ptr("SAMPLING_FRAMES"),
		progress:    35,
		workerID:    ptr("owner-worker"),
		claimedAt:   ptrTime(now.Add(-30 * time.Second)),
		heartbeatAt: ptrTime(now.Add(-2 * time.Second)),
		createdAt:   now.Add(-40 * time.Second),
		updatedAt:   now.Add(-2 * time.Second),
	})

	updated, err := store.UpdateJobProgress(ctx, "job-owner-1", "different-worker", "POSTPROCESSING", 90, now)
	if err != nil {
		t.Fatalf("expected ownership mismatch to be handled, got: %v", err)
	}
	if updated {
		t.Fatal("expected progress update to fail for non-owning worker")
	}

	refreshed, err := store.RefreshHeartbeat(ctx, "job-owner-1", "different-worker", now)
	if err != nil {
		t.Fatalf("expected ownership mismatch heartbeat to be handled, got: %v", err)
	}
	if refreshed {
		t.Fatal("expected heartbeat update to fail for non-owning worker")
	}

	const query = `SELECT stage, progress FROM jobs WHERE id = ?;`
	var stage sql.NullString
	var progress int
	if err := store.db.QueryRowContext(ctx, query, "job-owner-1").Scan(&stage, &progress); err != nil {
		t.Fatalf("expected owned job row, got: %v", err)
	}
	if !stage.Valid || stage.String != "SAMPLING_FRAMES" {
		t.Fatalf("expected stage to remain SAMPLING_FRAMES, got %#v", stage)
	}
	if progress != 35 {
		t.Fatalf("expected progress to remain 35, got %d", progress)
	}
}

func TestFailExpiredProcessingJobsMarksTimeoutFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 12, 20, 0, 0, time.UTC)
	cutoff := now.Add(-30 * time.Second)

	seedJob(t, store, seededJob{
		id:          "job-expired-1",
		uploadID:    "upload-expired-1",
		sessionID:   "session-expired-1",
		status:      JobStatusProcessing,
		stage:       ptr("DETECTING_APPRAISAL_SCREENS"),
		progress:    47,
		workerID:    ptr("worker-expired"),
		claimedAt:   ptrTime(now.Add(-2 * time.Minute)),
		heartbeatAt: ptrTime(now.Add(-90 * time.Second)),
		createdAt:   now.Add(-3 * time.Minute),
		updatedAt:   now.Add(-90 * time.Second),
	})
	seedJob(t, store, seededJob{
		id:          "job-fresh-1",
		uploadID:    "upload-fresh-1",
		sessionID:   "session-fresh-1",
		status:      JobStatusProcessing,
		stage:       ptr("SAMPLING_FRAMES"),
		progress:    40,
		workerID:    ptr("worker-fresh"),
		claimedAt:   ptrTime(now.Add(-45 * time.Second)),
		heartbeatAt: ptrTime(now.Add(-10 * time.Second)),
		createdAt:   now.Add(-1 * time.Minute),
		updatedAt:   now.Add(-10 * time.Second),
	})

	rows, err := store.FailExpiredProcessingJobs(ctx, cutoff, now)
	if err != nil {
		t.Fatalf("expected timeout sweep to succeed, got: %v", err)
	}
	if rows != 1 {
		t.Fatalf("expected 1 expired job to be failed, got %d", rows)
	}

	const expiredQuery = `
SELECT status, error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var status string
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var finishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, expiredQuery, "job-expired-1").Scan(
		&status,
		&errorCode,
		&errorMessage,
		&finishedAt,
	); err != nil {
		t.Fatalf("expected expired job row, got: %v", err)
	}
	if status != JobStatusFailed {
		t.Fatalf("expected expired job status %q, got %q", JobStatusFailed, status)
	}
	if !errorCode.Valid || errorCode.String != ErrorCodeWorkerTimeout {
		t.Fatalf("expected error_code %q, got %#v", ErrorCodeWorkerTimeout, errorCode)
	}
	if !errorMessage.Valid || errorMessage.String != ErrorMessageWorkerTimeout {
		t.Fatalf("expected error_message %q, got %#v", ErrorMessageWorkerTimeout, errorMessage)
	}
	if !finishedAt.Valid {
		t.Fatal("expected finished_at to be populated for expired job")
	}

	var freshStatus string
	if err := store.db.QueryRowContext(ctx, "SELECT status FROM jobs WHERE id = ?;", "job-fresh-1").Scan(&freshStatus); err != nil {
		t.Fatalf("expected fresh job row, got: %v", err)
	}
	if freshStatus != JobStatusProcessing {
		t.Fatalf("expected fresh job to remain %q, got %q", JobStatusProcessing, freshStatus)
	}
}

func TestMarkJobSucceededAndFailedPersistTerminalFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 12, 30, 0, 0, time.UTC)

	seedJob(t, store, seededJob{
		id:          "job-success-1",
		uploadID:    "upload-success-1",
		sessionID:   "session-success-1",
		status:      JobStatusProcessing,
		stage:       ptr("PERSISTING_RESULTS"),
		progress:    96,
		workerID:    ptr("worker-success"),
		claimedAt:   ptrTime(now.Add(-40 * time.Second)),
		heartbeatAt: ptrTime(now.Add(-1 * time.Second)),
		createdAt:   now.Add(-1 * time.Minute),
		updatedAt:   now.Add(-1 * time.Second),
	})
	seedJob(t, store, seededJob{
		id:          "job-fail-1",
		uploadID:    "upload-fail-1",
		sessionID:   "session-fail-1",
		status:      JobStatusProcessing,
		stage:       ptr("DECODING_VIDEO"),
		progress:    12,
		workerID:    ptr("worker-fail"),
		claimedAt:   ptrTime(now.Add(-50 * time.Second)),
		heartbeatAt: ptrTime(now.Add(-2 * time.Second)),
		createdAt:   now.Add(-70 * time.Second),
		updatedAt:   now.Add(-2 * time.Second),
	})

	ok, err := store.MarkJobSucceeded(ctx, "job-success-1", "different-worker", now)
	if err != nil {
		t.Fatalf("expected ownership mismatch to be handled, got: %v", err)
	}
	if ok {
		t.Fatal("expected mark succeeded to fail for non-owner")
	}

	ok, err = store.MarkJobSucceeded(ctx, "job-success-1", "worker-success", now)
	if err != nil {
		t.Fatalf("expected mark succeeded to work, got: %v", err)
	}
	if !ok {
		t.Fatal("expected mark succeeded to update the row")
	}

	const successQuery = `
SELECT status, progress, stage, error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var successStatus string
	var successProgress int
	var successStage sql.NullString
	var successErrorCode sql.NullString
	var successErrorMessage sql.NullString
	var successFinishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, successQuery, "job-success-1").Scan(
		&successStatus,
		&successProgress,
		&successStage,
		&successErrorCode,
		&successErrorMessage,
		&successFinishedAt,
	); err != nil {
		t.Fatalf("expected success row, got: %v", err)
	}
	if successStatus != JobStatusSucceeded {
		t.Fatalf("expected status %q, got %q", JobStatusSucceeded, successStatus)
	}
	if successProgress != 100 {
		t.Fatalf("expected progress 100, got %d", successProgress)
	}
	if successStage.Valid {
		t.Fatalf("expected stage to be NULL, got %q", successStage.String)
	}
	if successErrorCode.Valid || successErrorMessage.Valid {
		t.Fatalf("expected error fields to be NULL, got code=%#v message=%#v", successErrorCode, successErrorMessage)
	}
	if !successFinishedAt.Valid {
		t.Fatal("expected finished_at on succeeded row")
	}

	ok, err = store.MarkJobFailed(ctx, "job-fail-1", "worker-fail", "DECODE_FAILED", "Unable to decode video container", now)
	if err != nil {
		t.Fatalf("expected mark failed to work, got: %v", err)
	}
	if !ok {
		t.Fatal("expected mark failed to update the row")
	}

	const failedQuery = `
SELECT status, error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var failedStatus string
	var failedErrorCode sql.NullString
	var failedErrorMessage sql.NullString
	var failedFinishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, failedQuery, "job-fail-1").Scan(
		&failedStatus,
		&failedErrorCode,
		&failedErrorMessage,
		&failedFinishedAt,
	); err != nil {
		t.Fatalf("expected failed row, got: %v", err)
	}
	if failedStatus != JobStatusFailed {
		t.Fatalf("expected status %q, got %q", JobStatusFailed, failedStatus)
	}
	if !failedErrorCode.Valid || failedErrorCode.String != "DECODE_FAILED" {
		t.Fatalf("expected structured error_code, got %#v", failedErrorCode)
	}
	if !failedErrorMessage.Valid || failedErrorMessage.String != "Unable to decode video container" {
		t.Fatalf("expected structured error_message, got %#v", failedErrorMessage)
	}
	if !failedFinishedAt.Valid {
		t.Fatal("expected finished_at on failed row")
	}
}

func TestMarkJobPendingUserDedupPersistsTerminalFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 12, 40, 0, 0, time.UTC)

	seedJob(t, store, seededJob{
		id:          "job-pending-1",
		uploadID:    "upload-pending-1",
		sessionID:   "session-pending-1",
		status:      JobStatusProcessing,
		stage:       ptr("PERSISTING_RESULTS"),
		progress:    96,
		workerID:    ptr("worker-pending"),
		claimedAt:   ptrTime(now.Add(-40 * time.Second)),
		heartbeatAt: ptrTime(now.Add(-1 * time.Second)),
		createdAt:   now.Add(-1 * time.Minute),
		updatedAt:   now.Add(-1 * time.Second),
	})
	seedJob(t, store, seededJob{
		id:        "job-pending-queued",
		uploadID:  "upload-pending-queued",
		sessionID: "session-pending-queued",
		status:    JobStatusQueued,
		progress:  0,
		createdAt: now.Add(-2 * time.Minute),
		updatedAt: now.Add(-2 * time.Minute),
	})

	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE jobs SET error_code = ?, error_message = ? WHERE id = ?;`,
		"SOME_ERR",
		"some error",
		"job-pending-1",
	); err != nil {
		t.Fatalf("expected pre-seeded error fields update to succeed, got: %v", err)
	}

	ok, err := store.MarkJobPendingUserDedup(ctx, "job-pending-queued", "worker-pending", now)
	if err != nil {
		t.Fatalf("expected queued status mismatch to be handled, got: %v", err)
	}
	if ok {
		t.Fatal("expected pending-user-dedup transition to fail for non-processing row")
	}

	ok, err = store.MarkJobPendingUserDedup(ctx, "job-pending-1", "different-worker", now)
	if err != nil {
		t.Fatalf("expected ownership mismatch to be handled, got: %v", err)
	}
	if ok {
		t.Fatal("expected pending-user-dedup transition to fail for non-owner")
	}

	ok, err = store.MarkJobPendingUserDedup(ctx, "job-pending-1", "worker-pending", now)
	if err != nil {
		t.Fatalf("expected pending-user-dedup transition to work, got: %v", err)
	}
	if !ok {
		t.Fatal("expected pending-user-dedup transition to update the row")
	}

	const query = `
SELECT status, progress, stage, error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var status string
	var progress int
	var stage sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var finishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, query, "job-pending-1").Scan(
		&status,
		&progress,
		&stage,
		&errorCode,
		&errorMessage,
		&finishedAt,
	); err != nil {
		t.Fatalf("expected pending-user-dedup row, got: %v", err)
	}
	if status != JobStatusPendingUserDedup {
		t.Fatalf("expected status %q, got %q", JobStatusPendingUserDedup, status)
	}
	if progress != 100 {
		t.Fatalf("expected progress 100, got %d", progress)
	}
	if stage.Valid {
		t.Fatalf("expected stage to be NULL, got %q", stage.String)
	}
	if errorCode.Valid || errorMessage.Valid {
		t.Fatalf("expected error fields to be NULL, got code=%#v message=%#v", errorCode, errorMessage)
	}
	if !finishedAt.Valid {
		t.Fatal("expected finished_at on pending-user-dedup row")
	}
}

type seededJob struct {
	id          string
	uploadID    string
	sessionID   string
	status      string
	stage       *string
	progress    int
	workerID    *string
	claimedAt   *time.Time
	heartbeatAt *time.Time
	createdAt   time.Time
	updatedAt   time.Time
}

func newTestStore(t *testing.T) *sqliteStore {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "jobqueue.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store to initialize, got: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	sqliteStore, ok := store.(*sqliteStore)
	if !ok {
		t.Fatalf("expected *sqliteStore, got %T", store)
	}

	bootstrapTestSchema(t, sqliteStore.db)
	return sqliteStore
}

func bootstrapTestSchema(t *testing.T, db *sql.DB) {
	t.Helper()

	const schema = `
CREATE TABLE IF NOT EXISTS uploads (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	uploadthing_url TEXT NOT NULL,
	content_type TEXT NOT NULL,
	byte_size INTEGER NOT NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	parent_job_id TEXT NULL,
	status TEXT NOT NULL,
	progress INTEGER NOT NULL,
	stage TEXT NULL,
	worker_id TEXT NULL,
	claimed_at TEXT NULL,
	heartbeat_at TEXT NULL,
	error_code TEXT NULL,
	error_message TEXT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	finished_at TEXT NULL,
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE TABLE IF NOT EXISTS job_debug_jobs (
	job_id TEXT PRIMARY KEY,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	processing_started_at TEXT NOT NULL,
	downloading_finished_at TEXT NULL,
	decoding_finished_at TEXT NULL,
	sampling_finished_at TEXT NULL,
	extracting_finished_at TEXT NULL,
	postprocessing_finished_at TEXT NULL,
	persisting_finished_at TEXT NULL,
	processing_finished_at TEXT NULL,
	species_finished_at TEXT NULL,
	cp_finished_at TEXT NULL,
	hp_finished_at TEXT NULL,
	iv_finished_at TEXT NULL,
	download_meta_json TEXT NULL,
	decode_meta_json TEXT NULL,
	sampling_meta_json TEXT NULL,
	postprocessing_meta_json TEXT NULL,
	persisting_meta_json TEXT NULL,
	terminal_meta_json TEXT NULL,
	error_code TEXT NULL,
	error_message TEXT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY(job_id) REFERENCES jobs(id),
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE TABLE IF NOT EXISTS job_debug_frames (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	frame_index INTEGER NOT NULL,
	frame_timestamp_ms INTEGER NULL,
	frame_status TEXT NOT NULL,
	frame_started_at TEXT NULL,
	frame_finished_at TEXT NOT NULL,
	frame_duration_ms INTEGER NOT NULL,
	species_finished_at TEXT NULL,
	cp_finished_at TEXT NULL,
	hp_finished_at TEXT NULL,
	iv_finished_at TEXT NULL,
	layout_meta_json TEXT NULL,
	species_meta_json TEXT NULL,
	cp_meta_json TEXT NULL,
	hp_meta_json TEXT NULL,
	iv_meta_json TEXT NULL,
	iv_bar_measurement_meta_json TEXT NULL,
	frame_stability_meta_json TEXT NULL,
	selection_meta_json TEXT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY(job_id) REFERENCES jobs(id),
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE INDEX IF NOT EXISTS idx_job_debug_jobs_session_id ON job_debug_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_job_debug_jobs_created_at ON job_debug_jobs(created_at);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_frame_index ON job_debug_frames(job_id, frame_index);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_frame_ts ON job_debug_frames(job_id, frame_timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_created_at ON job_debug_frames(job_id, created_at);`

	if err := execSchemaStatements(context.Background(), db, schema); err != nil {
		t.Fatalf("expected schema bootstrap to succeed, got: %v", err)
	}
}

func execSchemaStatements(ctx context.Context, db *sql.DB, schema string) error {
	statements := strings.Split(schema, ";")
	for idx, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("statement %d: %w", idx+1, err)
		}
	}
	return nil
}

func seedJob(t *testing.T, store *sqliteStore, row seededJob) {
	t.Helper()

	insertUpload(t, store.db, row.uploadID, row.sessionID, row.createdAt)

	const insertJob = `
INSERT INTO jobs(
	id, upload_id, session_id, parent_job_id, status, progress, stage,
	worker_id, claimed_at, heartbeat_at, error_code, error_message,
	created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var claimedAt any
	if row.claimedAt != nil {
		claimedAt = row.claimedAt.UTC().Format(time.RFC3339Nano)
	}
	var heartbeatAt any
	if row.heartbeatAt != nil {
		heartbeatAt = row.heartbeatAt.UTC().Format(time.RFC3339Nano)
	}

	var stage any
	if row.stage != nil {
		stage = *row.stage
	}

	var workerID any
	if row.workerID != nil {
		workerID = *row.workerID
	}

	if _, err := store.db.ExecContext(
		context.Background(),
		insertJob,
		row.id,
		row.uploadID,
		row.sessionID,
		nil,
		row.status,
		row.progress,
		stage,
		workerID,
		claimedAt,
		heartbeatAt,
		nil,
		nil,
		row.createdAt.UTC().Format(time.RFC3339Nano),
		row.updatedAt.UTC().Format(time.RFC3339Nano),
		nil,
	); err != nil {
		t.Fatalf("expected seeded job insert to succeed, got: %v", err)
	}
}

func insertUpload(t *testing.T, db *sql.DB, uploadID string, sessionID string, now time.Time) {
	t.Helper()

	const insertUpload = `
INSERT INTO uploads(id, session_id, kind, uploadthing_url, content_type, byte_size, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.ExecContext(
		context.Background(),
		insertUpload,
		uploadID,
		sessionID,
		"image",
		"local://uploads/"+uploadID+".png",
		"image/png",
		2048,
		now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected seeded upload insert to succeed, got: %v", err)
	}
}

func ptr(value string) *string {
	return &value
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
