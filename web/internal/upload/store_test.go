package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

func TestSQLiteStoreBootstrapsUploadAndJobSchema(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	assertSQLiteObjectExists(t, store.db, "table", "uploads")
	assertSQLiteObjectExists(t, store.db, "table", "jobs")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_candidates")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_results")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_result_dedupe_tombstones")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_result_pvp_evaluations")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_result_pvp_eval_queue")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_pending_readings")
	assertSQLiteObjectExists(t, store.db, "table", "appraisal_pending_species_options")
	assertSQLiteObjectExists(t, store.db, "table", "job_debug_jobs")
	assertSQLiteObjectExists(t, store.db, "table", "job_debug_frames")
	assertSQLiteObjectExists(t, store.db, "index", "idx_jobs_status_created_at")
	assertSQLiteObjectExists(t, store.db, "index", "idx_jobs_status_heartbeat_at")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_candidates_job_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_candidates_upload_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_candidates_session_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_results_job_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_results_upload_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_results_session_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_dedupe_tombstones_session_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_dedupe_tombstones_source_result_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_pvp_evals_result_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_pvp_evals_species_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_pvp_eval_queue_status_next_retry")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_result_pvp_eval_queue_result_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_pending_readings_job_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_pending_readings_session_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_appraisal_pending_species_options_pending_reading_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_job_debug_jobs_session_id")
	assertSQLiteObjectExists(t, store.db, "index", "idx_job_debug_jobs_created_at")
	assertSQLiteObjectExists(t, store.db, "index", "idx_job_debug_frames_job_id_frame_index")
	assertSQLiteObjectExists(t, store.db, "index", "idx_job_debug_frames_job_id_frame_ts")
	assertSQLiteObjectExists(t, store.db, "index", "idx_job_debug_frames_job_id_created_at")
	assertSQLiteColumnExists(t, store.db, "appraisal_results", "deleted_at")
}

func TestSQLiteStoreBootstrapAddsDeletedAtColumnToExistingAppraisalResultsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-upload.db")
	db, err := sql.Open("libsql", "file:"+dbPath)
	if err != nil {
		t.Fatalf("expected legacy db open to succeed, got %v", err)
	}

	const legacySchema = `
CREATE TABLE appraisal_results (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	species_name TEXT NOT NULL,
	cp INTEGER NOT NULL,
	hp INTEGER NOT NULL,
	power_up_stardust_cost INTEGER NOT NULL,
	iv_attack INTEGER NOT NULL,
	iv_defense INTEGER NOT NULL,
	iv_stamina INTEGER NOT NULL,
	level_estimate REAL NULL,
	level_confidence REAL NULL,
	level_method TEXT NOT NULL,
	source_type TEXT NOT NULL,
	start_ms INTEGER NULL,
	end_ms INTEGER NULL,
	frame_timestamp_ms INTEGER NULL,
	extraction_confidence REAL NULL,
	created_at TEXT NOT NULL
);`
	if _, err := db.ExecContext(context.Background(), legacySchema); err != nil {
		_ = db.Close()
		t.Fatalf("expected legacy schema create to succeed, got %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("expected legacy db close to succeed, got %v", err)
	}

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store to initialize from legacy schema, got %v", err)
	}

	sqliteStore, ok := store.(*sqliteStore)
	if !ok {
		t.Fatalf("expected *sqliteStore, got %T", store)
	}
	defer sqliteStore.db.Close()

	assertSQLiteColumnExists(t, sqliteStore.db, "appraisal_results", "deleted_at")
	assertSQLiteObjectExists(t, sqliteStore.db, "table", "appraisal_result_dedupe_tombstones")
	assertSQLiteObjectExists(t, sqliteStore.db, "index", "idx_appraisal_result_dedupe_tombstones_session_id")
}

func TestCreateUploadAndQueuedJobCreatesBothRowsWithQueuedDefaults(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 1, 20, 15, 0, 0, time.UTC)

	createdUpload, createdJob, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		OwnerKey:    "12f9f169-d9ca-4ea3-91e0-18356a1e1477",
		Kind:        KindImage,
		MediaURL:    "local://uploads/example.png",
		ContentType: "image/png",
		ByteSize:    2048,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected create upload+job to succeed, got: %v", err)
	}

	if createdUpload.ID == "" {
		t.Fatal("expected upload ID")
	}
	if createdJob.ID == "" {
		t.Fatal("expected job ID")
	}
	if createdJob.UploadID != createdUpload.ID {
		t.Fatalf("expected job upload id %q, got %q", createdUpload.ID, createdJob.UploadID)
	}
	if createdJob.Status != JobStatusQueued {
		t.Fatalf("expected status %q, got %q", JobStatusQueued, createdJob.Status)
	}
	if createdJob.Progress != 0 {
		t.Fatalf("expected progress 0, got %d", createdJob.Progress)
	}

	assertRowCount(t, store.db, "uploads", "id", createdUpload.ID, 1)
	assertRowCount(t, store.db, "jobs", "id", createdJob.ID, 1)

	const query = `
SELECT parent_job_id, status, progress, stage, worker_id, claimed_at, heartbeat_at,
       error_code, error_message, finished_at
FROM jobs
WHERE id = ?;`

	var parentJobID sql.NullString
	var status string
	var progress int
	var stage sql.NullString
	var workerID sql.NullString
	var claimedAt sql.NullString
	var heartbeatAt sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var finishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, query, createdJob.ID).Scan(
		&parentJobID,
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
		t.Fatalf("expected queued job row, got: %v", err)
	}

	if parentJobID.Valid {
		t.Fatalf("expected parent_job_id to be NULL, got %q", parentJobID.String)
	}
	if status != JobStatusQueued {
		t.Fatalf("expected status %q, got %q", JobStatusQueued, status)
	}
	if progress != 0 {
		t.Fatalf("expected progress 0, got %d", progress)
	}
	for _, field := range []struct {
		name  string
		value sql.NullString
	}{
		{name: "stage", value: stage},
		{name: "worker_id", value: workerID},
		{name: "claimed_at", value: claimedAt},
		{name: "heartbeat_at", value: heartbeatAt},
		{name: "error_code", value: errorCode},
		{name: "error_message", value: errorMessage},
		{name: "finished_at", value: finishedAt},
	} {
		if field.value.Valid {
			t.Fatalf("expected %s to be NULL, got %q", field.name, field.value.String)
		}
	}
}

func TestCreateUploadAndQueuedJobRollsBackWhenJobInsertFails(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 1, 20, 20, 0, 0, time.UTC)

	const duplicateJobID = "b8f24543-9f6e-493f-8f50-035519f64e90"

	_, _, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		UploadID:    "9ec17f15-9807-44a5-a6de-183f0bce7136",
		JobID:       duplicateJobID,
		OwnerKey:    "12f9f169-d9ca-4ea3-91e0-18356a1e1477",
		Kind:        KindVideo,
		MediaURL:    "local://uploads/a.mp4",
		ContentType: "video/mp4",
		ByteSize:    1024,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected initial create to succeed, got: %v", err)
	}

	secondUploadID := "ed03a0df-38ce-4d0a-a0b9-5e95fac63f72"
	_, _, err = store.CreateUploadAndQueuedJob(ctx, CreateParams{
		UploadID:    secondUploadID,
		JobID:       duplicateJobID,
		OwnerKey:    "12f9f169-d9ca-4ea3-91e0-18356a1e1477",
		Kind:        KindVideo,
		MediaURL:    "local://uploads/b.mp4",
		ContentType: "video/mp4",
		ByteSize:    2048,
		Now:         now.Add(time.Second),
	})
	if err == nil {
		t.Fatal("expected duplicate job id error")
	}

	assertRowCount(t, store.db, "jobs", "id", duplicateJobID, 1)
	assertRowCount(t, store.db, "uploads", "id", secondUploadID, 0)
}

func TestCreateRetryJobCreatesQueuedChildWithParentLinkage(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)

	createdUpload, parentJob, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		UploadID:    "upload-parent",
		JobID:       "job-parent",
		OwnerKey:    "session-a",
		Kind:        KindImage,
		MediaURL:    "local://uploads/parent.png",
		ContentType: "image/png",
		ByteSize:    1024,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected parent job create to succeed, got: %v", err)
	}
	failedStage := "POSTPROCESSING"
	failedErrorCode := "NO_APPRAISALS_FOUND"
	failedErrorMessage := "No readable appraisals detected"
	parentFinishedAt := now.Add(20 * time.Second)
	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE jobs
		 SET status = ?, progress = ?, stage = ?, error_code = ?, error_message = ?, finished_at = ?, updated_at = ?
		 WHERE id = ?;`,
		JobStatusFailed,
		100,
		failedStage,
		failedErrorCode,
		failedErrorMessage,
		parentFinishedAt.UTC().Format(time.RFC3339Nano),
		parentFinishedAt.UTC().Format(time.RFC3339Nano),
		parentJob.ID,
	); err != nil {
		t.Fatalf("expected parent job update to failed to succeed, got: %v", err)
	}

	retryNow := now.Add(30 * time.Second)
	retryJob, err := store.CreateRetryJob(ctx, parentJob.ID, parentJob.SessionID, retryNow)
	if err != nil {
		t.Fatalf("expected retry job create to succeed, got: %v", err)
	}

	if retryJob.ID == "" {
		t.Fatal("expected retry job ID")
	}
	if retryJob.ID == parentJob.ID {
		t.Fatalf("expected retry job id to differ from parent id %q", parentJob.ID)
	}
	if retryJob.ParentJobID != parentJob.ID {
		t.Fatalf("expected parent id %q, got %q", parentJob.ID, retryJob.ParentJobID)
	}
	if retryJob.UploadID != createdUpload.ID {
		t.Fatalf("expected upload id %q, got %q", createdUpload.ID, retryJob.UploadID)
	}
	if retryJob.SessionID != parentJob.SessionID {
		t.Fatalf("expected session id %q, got %q", parentJob.SessionID, retryJob.SessionID)
	}
	if retryJob.Status != JobStatusQueued {
		t.Fatalf("expected status %q, got %q", JobStatusQueued, retryJob.Status)
	}
	if !retryJob.CreatedAt.Equal(retryNow) {
		t.Fatalf("expected createdAt %s, got %s", retryNow, retryJob.CreatedAt)
	}
	if !retryJob.UpdatedAt.Equal(retryNow) {
		t.Fatalf("expected updatedAt %s, got %s", retryNow, retryJob.UpdatedAt)
	}

	assertRowCount(t, store.db, "jobs", "id", parentJob.ID, 1)
	assertRowCount(t, store.db, "jobs", "id", retryJob.ID, 1)

	const query = `
