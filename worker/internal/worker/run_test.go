package worker

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/tursodatabase/go-libsql"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

func TestRunQueueTickClaimsQueuedJobAndPersistsTerminalLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "run-claim.db")
	db := newRunQueueTestDB(t, dbPath)

	seedRunQueueJob(t, db, runQueueSeededJob{
		id:        "job-run-claim-1",
		uploadID:  "upload-run-claim-1",
		sessionID: "session-run-claim-1",
		status:    jobqueue.JobStatusQueued,
		progress:  0,
		createdAt: time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC),
		updatedAt: time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC),
	})

	store, err := jobqueue.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected queue store to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	processor := &runQueueTestProcessor{
		stages: []runQueueStageUpdate{
			{stage: jobqueue.StageExtractingAppraisal, progress: 60},
			{stage: jobqueue.StagePersistingResults, progress: 95},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tickTime := time.Date(2026, time.March, 6, 12, 0, 30, 0, time.UTC)
	now := tickTime
	nowFn := func() time.Time {
		now = now.Add(1 * time.Second)
		return now
	}

	runQueueTick(
		context.Background(),
		store,
		logger,
		"worker-run-1",
		30*time.Second,
		200*time.Millisecond,
		processor,
		tickTime,
		nowFn,
	)

	if processor.calls != 1 {
		t.Fatalf("expected processor to run once, got %d calls", processor.calls)
	}

	const query = `
SELECT status, progress, stage, worker_id, claimed_at, heartbeat_at, error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var status string
	var progress int
	var stage sql.NullString
	var workerID sql.NullString
	var claimedAt sql.NullString
	var heartbeatAt sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var finishedAt sql.NullString
	if err := db.QueryRowContext(context.Background(), query, "job-run-claim-1").Scan(
		&status,
		&progress,
		&stage,
		&workerID,
		&claimedAt,
		&heartbeatAt,
		&errorCode,
		&errorMessage,
		&finishedAt,
	); err != nil {
		t.Fatalf("expected claimed row to be queryable: %v", err)
	}

	if status != jobqueue.JobStatusSucceeded {
		t.Fatalf("expected status %q, got %q", jobqueue.JobStatusSucceeded, status)
	}
	if progress != 100 {
		t.Fatalf("expected progress 100, got %d", progress)
	}
	if stage.Valid {
		t.Fatalf("expected terminal stage to be NULL, got %q", stage.String)
	}
	if !workerID.Valid || workerID.String == "" {
		t.Fatal("expected worker_id to be populated after claim")
	}
	if !claimedAt.Valid || !heartbeatAt.Valid {
		t.Fatalf("expected claimed_at and heartbeat_at to be populated, got claimed_at=%#v heartbeat_at=%#v", claimedAt, heartbeatAt)
	}
	if errorCode.Valid || errorMessage.Valid {
		t.Fatalf("expected terminal success to clear errors, got code=%#v message=%#v", errorCode, errorMessage)
	}
	if !finishedAt.Valid {
		t.Fatal("expected finished_at to be populated for terminal success")
	}
}

func TestRunQueueTickExpiresStaleProcessingJobs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "run-expire.db")
	db := newRunQueueTestDB(t, dbPath)
	now := time.Date(2026, time.March, 6, 12, 30, 0, 0, time.UTC)
	staleHeartbeat := now.Add(-2 * time.Minute)

	seedRunQueueJob(t, db, runQueueSeededJob{
		id:          "job-run-expired-1",
		uploadID:    "upload-run-expired-1",
		sessionID:   "session-run-expired-1",
		status:      jobqueue.JobStatusProcessing,
		stage:       ptrString(jobqueue.StageExtractingAppraisal),
		progress:    40,
		workerID:    ptrString("worker-expired"),
		claimedAt:   ptrTime(now.Add(-3 * time.Minute)),
		heartbeatAt: &staleHeartbeat,
		createdAt:   now.Add(-4 * time.Minute),
		updatedAt:   staleHeartbeat,
	})

	store, err := jobqueue.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected queue store to initialize: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runQueueTick(
		context.Background(),
		store,
		logger,
		"worker-run-expire",
		30*time.Second,
		200*time.Millisecond,
		nil,
		now,
		func() time.Time { return now },
	)

	const query = `SELECT status, error_code, error_message, finished_at FROM jobs WHERE id = ?;`
	var status string
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var finishedAt sql.NullString
	if err := db.QueryRowContext(context.Background(), query, "job-run-expired-1").Scan(
		&status,
		&errorCode,
		&errorMessage,
		&finishedAt,
	); err != nil {
		t.Fatalf("expected expired row to be queryable: %v", err)
	}

	if status != jobqueue.JobStatusFailed {
		t.Fatalf("expected status %q, got %q", jobqueue.JobStatusFailed, status)
	}
	if !errorCode.Valid || errorCode.String != jobqueue.ErrorCodeWorkerTimeout {
		t.Fatalf("expected error_code %q, got %#v", jobqueue.ErrorCodeWorkerTimeout, errorCode)
	}
	if !errorMessage.Valid || errorMessage.String != jobqueue.ErrorMessageWorkerTimeout {
		t.Fatalf("expected error_message %q, got %#v", jobqueue.ErrorMessageWorkerTimeout, errorMessage)
	}
	if !finishedAt.Valid {
		t.Fatal("expected finished_at to be populated for expired processing job")
	}
}

type runQueueStageUpdate struct {
	stage    string
	progress int
}

type runQueueTestProcessor struct {
	stages []runQueueStageUpdate
	err    error
	calls  int
}

func (p *runQueueTestProcessor) Process(
	_ context.Context,
	_ jobqueue.ClaimedJob,
	reportProgress ProgressReporter,
) error {
	p.calls++
	for _, update := range p.stages {
		if err := reportProgress(update.stage, update.progress); err != nil {
			return err
		}
	}
	return p.err
}

type runQueueSeededJob struct {
	id          string
	uploadID    string
	sessionID   string
	status      string
	stage       *string
	progress    int
	workerID    *string
	claimedAt   *time.Time
	heartbeatAt *time.Time
	errorCode   *string
	errorMsg    *string
	createdAt   time.Time
	updatedAt   time.Time
	finishedAt  *time.Time
}

func newRunQueueTestDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()

	db, err := sql.Open("libsql", "file:"+dbPath)
	if err != nil {
		t.Fatalf("expected sqlite db to open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	const schema = `
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
	finished_at TEXT NULL
);`

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		t.Fatalf("expected schema bootstrap to succeed: %v", err)
	}

	return db
}

func seedRunQueueJob(t *testing.T, db *sql.DB, job runQueueSeededJob) {
	t.Helper()

	const insert = `
INSERT INTO jobs (
	id, upload_id, session_id, parent_job_id, status, progress, stage, worker_id,
	claimed_at, heartbeat_at, error_code, error_message, created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.ExecContext(
		context.Background(),
		insert,
		job.id,
		job.uploadID,
		job.sessionID,
		nil,
		job.status,
		job.progress,
		job.stage,
		job.workerID,
		formatOptionalTime(job.claimedAt),
		formatOptionalTime(job.heartbeatAt),
		job.errorCode,
		job.errorMsg,
		job.createdAt.UTC().Format(time.RFC3339Nano),
		job.updatedAt.UTC().Format(time.RFC3339Nano),
		formatOptionalTime(job.finishedAt),
	); err != nil {
		t.Fatalf("expected seed job insert to succeed: %v", err)
	}
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func ptrString(value string) *string {
	return &value
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