SELECT upload_id, session_id, parent_job_id, status, progress, stage, worker_id, claimed_at,
       heartbeat_at, error_code, error_message, created_at, updated_at, finished_at
FROM jobs
WHERE id = ?;`

	var uploadID string
	var sessionID string
	var parentJobID sql.NullString
	var status string
	var progress int
	var stage sql.NullString
	var workerID sql.NullString
	var claimedAt sql.NullString
	var heartbeatAt sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString
	var createdAtRaw string
	var updatedAtRaw string
	var finishedAt sql.NullString
	if err := store.db.QueryRowContext(ctx, query, retryJob.ID).Scan(
		&uploadID,
		&sessionID,
		&parentJobID,
		&status,
		&progress,
		&stage,
		&workerID,
		&claimedAt,
		&heartbeatAt,
		&errorCode,
		&errorMessage,
		&createdAtRaw,
		&updatedAtRaw,
		&finishedAt,
	); err != nil {
		t.Fatalf("expected retry job row, got: %v", err)
	}

	if uploadID != createdUpload.ID {
		t.Fatalf("expected upload_id %q, got %q", createdUpload.ID, uploadID)
	}
	if sessionID != parentJob.SessionID {
		t.Fatalf("expected session_id %q, got %q", parentJob.SessionID, sessionID)
	}
	if !parentJobID.Valid || parentJobID.String != parentJob.ID {
		t.Fatalf("expected parent_job_id %q, got %#v", parentJob.ID, parentJobID)
	}
	if status != JobStatusQueued {
		t.Fatalf("expected status %q, got %q", JobStatusQueued, status)
	}
	if progress != 0 {
		t.Fatalf("expected progress 0, got %d", progress)
	}
	for _, field := range []struct {
		name  string
		value sql.NullString
	}{
		{name: "stage", value: stage},
		{name: "worker_id", value: workerID},
		{name: "claimed_at", value: claimedAt},
		{name: "heartbeat_at", value: heartbeatAt},
		{name: "error_code", value: errorCode},
		{name: "error_message", value: errorMessage},
		{name: "finished_at", value: finishedAt},
	} {
		if field.value.Valid {
			t.Fatalf("expected %s to be NULL, got %q", field.name, field.value.String)
		}
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		t.Fatalf("expected created_at parse to succeed, got: %v", err)
	}
	if !createdAt.Equal(retryNow) {
		t.Fatalf("expected created_at %s, got %s", retryNow, createdAt)
	}

	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		t.Fatalf("expected updated_at parse to succeed, got: %v", err)
	}
	if !updatedAt.Equal(retryNow) {
		t.Fatalf("expected updated_at %s, got %s", retryNow, updatedAt)
	}
}

func TestCreateRetryJobRetriesOwnedFailedParent(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	sessionID := "session-a"
	createdAt := time.Date(2026, time.March, 2, 9, 15, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	stage := "POSTPROCESSING"
	errorCode := "NO_APPRAISALS_FOUND"
	errorMessage := "No readable appraisals detected"

	seedUploadRow(t, store.db, "upload-parent", sessionID, createdAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:           "job-parent",
		UploadID:     "upload-parent",
		SessionID:    sessionID,
		Status:       JobStatusFailed,
		Progress:     100,
		Stage:        &stage,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		FinishedAt:   &updatedAt,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})

	retryJob, err := store.CreateRetryJob(ctx, "job-parent", sessionID, updatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected retry create to succeed for failed parent, got: %v", err)
	}

	if retryJob.ParentJobID != "job-parent" {
		t.Fatalf("expected parent id %q, got %q", "job-parent", retryJob.ParentJobID)
	}
	if retryJob.Status != JobStatusQueued {
		t.Fatalf("expected status %q, got %q", JobStatusQueued, retryJob.Status)
	}
}

func TestCreateRetryJobReturnsNotAllowedForOwnedNonFailedParent(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 9, 25, 0, 0, time.UTC)

	_, _, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		UploadID:    "upload-parent",
		JobID:       "job-parent",
		OwnerKey:    "session-a",
		Kind:        KindImage,
		MediaURL:    "local://uploads/parent.png",
		ContentType: "image/png",
		ByteSize:    1024,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected parent job create to succeed, got: %v", err)
	}

	_, err = store.CreateRetryJob(ctx, "job-parent", "session-a", now.Add(time.Minute))
	if !errors.Is(err, ErrJobRetryNotAllowed) {
		t.Fatalf("expected ErrJobRetryNotAllowed for non-failed parent, got %v", err)
	}
}

func TestCreateRetryJobReturnsNotFoundForSessionMismatch(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 9, 30, 0, 0, time.UTC)

	_, _, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		UploadID:    "upload-parent",
		JobID:       "job-parent",
		OwnerKey:    "session-a",
		Kind:        KindImage,
		MediaURL:    "local://uploads/parent.png",
		ContentType: "image/png",
		ByteSize:    1024,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected parent job create to succeed, got: %v", err)
	}

	_, err = store.CreateRetryJob(ctx, "job-parent", "session-b", now.Add(time.Minute))
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for cross-session retry, got %v", err)
	}
}

func TestCreateRetryJobReturnsNotFoundForUnknownParent(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	_, err := store.CreateRetryJob(context.Background(), "missing-job", "session-a", time.Now().UTC())
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for unknown parent, got %v", err)
	}
}

func TestCreateRetryJobKeepsParentRowImmutable(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	sessionID := "session-a"
	createdAt := time.Date(2026, time.March, 2, 9, 45, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	stage := "POSTPROCESSING"
	errorCode := "NO_APPRAISALS_FOUND"
	errorMessage := "No readable appraisals detected"

	seedUploadRow(t, store.db, "upload-parent", sessionID, createdAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:           "job-parent",
		UploadID:     "upload-parent",
		SessionID:    sessionID,
		Status:       JobStatusFailed,
		Progress:     100,
		Stage:        &stage,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		FinishedAt:   &updatedAt,
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})

	before := readJobSnapshot(t, store.db, "job-parent")

	if _, err := store.CreateRetryJob(ctx, "job-parent", sessionID, updatedAt.Add(time.Minute)); err != nil {
		t.Fatalf("expected retry create to succeed, got: %v", err)
	}

	after := readJobSnapshot(t, store.db, "job-parent")
	if before != after {
		t.Fatalf("expected parent job row to remain unchanged, before=%#v after=%#v", before, after)
	}
}

func TestGetJobStatusReturnsFullLifecycleData(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	sessionID := "12f9f169-d9ca-4ea3-91e0-18356a1e1477"
	createdAt := time.Date(2026, time.March, 2, 10, 0, 0, 123_456_000, time.UTC)
	updatedAt := createdAt.Add(30 * time.Second)
	finishedAt := createdAt.Add(5 * time.Minute)

	processingStage := "OCR"
	failedStage := "OCR"
	failedCode := "OCR_TIMEOUT"
	failedMessage := "OCR timed out"

	cases := []struct {
		name         string
		status       string
		progress     int
		stage        *string
		finishedAt   *time.Time
		errorCode    *string
		errorMessage *string
	}{
		{
			name:       "queued",
			status:     JobStatusQueued,
			progress:   0,
			stage:      nil,
			finishedAt: nil,
		},
		{
			name:       "processing",
			status:     JobStatusProcessing,
			progress:   45,
			stage:      &processingStage,
			finishedAt: nil,
		},
		{
			name:       "succeeded",
			status:     JobStatusSucceeded,
			progress:   100,
			stage:      nil,
			finishedAt: &finishedAt,
		},
		{
			name:         "failed",
			status:       JobStatusFailed,
			progress:     100,
			stage:        &failedStage,
			finishedAt:   &finishedAt,
			errorCode:    &failedCode,
			errorMessage: &failedMessage,
		},
		{
			name:       "pending_user_dedup",
			status:     JobStatusPendingUserDedup,
			progress:   100,
			stage:      nil,
			finishedAt: &finishedAt,
		},
	}

	for idx, tc := range cases {
		uploadID := fmt.Sprintf("upload-%d", idx)
		jobID := fmt.Sprintf("job-%d", idx)
		seedUploadRow(t, store.db, uploadID, sessionID, createdAt)
		seedJobRow(t, store.db, seededJobRow{
			ID:           jobID,
			UploadID:     uploadID,
			SessionID:    sessionID,
			Status:       tc.status,
			Progress:     tc.progress,
			Stage:        tc.stage,
			CreatedAt:    createdAt,
			UpdatedAt:    updatedAt,
			FinishedAt:   tc.finishedAt,
			ErrorCode:    tc.errorCode,
			ErrorMessage: tc.errorMessage,
		})

		record, err := store.GetJobStatus(ctx, jobID, sessionID)
		if err != nil {
			t.Fatalf("%s: expected no error, got %v", tc.name, err)
		}

		if record.ID != jobID {
			t.Fatalf("%s: expected id %q, got %q", tc.name, jobID, record.ID)
		}
		if record.UploadID != uploadID {
			t.Fatalf("%s: expected upload id %q, got %q", tc.name, uploadID, record.UploadID)
		}
		if record.SessionID != sessionID {
			t.Fatalf("%s: expected session id %q, got %q", tc.name, sessionID, record.SessionID)
		}
		if record.Status != tc.status {
			t.Fatalf("%s: expected status %q, got %q", tc.name, tc.status, record.Status)
		}
		if record.Progress != tc.progress {
			t.Fatalf("%s: expected progress %d, got %d", tc.name, tc.progress, record.Progress)
		}
		assertOptionalStringEqual(t, tc.name, "stage", tc.stage, record.Stage)
		if !record.CreatedAt.Equal(createdAt) {
			t.Fatalf("%s: expected createdAt %s, got %s", tc.name, createdAt, record.CreatedAt)
		}
		if !record.UpdatedAt.Equal(updatedAt) {
			t.Fatalf("%s: expected updatedAt %s, got %s", tc.name, updatedAt, record.UpdatedAt)
		}
		assertOptionalTimeEqual(t, tc.name, "finishedAt", tc.finishedAt, record.FinishedAt)
		assertOptionalStringEqual(t, tc.name, "errorCode", tc.errorCode, record.ErrorCode)
		assertOptionalStringEqual(t, tc.name, "errorMessage", tc.errorMessage, record.ErrorMessage)
	}
}

func TestGetJobStatusReturnsNotFoundForSessionMismatch(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	createdAt := time.Date(2026, time.March, 2, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-1", sessionID, createdAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-1",
		UploadID:  "upload-1",
		SessionID: sessionID,
		Status:    JobStatusQueued,
		Progress:  0,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	})

	_, err := store.GetJobStatus(ctx, "job-1", "session-b")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for cross-session access, got %v", err)
	}
}

func TestGetJobStatusReturnsNotFoundForUnknownJob(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	_, err := store.GetJobStatus(context.Background(), "missing-job", "session-a")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected ErrJobNotFound for unknown job, got %v", err)
	}
}

func TestListPokemonResultsReturnsSessionScopedDeterministicRowsAndNullables(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	sessionA := "session-a"
	sessionB := "session-b"
	baseCreatedAt := time.Date(2026, time.March, 5, 10, 0, 0, 0, time.UTC)

	levelEstimate := 23.5
	levelConfidence := 0.72
	startMS := int64(12000)
	endMS := int64(15500)
	frameTimestampMS := int64(13200)
	extractionConfidence := 0.86

	seedUploadRow(t, store.db, "upload-a2", sessionA, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a2",
		UploadID:  "upload-a2",
		SessionID: sessionA,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-2",
		JobID:               "job-a2",
		UploadID:            "upload-a2",
		SessionID:           sessionA,
		SpeciesName:         "Pikachu",
		CP:                  410,
		HP:                  64,
		PowerUpStardustCost: 3000,
		IVAttack:            10,
		IVDefense:           12,
		IVStamina:           11,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           baseCreatedAt,
	})

	seedUploadRow(t, store.db, "upload-b1", sessionB, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-b1",
		UploadID:  "upload-b1",
		SessionID: sessionB,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-z",
		JobID:               "job-b1",
		UploadID:            "upload-b1",
		SessionID:           sessionB,
		SpeciesName:         "Charmander",
		CP:                  500,
		HP:                  70,
		PowerUpStardustCost: 3000,
		IVAttack:            11,
		IVDefense:           12,
		IVStamina:           13,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           baseCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "pvp-eval-b1",
		AppraisalResultID:  "result-z",
		MaxCP:              1500,
		EvaluatedSpeciesID: "charmeleon",
		BestLevel:          20.0,
		BestCP:             1495,
		StatProduct:        1234567.89,
		RankPosition:       210,
		Percentage:         90.01,
		CreatedAt:          baseCreatedAt,
	})

	seedUploadRow(t, store.db, "upload-a1", sessionA, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a1",
		UploadID:  "upload-a1",
		SessionID: sessionA,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                   "result-1",
		JobID:                "job-a1",
		UploadID:             "upload-a1",
		SessionID:            sessionA,
		SpeciesName:          "Machop",
		CP:                   512,
		HP:                   64,
		PowerUpStardustCost:  2500,
		IVAttack:             12,
		IVDefense:            15,
		IVStamina:            13,
		LevelEstimate:        &levelEstimate,
		LevelConfidence:      &levelConfidence,
		LevelMethod:          "ARC_POSITION",
		SourceType:           "VIDEO",
		StartMS:              &startMS,
		EndMS:                &endMS,
		FrameTimestampMS:     &frameTimestampMS,
		ExtractionConfidence: &extractionConfidence,
		CreatedAt:            baseCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "pvp-eval-a1",
		AppraisalResultID:  "result-1",
		MaxCP:              1500,
		EvaluatedSpeciesID: "machoke",
		BestLevel:          23.5,
		BestCP:             1498,
		StatProduct:        1567890.12,
		RankPosition:       143,
		Percentage:         93.32,
		CreatedAt:          baseCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "pvp-eval-a2",
		AppraisalResultID:  "result-1",
		MaxCP:              2500,
		EvaluatedSpeciesID: "machamp",
		BestLevel:          39.0,
		BestCP:             2499,
		StatProduct:        2789012.34,
		RankPosition:       98,
		Percentage:         96.11,
		CreatedAt:          baseCreatedAt,
	})

	laterCreatedAt := baseCreatedAt.Add(time.Second)
	seedUploadRow(t, store.db, "upload-a3", sessionA, laterCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a3",
		UploadID:  "upload-a3",
		SessionID: sessionA,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: laterCreatedAt,
		UpdatedAt: laterCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-3",
		JobID:               "job-a3",
		UploadID:            "upload-a3",
		SessionID:           sessionA,
		SpeciesName:         "Bulbasaur",
		CP:                  300,
		HP:                  50,
		PowerUpStardustCost: 2500,
		IVAttack:            9,
		IVDefense:           8,
		IVStamina:           10,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           laterCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "pvp-eval-a3",
		AppraisalResultID:  "result-3",
		MaxCP:              500,
		EvaluatedSpeciesID: "ivysaur",
		BestLevel:          10.0,
		BestCP:             499,
		StatProduct:        543210.0,
		RankPosition:       12,
		Percentage:         99.01,
		CreatedAt:          laterCreatedAt,
	})

	results, err := store.ListPokemonResults(ctx, sessionA)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results for session %q, got %d", sessionA, len(results))
	}

	expected := []PokemonResultRecord{
		{
			ID:                   "result-1",
			JobID:                "job-a1",
			UploadID:             "upload-a1",
			SessionID:            sessionA,
			SpeciesName:          "Machop",
			CP:                   512,
			HP:                   64,
			PowerUpStardustCost:  2500,
			IVAttack:             12,
			IVDefense:            15,
			IVStamina:            13,
			LevelEstimate:        &levelEstimate,
			LevelConfidence:      &levelConfidence,
			LevelMethod:          "ARC_POSITION",
			SourceType:           "VIDEO",
			StartMS:              &startMS,
			EndMS:                &endMS,
			FrameTimestampMS:     &frameTimestampMS,
			ExtractionConfidence: &extractionConfidence,
			MaxCPEvaluations: []PokemonResultMaxCPEvaluationRecord{
				{
					MaxCP:              1500,
					EvaluatedSpeciesID: "machoke",
					BestLevel:          23.5,
					BestCP:             1498,
					StatProduct:        1567890.12,
					Rank:               143,
					Percentage:         93.32,
				},
				{
					MaxCP:              2500,
					EvaluatedSpeciesID: "machamp",
					BestLevel:          39.0,
					BestCP:             2499,
					StatProduct:        2789012.34,
					Rank:               98,
					Percentage:         96.11,
				},
			},
			CreatedAt: baseCreatedAt,
		},
		{
			ID:                   "result-2",
			JobID:                "job-a2",
			UploadID:             "upload-a2",
			SessionID:            sessionA,
			SpeciesName:          "Pikachu",
			CP:                   410,
			HP:                   64,
			PowerUpStardustCost:  3000,
			IVAttack:             10,
			IVDefense:            12,
			IVStamina:            11,
			LevelEstimate:        nil,
			LevelConfidence:      nil,
			LevelMethod:          "UNKNOWN",
			SourceType:           "IMAGE",
			StartMS:              nil,
			EndMS:                nil,
			FrameTimestampMS:     nil,
			ExtractionConfidence: nil,
			MaxCPEvaluations:     nil,
			CreatedAt:            baseCreatedAt,
		},
		{
			ID:                   "result-3",
			JobID:                "job-a3",
			UploadID:             "upload-a3",
			SessionID:            sessionA,
			SpeciesName:          "Bulbasaur",
			CP:                   300,
			HP:                   50,
			PowerUpStardustCost:  2500,
			IVAttack:             9,
			IVDefense:            8,
			IVStamina:            10,
			LevelEstimate:        nil,
			LevelConfidence:      nil,
			LevelMethod:          "UNKNOWN",
			SourceType:           "IMAGE",
			StartMS:              nil,
			EndMS:                nil,
			FrameTimestampMS:     nil,
			ExtractionConfidence: nil,
			MaxCPEvaluations: []PokemonResultMaxCPEvaluationRecord{
				{
					MaxCP:              500,
					EvaluatedSpeciesID: "ivysaur",
					BestLevel:          10.0,
					BestCP:             499,
					StatProduct:        543210.0,
					Rank:               12,
					Percentage:         99.01,
				},
			},
			CreatedAt: laterCreatedAt,
		},
	}
	for idx := range expected {
		assertPokemonResultRecordEqual(t, idx, expected[idx], results[idx])
	}

	repeatedResults, err := store.ListPokemonResults(ctx, sessionA)
	if err != nil {
		t.Fatalf("expected second read to succeed, got %v", err)
	}
	if len(repeatedResults) != len(expected) {
		t.Fatalf("expected %d repeated results, got %d", len(expected), len(repeatedResults))
	}
	for idx := range expected {
		assertPokemonResultRecordEqual(t, idx, expected[idx], repeatedResults[idx])
	}
}

func TestListPokemonResultsReturnsEmptyForUnknownSession(t *testing.T) {
	store, _ := newTestSQLiteStore(t)

	results, err := store.ListPokemonResults(context.Background(), "missing-session")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty results, got %d entries", len(results))
	}
}

func TestListPokemonResultsKeepsLatestDuplicatePerDedupeKey(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	sessionID := "session-a"

	baseCreatedAt := time.Date(2026, time.March, 6, 10, 0, 0, 0, time.UTC)
	levelEstimate := 20.5

	seedUploadRow(t, store.db, "upload-old", sessionID, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-old",
		UploadID:  "upload-old",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:            "result-old",
		JobID:         "job-old",
		UploadID:      "upload-old",
		SessionID:     sessionID,
		SpeciesName:   "Pikachu",
		CP:            410,
		HP:            64,
		IVAttack:      10,
		IVDefense:     12,
		IVStamina:     11,
		LevelEstimate: &levelEstimate,
		LevelMethod:   "UNKNOWN",
		SourceType:    "IMAGE",
		CreatedAt:     baseCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "eval-old",
		AppraisalResultID:  "result-old",
		MaxCP:              1500,
		EvaluatedSpeciesID: "raichu",
		BestLevel:          20.5,
		BestCP:             1498,
		StatProduct:        123456.78,
		RankPosition:       50,
		Percentage:         97.5,
		CreatedAt:          baseCreatedAt,
	})

	laterCreatedAt := baseCreatedAt.Add(time.Minute)
	seedUploadRow(t, store.db, "upload-new", sessionID, laterCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-new",
		UploadID:  "upload-new",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: laterCreatedAt,
		UpdatedAt: laterCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-new",
		JobID:               "job-new",
		UploadID:            "upload-new",
		SessionID:           sessionID,
		SpeciesName:         "  pikachu  ",
		CP:                  410,
		HP:                  64,
		PowerUpStardustCost: 4000,
		IVAttack:            10,
		IVDefense:           12,
		IVStamina:           11,
		LevelEstimate:       &levelEstimate,
		LevelMethod:         "UNKNOWN",
		SourceType:          "VIDEO",
		CreatedAt:           laterCreatedAt,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "eval-new",
		AppraisalResultID:  "result-new",
		MaxCP:              2500,
		EvaluatedSpeciesID: "raichu",
		BestLevel:          30.0,
		BestCP:             2499,
		StatProduct:        234567.89,
		RankPosition:       15,
		Percentage:         99.1,
		CreatedAt:          laterCreatedAt,
	})

	results, err := store.ListPokemonResults(ctx, sessionID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 deduplicated result, got %d", len(results))
	}
	if results[0].ID != "result-new" {
		t.Fatalf("expected latest duplicate survivor %q, got %q", "result-new", results[0].ID)
	}
	if len(results[0].MaxCPEvaluations) != 1 {
		t.Fatalf("expected survivor evaluations only, got %#v", results[0].MaxCPEvaluations)
	}
	if results[0].MaxCPEvaluations[0].MaxCP != 2500 {
		t.Fatalf("expected survivor evaluation max cp 2500, got %#v", results[0].MaxCPEvaluations[0])
	}
}

func TestListPokemonResultsKeepsDistinctLevelEstimates(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	sessionID := "session-a"

	baseCreatedAt := time.Date(2026, time.March, 6, 11, 0, 0, 0, time.UTC)
	levelEstimate := 20.0

	seedUploadRow(t, store.db, "upload-nil-old", sessionID, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-nil-old",
		UploadID:  "upload-nil-old",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:          "result-nil-old",
		JobID:       "job-nil-old",
		UploadID:    "upload-nil-old",
		SessionID:   sessionID,
		SpeciesName: "Eevee",
		CP:          500,
		HP:          80,
		IVAttack:    10,
		IVDefense:   10,
		IVStamina:   10,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		CreatedAt:   baseCreatedAt,
	})

	secondCreatedAt := baseCreatedAt.Add(time.Minute)
	seedUploadRow(t, store.db, "upload-level", sessionID, secondCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-level",
		UploadID:  "upload-level",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: secondCreatedAt,
		UpdatedAt: secondCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:            "result-level",
		JobID:         "job-level",
		UploadID:      "upload-level",
		SessionID:     sessionID,
		SpeciesName:   "Eevee",
		CP:            500,
		HP:            80,
		IVAttack:      10,
		IVDefense:     10,
		IVStamina:     10,
		LevelEstimate: &levelEstimate,
		LevelMethod:   "ARC_POSITION",
		SourceType:    "IMAGE",
		CreatedAt:     secondCreatedAt,
	})

	thirdCreatedAt := secondCreatedAt.Add(time.Minute)
	seedUploadRow(t, store.db, "upload-nil-new", sessionID, thirdCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-nil-new",
		UploadID:  "upload-nil-new",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: thirdCreatedAt,
		UpdatedAt: thirdCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:          "result-nil-new",
		JobID:       "job-nil-new",
		UploadID:    "upload-nil-new",
		SessionID:   sessionID,
		SpeciesName: "eevee",
		CP:          500,
		HP:          80,
		IVAttack:    10,
		IVDefense:   10,
		IVStamina:   10,
		LevelMethod: "UNKNOWN",
		SourceType:  "VIDEO",
		CreatedAt:   thirdCreatedAt,
	})

	results, err := store.ListPokemonResults(ctx, sessionID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 visible results, got %d", len(results))
	}
	if results[0].ID != "result-level" {
		t.Fatalf("expected first visible result %q, got %q", "result-level", results[0].ID)
	}
	if results[1].ID != "result-nil-new" {
		t.Fatalf("expected second visible result %q, got %q", "result-nil-new", results[1].ID)
	}
}

func TestSoftDeletePokemonResultMarksDeletedAndExcludesItFromResults(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 5, 10, 0, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-1", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-1",
		UploadID:  "upload-1",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-1",
		JobID:               "job-1",
		UploadID:            "upload-1",
		SessionID:           sessionID,
		SpeciesName:         "Machop",
		CP:                  512,
		HP:                  64,
		PowerUpStardustCost: 2500,
		IVAttack:            12,
		IVDefense:           15,
		IVStamina:           13,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           now,
	})
	seedPvPEvaluationRow(t, store.db, seededPvPEvaluationRow{
		ID:                 "eval-1",
		AppraisalResultID:  "result-1",
		MaxCP:              1500,
		EvaluatedSpeciesID: "machoke",
		BestLevel:          23.5,
		BestCP:             1498,
		StatProduct:        1567890.12,
		RankPosition:       143,
		Percentage:         93.32,
		CreatedAt:          now,
	})

	later := now.Add(time.Second)
	seedUploadRow(t, store.db, "upload-2", sessionID, later)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-2",
		UploadID:  "upload-2",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: later,
		UpdatedAt: later,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-2",
		JobID:               "job-2",
		UploadID:            "upload-2",
		SessionID:           sessionID,
		SpeciesName:         "Pikachu",
		CP:                  410,
		HP:                  64,
		PowerUpStardustCost: 3000,
		IVAttack:            10,
		IVDefense:           12,
		IVStamina:           11,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           later,
	})

	deleteAt := later.Add(time.Minute)
	if err := store.SoftDeletePokemonResult(ctx, "result-1", sessionID, deleteAt); err != nil {
		t.Fatalf("expected soft delete to succeed, got %v", err)
	}

	var deletedAtRaw sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT deleted_at FROM appraisal_results WHERE id = ?;`, "result-1").Scan(&deletedAtRaw); err != nil {
		t.Fatalf("expected deleted_at query to succeed, got %v", err)
	}
	if !deletedAtRaw.Valid || deletedAtRaw.String != deleteAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("expected deleted_at %q, got %#v", deleteAt.UTC().Format(time.RFC3339Nano), deletedAtRaw)
	}

	results, err := store.ListPokemonResults(ctx, sessionID)
	if err != nil {
		t.Fatalf("expected list pokemon results to succeed, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 remaining result, got %d", len(results))
	}
	if results[0].ID != "result-2" {
		t.Fatalf("expected remaining result %q, got %q", "result-2", results[0].ID)
	}
}

func TestSoftDeletePokemonResultTombstonesDuplicateGroup(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	sessionID := "session-a"
	baseCreatedAt := time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC)
	levelEstimate := 21.0

	seedUploadRow(t, store.db, "upload-older", sessionID, baseCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-older",
		UploadID:  "upload-older",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: baseCreatedAt,
		UpdatedAt: baseCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:            "result-older",
		JobID:         "job-older",
		UploadID:      "upload-older",
		SessionID:     sessionID,
		SpeciesName:   "Machop",
		CP:            512,
		HP:            64,
		IVAttack:      12,
		IVDefense:     15,
		IVStamina:     13,
		LevelEstimate: &levelEstimate,
		LevelMethod:   "ARC_POSITION",
		SourceType:    "IMAGE",
		CreatedAt:     baseCreatedAt,
	})

	laterCreatedAt := baseCreatedAt.Add(time.Minute)
	seedUploadRow(t, store.db, "upload-newer", sessionID, laterCreatedAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-newer",
		UploadID:  "upload-newer",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: laterCreatedAt,
		UpdatedAt: laterCreatedAt,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:            "result-newer",
		JobID:         "job-newer",
		UploadID:      "upload-newer",
		SessionID:     sessionID,
		SpeciesName:   "machop",
		CP:            512,
		HP:            64,
		IVAttack:      12,
		IVDefense:     15,
		IVStamina:     13,
		LevelEstimate: &levelEstimate,
		LevelMethod:   "ARC_POSITION",
		SourceType:    "VIDEO",
		CreatedAt:     laterCreatedAt,
	})

	results, err := store.ListPokemonResults(ctx, sessionID)
	if err != nil {
		t.Fatalf("expected list pokemon results to succeed, got %v", err)
	}
	if len(results) != 1 || results[0].ID != "result-newer" {
		t.Fatalf("expected latest duplicate survivor before delete, got %#v", results)
	}

	deleteAt := laterCreatedAt.Add(time.Minute)
	if err := store.SoftDeletePokemonResult(ctx, "result-newer", sessionID, deleteAt); err != nil {
		t.Fatalf("expected soft delete to succeed, got %v", err)
	}

	var deletedAtRaw sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT deleted_at FROM appraisal_results WHERE id = ?;`, "result-newer").Scan(&deletedAtRaw); err != nil {
		t.Fatalf("expected deleted_at query to succeed, got %v", err)
	}
	if !deletedAtRaw.Valid || deletedAtRaw.String != deleteAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("expected deleted_at %q, got %#v", deleteAt.UTC().Format(time.RFC3339Nano), deletedAtRaw)
	}

	var tombstoneSourceID string
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT source_result_id FROM appraisal_result_dedupe_tombstones WHERE session_id = ? AND dedupe_key = ?;`,
		sessionID,
		pokemonResultDedupKeyFromIdentity(pokemonResultDedupIdentity{
			SpeciesName:   "Machop",
			CP:            512,
			HP:            64,
			IVAttack:      12,
			IVDefense:     15,
			IVStamina:     13,
			LevelEstimate: &levelEstimate,
		}),
	).Scan(&tombstoneSourceID); err != nil {
		t.Fatalf("expected dedupe tombstone query to succeed, got %v", err)
	}
	if tombstoneSourceID != "result-newer" {
		t.Fatalf("expected tombstone source result %q, got %q", "result-newer", tombstoneSourceID)
	}

	results, err = store.ListPokemonResults(ctx, sessionID)
	if err != nil {
		t.Fatalf("expected list pokemon results after delete to succeed, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected duplicate group to remain hidden after survivor delete, got %#v", results)
	}
}

func TestSoftDeletePokemonResultReturnsNotFoundForUnknownSessionOrDeletedRow(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 5, 11, 0, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-1", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-1",
		UploadID:  "upload-1",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-1",
		JobID:               "job-1",
		UploadID:            "upload-1",
		SessionID:           sessionID,
		SpeciesName:         "Machop",
		CP:                  512,
		HP:                  64,
		PowerUpStardustCost: 2500,
		IVAttack:            12,
		IVDefense:           15,
		IVStamina:           13,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           now,
	})

	if err := store.SoftDeletePokemonResult(ctx, "result-1", "session-b", now); !errors.Is(err, ErrPokemonResultNotFound) {
		t.Fatalf("expected ErrPokemonResultNotFound for wrong session, got %v", err)
	}

	if err := store.SoftDeletePokemonResult(ctx, "result-1", sessionID, now); err != nil {
		t.Fatalf("expected first soft delete to succeed, got %v", err)
	}

	if err := store.SoftDeletePokemonResult(ctx, "result-1", sessionID, now.Add(time.Minute)); !errors.Is(err, ErrPokemonResultNotFound) {
		t.Fatalf("expected ErrPokemonResultNotFound for already deleted row, got %v", err)
	}
}

func TestListPendingReadingsReturnsSessionScopedReadingsWithRankedOptions(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()

	sessionA := "session-a"
	sessionB := "session-b"
	createdAt := time.Date(2026, time.March, 6, 11, 0, 0, 0, time.UTC)
	frameTimestamp := int64(300)
	levelEstimate := 23.5
	levelConfidence := 0.72
	extractionConfidence := 0.86

	seedUploadRow(t, store.db, "upload-a", sessionA, createdAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionA,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		FinishedAt: func() *time.Time {
			finished := createdAt
			return &finished
		}(),
	})
	seedUploadRow(t, store.db, "upload-b", sessionB, createdAt)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-b",
		UploadID:  "upload-b",
		SessionID: sessionB,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		FinishedAt: func() *time.Time {
			finished := createdAt
			return &finished
		}(),
	})

	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:                   "reading-a",
		JobID:                "job-a",
		UploadID:             "upload-a",
		SessionID:            sessionA,
		CP:                   712,
		HP:                   120,
		IVAttack:             10,
		IVDefense:            11,
		IVStamina:            12,
		LevelEstimate:        &levelEstimate,
		LevelConfidence:      &levelConfidence,
		LevelMethod:          "ARC_POSITION",
		SourceType:           "VIDEO",
		FrameTimestampMS:     &frameTimestamp,
		ExtractionConfidence: &extractionConfidence,
		Status:               JobStatusPendingUserDedup,
		Locked:               false,
		CreatedAt:            createdAt,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a2",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka (Galarian)",
		MatchMode:        "fuzzy",
		MatchDistance:    1,
		OptionRank:       2,
		CreatedAt:        createdAt,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        createdAt,
	})

	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-a-resolved",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionA,
		CP:          500,
		HP:          100,
		IVAttack:    10,
		IVDefense:   10,
		IVStamina:   10,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      "RESOLVED",
		Locked:      true,
		CreatedAt:   createdAt,
		SelectedSpeciesName: func() *string {
			value := "Pikachu"
			return &value
		}(),
		ResolvedAt: func() *time.Time {
			resolved := createdAt
			return &resolved
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-b",
		JobID:       "job-b",
		UploadID:    "upload-b",
		SessionID:   sessionB,
		CP:          600,
		HP:          90,
		IVAttack:    11,
		IVDefense:   11,
		IVStamina:   11,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   createdAt,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-b1",
		PendingReadingID: "reading-b",
		SpeciesName:      "Pikachu",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        createdAt,
	})

	readings, err := store.ListPendingReadings(ctx, sessionA)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(readings) != 1 {
		t.Fatalf("expected exactly one unresolved reading for session %q, got %d", sessionA, len(readings))
	}
	reading := readings[0]
	if reading.ID != "reading-a" || reading.JobID != "job-a" || reading.UploadID != "upload-a" {
		t.Fatalf("unexpected reading identity payload: %#v", reading)
	}
	if reading.CP != 712 || reading.HP != 120 {
		t.Fatalf("unexpected reading scalar payload: %#v", reading)
	}
	if len(reading.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(reading.Options))
	}
	if reading.Options[0].ID != "option-a1" || reading.Options[1].ID != "option-a2" {
		t.Fatalf("expected option ordering by rank, got %#v", reading.Options)
	}
}

func TestResolvePendingReadingFinalizesResultAndLocksReading(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 0, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	frameTimestamp := int64(600)
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:               "reading-a",
		JobID:            "job-a",
		UploadID:         "upload-a",
		SessionID:        sessionID,
		CP:               712,
		HP:               120,
		IVAttack:         10,
		IVDefense:        11,
		IVStamina:        12,
		LevelMethod:      "ARC_POSITION",
		SourceType:       "VIDEO",
		FrameTimestampMS: &frameTimestamp,
		Status:           JobStatusPendingUserDedup,
		Locked:           false,
		CreatedAt:        now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a2",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka (Galarian)",
		MatchMode:        "fuzzy",
		MatchDistance:    1,
		OptionRank:       2,
		CreatedAt:        now,
	})

	result, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-a",
		OptionID:  "option-a2",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("expected resolve to succeed, got %v", err)
	}
	if result.SpeciesName != "Darumaka (Galarian)" {
		t.Fatalf("expected resolved species %q, got %q", "Darumaka (Galarian)", result.SpeciesName)
	}
	if result.JobID != "job-a" || result.UploadID != "upload-a" || result.SessionID != sessionID {
		t.Fatalf("unexpected resolved result identity: %#v", result)
	}

	var status string
	var locked int
	var selectedSpecies sql.NullString
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT status, locked, selected_species_name FROM appraisal_pending_readings WHERE id = ?;`,
		"reading-a",
	).Scan(&status, &locked, &selectedSpecies); err != nil {
		t.Fatalf("expected pending reading query to succeed, got: %v", err)
	}
	if status != PendingReadingStatusResolved {
		t.Fatalf("expected pending reading status %q, got %q", PendingReadingStatusResolved, status)
	}
	if locked != 1 {
		t.Fatalf("expected pending reading locked=1, got %d", locked)
	}
	if !selectedSpecies.Valid || selectedSpecies.String != "Darumaka (Galarian)" {
		t.Fatalf("expected selected species %q, got %#v", "Darumaka (Galarian)", selectedSpecies)
	}

	var jobStatus string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?;`, "job-a").Scan(&jobStatus); err != nil {
		t.Fatalf("expected job status query to succeed, got: %v", err)
	}
	if jobStatus != JobStatusSucceeded {
		t.Fatalf("expected job status %q, got %q", JobStatusSucceeded, jobStatus)
	}

	assertRowCount(t, store.db, "appraisal_results", "job_id", "job-a", 1)
	assertRowCount(t, store.db, "appraisal_result_pvp_eval_queue", "appraisal_result_id", result.ID, 1)
}

func TestResolvePendingReadingReturnsLockedForAlreadyResolvedReading(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 30, 0, 0, time.UTC)
	sessionID := "session-a"
	selectedSpecies := "Darumaka"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:                  "reading-a",
		JobID:               "job-a",
		UploadID:            "upload-a",
		SessionID:           sessionID,
		CP:                  712,
		HP:                  120,
		IVAttack:            10,
		IVDefense:           11,
		IVStamina:           12,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		Status:              PendingReadingStatusResolved,
		Locked:              true,
		SelectedSpeciesName: &selectedSpecies,
		ResolvedAt: func() *time.Time {
			resolved := now
			return &resolved
		}(),
		CreatedAt: now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now,
	})

	_, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-a",
		OptionID:  "option-a1",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	})
	if !errors.Is(err, ErrPendingReadingLocked) {
		t.Fatalf("expected ErrPendingReadingLocked, got %v", err)
	}
}

func TestDismissPendingReadingLocksReadingWithoutCreatingResult(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 15, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-a",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          712,
		HP:          120,
		IVAttack:    10,
		IVDefense:   11,
		IVStamina:   12,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now,
	})

	if err := store.DismissPendingReading(ctx, DismissPendingReadingParams{
		ReadingID: "reading-a",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("expected dismiss to succeed, got %v", err)
	}

	var status string
	var locked int
	var selectedSpecies sql.NullString
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT status, locked, selected_species_name FROM appraisal_pending_readings WHERE id = ?;`,
		"reading-a",
	).Scan(&status, &locked, &selectedSpecies); err != nil {
		t.Fatalf("expected pending reading query to succeed, got: %v", err)
	}
	if status != PendingReadingStatusDismissed {
		t.Fatalf("expected pending reading status %q, got %q", PendingReadingStatusDismissed, status)
	}
	if locked != 1 {
		t.Fatalf("expected pending reading locked=1, got %d", locked)
	}
	if selectedSpecies.Valid {
		t.Fatalf("expected selected species to be cleared, got %#v", selectedSpecies)
	}

	var jobStatus string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?;`, "job-a").Scan(&jobStatus); err != nil {
		t.Fatalf("expected job status query to succeed, got: %v", err)
	}
	if jobStatus != JobStatusSucceeded {
		t.Fatalf("expected job status %q, got %q", JobStatusSucceeded, jobStatus)
	}

	assertRowCount(t, store.db, "appraisal_results", "job_id", "job-a", 0)
	assertRowCount(t, store.db, "appraisal_result_pvp_eval_queue", "appraisal_result_id", "reading-a", 0)
}

func TestResolveAndDismissPendingReadingsKeepJobPendingUntilLastReading(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 30, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-a",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          712,
		HP:          120,
		IVAttack:    10,
		IVDefense:   11,
		IVStamina:   12,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: "reading-a",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now,
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-b",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          657,
		HP:          98,
		IVAttack:    3,
		IVDefense:   14,
		IVStamina:   10,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now.Add(time.Second),
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-b1",
		PendingReadingID: "reading-b",
		SpeciesName:      "Meowth",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now.Add(time.Second),
	})

	if _, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-a",
		OptionID:  "option-a1",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("expected first resolve to succeed, got %v", err)
	}

	var jobStatus string
	if err := store.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?;`, "job-a").Scan(&jobStatus); err != nil {
		t.Fatalf("expected job status query after resolve to succeed, got %v", err)
	}
	if jobStatus != JobStatusPendingUserDedup {
		t.Fatalf("expected job to remain %q while sibling reading exists, got %q", JobStatusPendingUserDedup, jobStatus)
	}

	if err := store.DismissPendingReading(ctx, DismissPendingReadingParams{
		ReadingID: "reading-b",
		OwnerKey:  sessionID,
		Now:       now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("expected sibling dismiss to succeed, got %v", err)
	}

	if err := store.db.QueryRowContext(ctx, `SELECT status FROM jobs WHERE id = ?;`, "job-a").Scan(&jobStatus); err != nil {
		t.Fatalf("expected final job status query to succeed, got %v", err)
	}
	if jobStatus != JobStatusSucceeded {
		t.Fatalf("expected job to transition to %q after last reading, got %q", JobStatusSucceeded, jobStatus)
	}
}

func TestDismissPendingReadingRepairsStalePendingRowAfterJobSucceeded(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 45, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-stale",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          657,
		HP:          98,
		IVAttack:    3,
		IVDefense:   14,
		IVStamina:   10,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now,
	})

	if err := store.DismissPendingReading(ctx, DismissPendingReadingParams{
		ReadingID: "reading-stale",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("expected dismiss to repair stale pending row, got %v", err)
	}

	var status string
	var locked int
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT status, locked FROM appraisal_pending_readings WHERE id = ?;`,
		"reading-stale",
	).Scan(&status, &locked); err != nil {
		t.Fatalf("expected stale pending reading query to succeed, got %v", err)
	}
	if status != PendingReadingStatusDismissed || locked != 1 {
		t.Fatalf("expected stale row to be dismissed+locked, got status=%q locked=%d", status, locked)
	}
}

func TestDismissPendingReadingReturnsNotFoundAndLockedErrors(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 12, 45, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusSucceeded,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-a",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          712,
		HP:          120,
		IVAttack:    10,
		IVDefense:   11,
		IVStamina:   12,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      PendingReadingStatusDismissed,
		Locked:      true,
		CreatedAt:   now,
		ResolvedAt: func() *time.Time {
			resolved := now
			return &resolved
		}(),
	})

	if err := store.DismissPendingReading(ctx, DismissPendingReadingParams{
		ReadingID: "reading-missing",
		OwnerKey:  sessionID,
		Now:       now,
	}); !errors.Is(err, ErrPendingReadingNotFound) {
		t.Fatalf("expected ErrPendingReadingNotFound, got %v", err)
	}

	if err := store.DismissPendingReading(ctx, DismissPendingReadingParams{
		ReadingID: "reading-a",
		OwnerKey:  sessionID,
		Now:       now.Add(time.Minute),
	}); !errors.Is(err, ErrPendingReadingLocked) {
		t.Fatalf("expected ErrPendingReadingLocked, got %v", err)
	}
}

func TestResolvePendingReadingReturnsNotFoundErrors(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 6, 13, 0, 0, 0, time.UTC)
	sessionID := "session-a"

	seedUploadRow(t, store.db, "upload-a", sessionID, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-a",
		UploadID:  "upload-a",
		SessionID: sessionID,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-a",
		JobID:       "job-a",
		UploadID:    "upload-a",
		SessionID:   sessionID,
		CP:          712,
		HP:          120,
		IVAttack:    10,
		IVDefense:   11,
		IVStamina:   12,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now,
	})

	_, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-missing",
		OptionID:  "option-a1",
		OwnerKey:  sessionID,
		Now:       now,
	})
	if !errors.Is(err, ErrPendingReadingNotFound) {
		t.Fatalf("expected ErrPendingReadingNotFound, got %v", err)
	}

	_, err = store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-a",
		OptionID:  "option-missing",
		OwnerKey:  sessionID,
		Now:       now,
	})
	if !errors.Is(err, ErrPendingOptionNotFound) {
		t.Fatalf("expected ErrPendingOptionNotFound, got %v", err)
	}
}

func TestCreateUploadAndGetActiveJobStatusSupportClerkOwnerKey(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 7, 9, 0, 0, 0, time.UTC)
	ownerKey := OwnerKeyForClerkUser("user_create")

	createdUpload, createdJob, err := store.CreateUploadAndQueuedJob(ctx, CreateParams{
		OwnerKey:    ownerKey,
		Kind:        KindImage,
		MediaURL:    "local://uploads/clerk-image.png",
		ContentType: "image/png",
		ByteSize:    2048,
		Now:         now,
	})
	if err != nil {
		t.Fatalf("expected create upload/job to succeed, got %v", err)
	}
	if createdUpload.SessionID != ownerKey {
		t.Fatalf("expected created upload owner key %q, got %q", ownerKey, createdUpload.SessionID)
	}
	if createdJob.SessionID != ownerKey {
		t.Fatalf("expected created job owner key %q, got %q", ownerKey, createdJob.SessionID)
	}

	activeJob, err := store.GetActiveJobStatus(ctx, ownerKey)
	if err != nil {
		t.Fatalf("expected active job lookup to succeed, got %v", err)
	}
	if activeJob.ID != createdJob.ID {
		t.Fatalf("expected active job id %q, got %q", createdJob.ID, activeJob.ID)
	}
	if activeJob.SessionID != ownerKey {
		t.Fatalf("expected active job owner key %q, got %q", ownerKey, activeJob.SessionID)
	}

	if _, err := store.GetActiveJobStatus(ctx, OwnerKeyForGuest("session-guest")); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected guest owner lookup to miss clerk job, got %v", err)
	}
}

func TestRetryListAndDeleteSupportClerkOwnerKey(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 7, 10, 0, 0, 0, time.UTC)
	ownerKey := OwnerKeyForClerkUser("user_results")

	seedUploadRow(t, store.db, "upload-clerk", ownerKey, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-clerk-parent",
		UploadID:  "upload-clerk",
		SessionID: ownerKey,
		Status:    JobStatusFailed,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedAppraisalResultRow(t, store.db, seededAppraisalResultRow{
		ID:                  "result-clerk",
		JobID:               "job-clerk-parent",
		UploadID:            "upload-clerk",
		SessionID:           ownerKey,
		SpeciesName:         "Machop",
		CP:                  512,
		HP:                  64,
		PowerUpStardustCost: 2500,
		IVAttack:            12,
		IVDefense:           13,
		IVStamina:           14,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           now,
	})

	retryJob, err := store.CreateRetryJob(ctx, "job-clerk-parent", ownerKey, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected retry job creation to succeed, got %v", err)
	}
	if retryJob.SessionID != ownerKey {
		t.Fatalf("expected retry job owner key %q, got %q", ownerKey, retryJob.SessionID)
	}

	results, err := store.ListPokemonResults(ctx, ownerKey)
	if err != nil {
		t.Fatalf("expected list pokemon results to succeed, got %v", err)
	}
	if len(results) != 1 || results[0].ID != "result-clerk" {
		t.Fatalf("expected one clerk-owned result, got %#v", results)
	}

	if err := store.SoftDeletePokemonResult(ctx, "result-clerk", ownerKey, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("expected soft delete to succeed, got %v", err)
	}

	results, err = store.ListPokemonResults(ctx, ownerKey)
	if err != nil {
		t.Fatalf("expected list pokemon results after delete to succeed, got %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected deleted clerk-owned result to be hidden, got %#v", results)
	}

	if _, err := store.CreateRetryJob(ctx, "job-clerk-parent", OwnerKeyForClerkUser("user_other"), now.Add(3*time.Minute)); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("expected clerk owner isolation on retry, got %v", err)
	}
}

func TestResolvePendingReadingSupportsClerkOwnerKey(t *testing.T) {
	store, _ := newTestSQLiteStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 7, 11, 0, 0, 0, time.UTC)
	ownerKey := OwnerKeyForClerkUser("user_pending")

	seedUploadRow(t, store.db, "upload-clerk-pending", ownerKey, now)
	seedJobRow(t, store.db, seededJobRow{
		ID:        "job-clerk-pending",
		UploadID:  "upload-clerk-pending",
		SessionID: ownerKey,
		Status:    JobStatusPendingUserDedup,
		Progress:  100,
		CreatedAt: now,
		UpdatedAt: now,
		FinishedAt: func() *time.Time {
			finished := now
			return &finished
		}(),
	})
	seedPendingReadingRow(t, store.db, seededPendingReadingRow{
		ID:          "reading-clerk",
		JobID:       "job-clerk-pending",
		UploadID:    "upload-clerk-pending",
		SessionID:   ownerKey,
		CP:          712,
		HP:          120,
		IVAttack:    10,
		IVDefense:   11,
		IVStamina:   12,
		LevelMethod: "UNKNOWN",
		SourceType:  "IMAGE",
		Status:      JobStatusPendingUserDedup,
		Locked:      false,
		CreatedAt:   now,
	})
	seedPendingOptionRow(t, store.db, seededPendingOptionRow{
		ID:               "option-clerk",
		PendingReadingID: "reading-clerk",
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        now,
	})

	result, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-clerk",
		OptionID:  "option-clerk",
		OwnerKey:  ownerKey,
		Now:       now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("expected resolve pending reading to succeed, got %v", err)
	}
	if result.SessionID != ownerKey {
		t.Fatalf("expected resolved result owner key %q, got %q", ownerKey, result.SessionID)
	}
	if result.SpeciesName != "Darumaka" {
		t.Fatalf("expected resolved species Darumaka, got %q", result.SpeciesName)
	}

	if _, err := store.ResolvePendingReading(ctx, ResolvePendingReadingParams{
		ReadingID: "reading-clerk",
		OptionID:  "option-clerk",
		OwnerKey:  OwnerKeyForClerkUser("user_other"),
		Now:       now.Add(2 * time.Minute),
	}); !errors.Is(err, ErrPendingReadingNotFound) {
		t.Fatalf("expected clerk owner isolation on resolve, got %v", err)
	}
}

func newTestSQLiteStore(t *testing.T) (*sqliteStore, string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "upload.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store to initialize, got: %v", err)
	}

	sqliteStore, ok := store.(*sqliteStore)
	if !ok {
		t.Fatalf("expected *sqliteStore, got %T", store)
	}

	return sqliteStore, dbPath
}

func assertSQLiteObjectExists(t *testing.T, db *sql.DB, objectType, objectName string) {
	t.Helper()

	const query = `
SELECT COUNT(*)
FROM sqlite_master
WHERE type = ? AND name = ?;`

	var count int
	if err := db.QueryRowContext(context.Background(), query, objectType, objectName).Scan(&count); err != nil {
		t.Fatalf("expected sqlite object lookup to succeed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected sqlite %s %q to exist, count=%d", objectType, objectName, count)
	}
}

func assertSQLiteColumnExists(t *testing.T, db *sql.DB, tableName string, columnName string) {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), "PRAGMA table_info("+tableName+");")
	if err != nil {
		t.Fatalf("expected table info query to succeed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("expected table info scan to succeed: %v", err)
		}
		if name == columnName {
			return
		}
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("expected table info rows to succeed: %v", err)
	}
	t.Fatalf("expected sqlite column %s.%s to exist", tableName, columnName)
}

func assertRowCount(t *testing.T, db *sql.DB, table, column, value string, expected int) {
	t.Helper()

	query := "SELECT COUNT(*) FROM " + table + " WHERE " + column + " = ?;"

	var count int
	if err := db.QueryRowContext(context.Background(), query, value).Scan(&count); err != nil {
		t.Fatalf("expected row count query to succeed for %s.%s: %v", table, column, err)
	}
	if count != expected {
		t.Fatalf("expected %d rows in %s for %s=%q, got %d", expected, table, column, value, count)
	}
}

type jobSnapshot struct {
	UploadID     string
	SessionID    string
	ParentJobID  sql.NullString
	Status       string
	Progress     int
	Stage        sql.NullString
	WorkerID     sql.NullString
	ClaimedAt    sql.NullString
	HeartbeatAt  sql.NullString
	ErrorCode    sql.NullString
	ErrorMessage sql.NullString
	CreatedAtRaw string
	UpdatedAtRaw string
	FinishedAt   sql.NullString
}

func readJobSnapshot(t *testing.T, db *sql.DB, jobID string) jobSnapshot {
	t.Helper()

	const query = `
SELECT upload_id, session_id, parent_job_id, status, progress, stage, worker_id, claimed_at,
       heartbeat_at, error_code, error_message, created_at, updated_at, finished_at
FROM jobs
WHERE id = ?;`

	var snapshot jobSnapshot
	if err := db.QueryRowContext(context.Background(), query, jobID).Scan(
		&snapshot.UploadID,
		&snapshot.SessionID,
		&snapshot.ParentJobID,
		&snapshot.Status,
		&snapshot.Progress,
		&snapshot.Stage,
		&snapshot.WorkerID,
		&snapshot.ClaimedAt,
		&snapshot.HeartbeatAt,
		&snapshot.ErrorCode,
		&snapshot.ErrorMessage,
		&snapshot.CreatedAtRaw,
		&snapshot.UpdatedAtRaw,
		&snapshot.FinishedAt,
	); err != nil {
		t.Fatalf("expected job snapshot query to succeed for %q: %v", jobID, err)
	}

	return snapshot
}

type seededJobRow struct {
	ID           string
	UploadID     string
	SessionID    string
	Status       string
	Progress     int
	Stage        *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	FinishedAt   *time.Time
	ErrorCode    *string
	ErrorMessage *string
}

type seededAppraisalResultRow struct {
	ID                   string
	JobID                string
	UploadID             string
	SessionID            string
	SpeciesName          string
	CP                   int
	HP                   int
	PowerUpStardustCost  int
	IVAttack             int
	IVDefense            int
	IVStamina            int
	LevelEstimate        *float64
	LevelConfidence      *float64
	LevelMethod          string
	SourceType           string
	StartMS              *int64
	EndMS                *int64
	FrameTimestampMS     *int64
	ExtractionConfidence *float64
	CreatedAt            time.Time
}

type seededPvPEvaluationRow struct {
	ID                 string
	AppraisalResultID  string
	MaxCP              int
	EvaluatedSpeciesID string
	BestLevel          float64
	BestCP             int
	StatProduct        float64
	RankPosition       int
	Percentage         float64
	CreatedAt          time.Time
}

type seededPendingReadingRow struct {
	ID                   string
	JobID                string
	UploadID             string
	SessionID            string
	CP                   int
	HP                   int
	IVAttack             int
	IVDefense            int
	IVStamina            int
	LevelEstimate        *float64
	LevelConfidence      *float64
	LevelMethod          string
	SourceType           string
	FrameTimestampMS     *int64
	ExtractionConfidence *float64
	Status               string
	Locked               bool
	SelectedSpeciesName  *string
	ResolvedAt           *time.Time
	CreatedAt            time.Time
}

type seededPendingOptionRow struct {
	ID               string
	PendingReadingID string
	SpeciesName      string
	MatchMode        string
	MatchDistance    int
	OptionRank       int
	CreatedAt        time.Time
}

func seedUploadRow(t *testing.T, db *sql.DB, uploadID, sessionID string, createdAt time.Time) {
	t.Helper()

	const insertUpload = `
INSERT INTO uploads(id, session_id, kind, uploadthing_url, content_type, byte_size, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.ExecContext(
		context.Background(),
		insertUpload,
		uploadID,
		sessionID,
		KindImage,
		"local://uploads/test.png",
		"image/png",
		1024,
		createdAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected upload seed insert to succeed: %v", err)
	}
}

func seedJobRow(t *testing.T, db *sql.DB, row seededJobRow) {
	t.Helper()

	const insertJob = `
INSERT INTO jobs(
	id, upload_id, session_id, parent_job_id, status, progress, stage,
	worker_id, claimed_at, heartbeat_at, error_code, error_message,
	created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var stage interface{}
	if row.Stage != nil {
		stage = *row.Stage
	}

	var finishedAt interface{}
	if row.FinishedAt != nil {
		finishedAt = row.FinishedAt.UTC().Format(time.RFC3339Nano)
	}

	var errorCode interface{}
	if row.ErrorCode != nil {
		errorCode = *row.ErrorCode
	}

	var errorMessage interface{}
	if row.ErrorMessage != nil {
		errorMessage = *row.ErrorMessage
	}

	if _, err := db.ExecContext(
		context.Background(),
		insertJob,
		row.ID,
		row.UploadID,
		row.SessionID,
		nil,
		row.Status,
		row.Progress,
		stage,
		nil,
		nil,
		nil,
		errorCode,
		errorMessage,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
		row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		finishedAt,
	); err != nil {
		t.Fatalf("expected job seed insert to succeed: %v", err)
	}
}

func seedAppraisalResultRow(t *testing.T, db *sql.DB, row seededAppraisalResultRow) {
	t.Helper()

	const insertResult = `
INSERT INTO appraisal_results(
	id, job_id, upload_id, session_id, species_name, cp, hp, power_up_stardust_cost,
	iv_attack, iv_defense, iv_stamina, level_estimate, level_confidence, level_method,
	source_type, start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var levelEstimate interface{}
	if row.LevelEstimate != nil {
		levelEstimate = *row.LevelEstimate
	}

	var levelConfidence interface{}
	if row.LevelConfidence != nil {
		levelConfidence = *row.LevelConfidence
	}

	var startMS interface{}
	if row.StartMS != nil {
		startMS = *row.StartMS
	}

	var endMS interface{}
	if row.EndMS != nil {
		endMS = *row.EndMS
	}

	var frameTimestampMS interface{}
	if row.FrameTimestampMS != nil {
		frameTimestampMS = *row.FrameTimestampMS
	}

	var extractionConfidence interface{}
	if row.ExtractionConfidence != nil {
		extractionConfidence = *row.ExtractionConfidence
	}

	if _, err := db.ExecContext(
		context.Background(),
		insertResult,
		row.ID,
		row.JobID,
		row.UploadID,
		row.SessionID,
		row.SpeciesName,
		row.CP,
		row.HP,
		row.PowerUpStardustCost,
		row.IVAttack,
		row.IVDefense,
		row.IVStamina,
		levelEstimate,
		levelConfidence,
		row.LevelMethod,
		row.SourceType,
		startMS,
		endMS,
		frameTimestampMS,
		extractionConfidence,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected appraisal result seed insert to succeed: %v", err)
	}
}

func seedPvPEvaluationRow(t *testing.T, db *sql.DB, row seededPvPEvaluationRow) {
	t.Helper()

	const insertEvaluation = `
INSERT INTO appraisal_result_pvp_evaluations(
	id, appraisal_result_id, max_cp, evaluated_species_id, best_level, best_cp,
	stat_product, rank_position, percentage, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.ExecContext(
		context.Background(),
		insertEvaluation,
		row.ID,
		row.AppraisalResultID,
		row.MaxCP,
		row.EvaluatedSpeciesID,
		row.BestLevel,
		row.BestCP,
		row.StatProduct,
		row.RankPosition,
		row.Percentage,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected pvp evaluation seed insert to succeed: %v", err)
	}
}

func seedPendingReadingRow(t *testing.T, db *sql.DB, row seededPendingReadingRow) {
	t.Helper()

	const insertPendingReading = `
INSERT INTO appraisal_pending_readings(
	id, job_id, upload_id, session_id, cp, hp, iv_attack, iv_defense, iv_stamina,
	level_estimate, level_confidence, level_method, source_type, frame_timestamp_ms,
	extraction_confidence, status, locked, selected_species_name, resolved_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var levelEstimate interface{}
	if row.LevelEstimate != nil {
		levelEstimate = *row.LevelEstimate
	}
	var levelConfidence interface{}
	if row.LevelConfidence != nil {
		levelConfidence = *row.LevelConfidence
	}
	var frameTimestampMS interface{}
	if row.FrameTimestampMS != nil {
		frameTimestampMS = *row.FrameTimestampMS
	}
	var extractionConfidence interface{}
	if row.ExtractionConfidence != nil {
		extractionConfidence = *row.ExtractionConfidence
	}
	var selectedSpeciesName interface{}
	if row.SelectedSpeciesName != nil {
		selectedSpeciesName = *row.SelectedSpeciesName
	}
	var resolvedAt interface{}
	if row.ResolvedAt != nil {
		resolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339Nano)
	}

	locked := 0
	if row.Locked {
		locked = 1
	}

	if _, err := db.ExecContext(
		context.Background(),
		insertPendingReading,
		row.ID,
		row.JobID,
		row.UploadID,
		row.SessionID,
		row.CP,
		row.HP,
		row.IVAttack,
		row.IVDefense,
		row.IVStamina,
		levelEstimate,
		levelConfidence,
		row.LevelMethod,
		row.SourceType,
		frameTimestampMS,
		extractionConfidence,
		row.Status,
		locked,
		selectedSpeciesName,
		resolvedAt,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected pending reading seed insert to succeed: %v", err)
	}
}

func seedPendingOptionRow(t *testing.T, db *sql.DB, row seededPendingOptionRow) {
	t.Helper()

	const insertPendingOption = `
INSERT INTO appraisal_pending_species_options(
	id, pending_reading_id, species_name, species_name_normalized,
	match_mode, match_distance, option_rank, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.ExecContext(
		context.Background(),
		insertPendingOption,
		row.ID,
		row.PendingReadingID,
		row.SpeciesName,
		row.SpeciesName,
		row.MatchMode,
		row.MatchDistance,
		row.OptionRank,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected pending option seed insert to succeed: %v", err)
	}
}

func assertPokemonResultRecordEqual(t *testing.T, index int, expected PokemonResultRecord, actual PokemonResultRecord) {
	t.Helper()

	if actual.ID != expected.ID {
		t.Fatalf("result[%d]: expected ID %q, got %q", index, expected.ID, actual.ID)
	}
	if actual.JobID != expected.JobID {
		t.Fatalf("result[%d]: expected JobID %q, got %q", index, expected.JobID, actual.JobID)
	}
	if actual.UploadID != expected.UploadID {
		t.Fatalf("result[%d]: expected UploadID %q, got %q", index, expected.UploadID, actual.UploadID)
	}
	if actual.SessionID != expected.SessionID {
		t.Fatalf("result[%d]: expected SessionID %q, got %q", index, expected.SessionID, actual.SessionID)
	}
	if actual.SpeciesName != expected.SpeciesName {
		t.Fatalf("result[%d]: expected SpeciesName %q, got %q", index, expected.SpeciesName, actual.SpeciesName)
	}
	if actual.CP != expected.CP {
		t.Fatalf("result[%d]: expected CP %d, got %d", index, expected.CP, actual.CP)
	}
	if actual.HP != expected.HP {
		t.Fatalf("result[%d]: expected HP %d, got %d", index, expected.HP, actual.HP)
	}
	if actual.PowerUpStardustCost != expected.PowerUpStardustCost {
		t.Fatalf(
			"result[%d]: expected PowerUpStardustCost %d, got %d",
			index,
			expected.PowerUpStardustCost,
			actual.PowerUpStardustCost,
		)
	}
	if actual.IVAttack != expected.IVAttack {
		t.Fatalf("result[%d]: expected IVAttack %d, got %d", index, expected.IVAttack, actual.IVAttack)
	}
	if actual.IVDefense != expected.IVDefense {
		t.Fatalf("result[%d]: expected IVDefense %d, got %d", index, expected.IVDefense, actual.IVDefense)
	}
	if actual.IVStamina != expected.IVStamina {
		t.Fatalf("result[%d]: expected IVStamina %d, got %d", index, expected.IVStamina, actual.IVStamina)
	}
	if actual.LevelMethod != expected.LevelMethod {
		t.Fatalf("result[%d]: expected LevelMethod %q, got %q", index, expected.LevelMethod, actual.LevelMethod)
	}
	if actual.SourceType != expected.SourceType {
		t.Fatalf("result[%d]: expected SourceType %q, got %q", index, expected.SourceType, actual.SourceType)
	}
	if !actual.CreatedAt.Equal(expected.CreatedAt) {
		t.Fatalf("result[%d]: expected CreatedAt %s, got %s", index, expected.CreatedAt, actual.CreatedAt)
	}

	assertOptionalFloat64PointerEqual(t, index, "LevelEstimate", expected.LevelEstimate, actual.LevelEstimate)
	assertOptionalFloat64PointerEqual(t, index, "LevelConfidence", expected.LevelConfidence, actual.LevelConfidence)
	assertOptionalInt64PointerEqual(t, index, "StartMS", expected.StartMS, actual.StartMS)
	assertOptionalInt64PointerEqual(t, index, "EndMS", expected.EndMS, actual.EndMS)
	assertOptionalInt64PointerEqual(t, index, "FrameTimestampMS", expected.FrameTimestampMS, actual.FrameTimestampMS)
	assertOptionalFloat64PointerEqual(t, index, "ExtractionConfidence", expected.ExtractionConfidence, actual.ExtractionConfidence)
	assertPokemonResultMaxCPEvaluationRecordsEqual(t, index, expected.MaxCPEvaluations, actual.MaxCPEvaluations)
}

func assertPokemonResultMaxCPEvaluationRecordsEqual(
	t *testing.T,
	index int,
	expected []PokemonResultMaxCPEvaluationRecord,
	actual []PokemonResultMaxCPEvaluationRecord,
) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("result[%d]: expected %d max cp evaluations, got %d", index, len(expected), len(actual))
	}

	for i := range expected {
		if actual[i].MaxCP != expected[i].MaxCP {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected MaxCP %d, got %d",
				index,
				i,
				expected[i].MaxCP,
				actual[i].MaxCP,
			)
		}
		if actual[i].EvaluatedSpeciesID != expected[i].EvaluatedSpeciesID {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected EvaluatedSpeciesID %q, got %q",
				index,
				i,
				expected[i].EvaluatedSpeciesID,
				actual[i].EvaluatedSpeciesID,
			)
		}
		if actual[i].BestLevel != expected[i].BestLevel {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected BestLevel %v, got %v",
				index,
				i,
				expected[i].BestLevel,
				actual[i].BestLevel,
			)
		}
		if actual[i].BestCP != expected[i].BestCP {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected BestCP %d, got %d",
				index,
				i,
				expected[i].BestCP,
				actual[i].BestCP,
			)
		}
		if actual[i].StatProduct != expected[i].StatProduct {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected StatProduct %v, got %v",
				index,
				i,
				expected[i].StatProduct,
				actual[i].StatProduct,
			)
		}
		if actual[i].Rank != expected[i].Rank {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected Rank %d, got %d",
				index,
				i,
				expected[i].Rank,
				actual[i].Rank,
			)
		}
		if actual[i].Percentage != expected[i].Percentage {
			t.Fatalf(
				"result[%d].maxCpEvaluations[%d]: expected Percentage %v, got %v",
				index,
				i,
				expected[i].Percentage,
				actual[i].Percentage,
			)
		}
	}
}

func assertOptionalFloat64PointerEqual(t *testing.T, index int, field string, expected *float64, actual *float64) {
	t.Helper()

	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil && actual != nil:
		t.Fatalf("result[%d]: expected %s nil, got %v", index, field, *actual)
	case expected != nil && actual == nil:
		t.Fatalf("result[%d]: expected %s %v, got nil", index, field, *expected)
	case *expected != *actual:
		t.Fatalf("result[%d]: expected %s %v, got %v", index, field, *expected, *actual)
	}
}

func assertOptionalInt64PointerEqual(t *testing.T, index int, field string, expected *int64, actual *int64) {
	t.Helper()

	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil && actual != nil:
		t.Fatalf("result[%d]: expected %s nil, got %d", index, field, *actual)
	case expected != nil && actual == nil:
		t.Fatalf("result[%d]: expected %s %d, got nil", index, field, *expected)
	case *expected != *actual:
		t.Fatalf("result[%d]: expected %s %d, got %d", index, field, *expected, *actual)
	}
}

func assertOptionalStringEqual(t *testing.T, caseName, field string, expected, actual *string) {
	t.Helper()

	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil && actual != nil:
		t.Fatalf("%s: expected %s nil, got %q", caseName, field, *actual)
	case expected != nil && actual == nil:
		t.Fatalf("%s: expected %s %q, got nil", caseName, field, *expected)
	case *expected != *actual:
		t.Fatalf("%s: expected %s %q, got %q", caseName, field, *expected, *actual)
	}
}

func assertOptionalTimeEqual(t *testing.T, caseName, field string, expected, actual *time.Time) {
	t.Helper()

	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil && actual != nil:
		t.Fatalf("%s: expected %s nil, got %s", caseName, field, actual)
	case expected != nil && actual == nil:
		t.Fatalf("%s: expected %s %s, got nil", caseName, field, expected)
	case !expected.Equal(*actual):
		t.Fatalf("%s: expected %s %s, got %s", caseName, field, expected, actual)
	}
}
