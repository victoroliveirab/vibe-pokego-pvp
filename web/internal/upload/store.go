package upload

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

// CreateParams contains data required to create an upload and its queued job.
type CreateParams struct {
	UploadID    string
	JobID       string
	SessionID   string
	Kind        string
	MediaURL    string
	ContentType string
	ByteSize    int64
	Now         time.Time
}

// Store persists upload metadata and processing jobs.
type Store interface {
	CreateUploadAndQueuedJob(ctx context.Context, params CreateParams) (Upload, Job, error)
	CreateRetryJob(ctx context.Context, parentJobID string, sessionID string, now time.Time) (RetryJob, error)
	GetJobStatus(ctx context.Context, jobID string, sessionID string) (JobStatusRecord, error)
	ListPokemonResultsBySession(ctx context.Context, sessionID string) ([]PokemonResultRecord, error)
	ListPendingReadingsBySession(ctx context.Context, sessionID string) ([]PendingSpeciesReadingRecord, error)
	ResolvePendingReading(ctx context.Context, params ResolvePendingReadingParams) (PokemonResultRecord, error)
}

type sqliteStore struct {
	db *sql.DB
}

// EnsureSQLiteSchema creates upload/job tables and indexes if missing.
func EnsureSQLiteSchema(databaseURL string) error {
	normalizedURL := normalizeDatabaseURL(databaseURL)
	db, err := sql.Open("libsql", normalizedURL)
	if err != nil {
		return fmt.Errorf("open libsql db: %w", err)
	}
	defer db.Close()

	if err := configureSQLiteDB(db, isLocalDatabaseURL(normalizedURL)); err != nil {
		return err
	}

	store := &sqliteStore{db: db}
	if err := store.bootstrap(context.Background()); err != nil {
		return err
	}

	return nil
}

// NewSQLiteStore initializes a SQLite-backed upload/job store.
func NewSQLiteStore(databaseURL string) (Store, error) {
	normalizedURL := normalizeDatabaseURL(databaseURL)
	db, err := sql.Open("libsql", normalizedURL)
	if err != nil {
		return nil, fmt.Errorf("open libsql db: %w", err)
	}

	if err := configureSQLiteDB(db, isLocalDatabaseURL(normalizedURL)); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &sqliteStore{db: db}
	if err := store.bootstrap(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func configureSQLiteDB(db *sql.DB, isLocal bool) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if !isLocal {
		return nil
	}

	if err := applySQLiteBusyTimeout(db); err != nil {
		return fmt.Errorf("configure sqlite busy timeout: %w", err)
	}

	return nil
}

func applySQLiteBusyTimeout(db *sql.DB) error {
	rows, err := db.Query("PRAGMA busy_timeout = 5000;")
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.Err()
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

func (s *sqliteStore) bootstrap(ctx context.Context) error {
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

CREATE TABLE IF NOT EXISTS appraisal_candidates (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	source_type TEXT NOT NULL,
	frame_timestamp_ms INTEGER NULL,
	species_name_raw TEXT NULL,
	species_name_normalized TEXT NULL,
	species_is_canonical INTEGER NOT NULL,
	cp_raw TEXT NULL,
	hp_raw TEXT NULL,
	stardust_raw TEXT NULL,
	iv_attack_raw TEXT NULL,
	iv_defense_raw TEXT NULL,
	iv_stamina_raw TEXT NULL,
	extraction_confidence REAL NULL,
	raw_text TEXT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY(job_id) REFERENCES jobs(id),
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE TABLE IF NOT EXISTS appraisal_results (
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
	created_at TEXT NOT NULL,
	FOREIGN KEY(job_id) REFERENCES jobs(id),
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE TABLE IF NOT EXISTS appraisal_pending_readings (
	id TEXT PRIMARY KEY,
	job_id TEXT NOT NULL,
	upload_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	cp INTEGER NOT NULL,
	hp INTEGER NOT NULL,
	iv_attack INTEGER NOT NULL,
	iv_defense INTEGER NOT NULL,
	iv_stamina INTEGER NOT NULL,
	level_estimate REAL NULL,
	level_confidence REAL NULL,
	level_method TEXT NOT NULL,
	source_type TEXT NOT NULL,
	frame_timestamp_ms INTEGER NULL,
	extraction_confidence REAL NULL,
	status TEXT NOT NULL,
	locked INTEGER NOT NULL,
	selected_species_name TEXT NULL,
	resolved_at TEXT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY(job_id) REFERENCES jobs(id),
	FOREIGN KEY(upload_id) REFERENCES uploads(id)
);

CREATE TABLE IF NOT EXISTS appraisal_pending_species_options (
	id TEXT PRIMARY KEY,
	pending_reading_id TEXT NOT NULL,
	species_name TEXT NOT NULL,
	species_name_normalized TEXT NOT NULL,
	match_mode TEXT NOT NULL,
	match_distance INTEGER NOT NULL,
	option_rank INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY(pending_reading_id) REFERENCES appraisal_pending_readings(id)
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

CREATE INDEX IF NOT EXISTS idx_jobs_status_created_at ON jobs(status, created_at);
CREATE INDEX IF NOT EXISTS idx_jobs_status_heartbeat_at ON jobs(status, heartbeat_at);
CREATE INDEX IF NOT EXISTS idx_appraisal_candidates_job_id ON appraisal_candidates(job_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_candidates_upload_id ON appraisal_candidates(upload_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_candidates_session_id ON appraisal_candidates(session_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_results_job_id ON appraisal_results(job_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_results_upload_id ON appraisal_results(upload_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_results_session_id ON appraisal_results(session_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_pending_readings_job_id ON appraisal_pending_readings(job_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_pending_readings_session_id ON appraisal_pending_readings(session_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_pending_species_options_pending_reading_id ON appraisal_pending_species_options(pending_reading_id);
CREATE INDEX IF NOT EXISTS idx_job_debug_jobs_session_id ON job_debug_jobs(session_id);
CREATE INDEX IF NOT EXISTS idx_job_debug_jobs_created_at ON job_debug_jobs(created_at);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_frame_index ON job_debug_frames(job_id, frame_index);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_frame_ts ON job_debug_frames(job_id, frame_timestamp_ms);
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_created_at ON job_debug_frames(job_id, created_at);
`

	if err := execSchemaStatements(ctx, s.db, schema); err != nil {
		return fmt.Errorf("bootstrap upload/jobs schema: %w", err)
	}

	return nil
}

func execSchemaStatements(ctx context.Context, db *sql.DB, schema string) error {
	statements := strings.Split(schema, ";")
	for idx, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("exec schema statement %d: %w", idx+1, err)
		}
	}
	return nil
}

// CreateUploadAndQueuedJob atomically creates an upload row and a queued job row.
func (s *sqliteStore) CreateUploadAndQueuedJob(ctx context.Context, params CreateParams) (Upload, Job, error) {
	uploadID := params.UploadID
	if uploadID == "" {
		id, err := NewUploadID()
		if err != nil {
			return Upload{}, Job{}, err
		}
		uploadID = id
	}

	jobID := params.JobID
	if jobID == "" {
		id, err := NewJobID()
		if err != nil {
			return Upload{}, Job{}, err
		}
		jobID = id
	}

	now := params.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	timestamp := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Upload{}, Job{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const insertUpload = `
INSERT INTO uploads(id, session_id, kind, uploadthing_url, content_type, byte_size, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.ExecContext(
		ctx,
		insertUpload,
		uploadID,
		params.SessionID,
		params.Kind,
		params.MediaURL,
		params.ContentType,
		params.ByteSize,
		timestamp,
	); err != nil {
		return Upload{}, Job{}, fmt.Errorf("insert upload: %w", err)
	}

	const insertJob = `
INSERT INTO jobs(
	id, upload_id, session_id, parent_job_id, status, progress, stage,
	worker_id, claimed_at, heartbeat_at, error_code, error_message,
	created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.ExecContext(
		ctx,
		insertJob,
		jobID,
		uploadID,
		params.SessionID,
		nil,
		JobStatusQueued,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		timestamp,
		timestamp,
		nil,
	); err != nil {
		return Upload{}, Job{}, fmt.Errorf("insert job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Upload{}, Job{}, fmt.Errorf("commit tx: %w", err)
	}

	return Upload{
			ID:          uploadID,
			SessionID:   params.SessionID,
			Kind:        params.Kind,
			MediaURL:    params.MediaURL,
			ContentType: params.ContentType,
			ByteSize:    params.ByteSize,
			CreatedAt:   now,
		}, Job{
			ID:        jobID,
			UploadID:  uploadID,
			SessionID: params.SessionID,
			Status:    JobStatusQueued,
			Progress:  0,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil
}

// CreateRetryJob creates a new queued child job for an existing parent job owned by the session.
func (s *sqliteStore) CreateRetryJob(ctx context.Context, parentJobID string, sessionID string, now time.Time) (RetryJob, error) {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	timestamp := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RetryJob{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	const queryParentUpload = `
SELECT upload_id, status
FROM jobs
WHERE id = ? AND session_id = ?;`

	var uploadID string
	var status string
	if err := tx.QueryRowContext(ctx, queryParentUpload, parentJobID, sessionID).Scan(&uploadID, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RetryJob{}, ErrJobNotFound
		}

		return RetryJob{}, fmt.Errorf("query parent job by session: %w", err)
	}
	if status != JobStatusFailed {
		return RetryJob{}, ErrJobRetryNotAllowed
	}

	retryJobID, err := NewJobID()
	if err != nil {
		return RetryJob{}, err
	}

	const insertRetryJob = `
INSERT INTO jobs(
	id, upload_id, session_id, parent_job_id, status, progress, stage,
	worker_id, claimed_at, heartbeat_at, error_code, error_message,
	created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.ExecContext(
		ctx,
		insertRetryJob,
		retryJobID,
		uploadID,
		sessionID,
		parentJobID,
		JobStatusQueued,
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		timestamp,
		timestamp,
		nil,
	); err != nil {
		return RetryJob{}, fmt.Errorf("insert retry job: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return RetryJob{}, fmt.Errorf("commit tx: %w", err)
	}

	return RetryJob{
		ID:          retryJobID,
		ParentJobID: parentJobID,
		UploadID:    uploadID,
		SessionID:   sessionID,
		Status:      JobStatusQueued,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// GetJobStatus returns one job status record scoped to a session.
func (s *sqliteStore) GetJobStatus(ctx context.Context, jobID string, sessionID string) (JobStatusRecord, error) {
	const query = `
SELECT id, upload_id, session_id, status, progress, stage,
       created_at, updated_at, finished_at, error_code, error_message
FROM jobs
WHERE id = ? AND session_id = ?;`

	var record JobStatusRecord
	var stage sql.NullString
	var createdAtRaw string
	var updatedAtRaw string
	var finishedAtRaw sql.NullString
	var errorCode sql.NullString
	var errorMessage sql.NullString

	if err := s.db.QueryRowContext(ctx, query, jobID, sessionID).Scan(
		&record.ID,
		&record.UploadID,
		&record.SessionID,
		&record.Status,
		&record.Progress,
		&stage,
		&createdAtRaw,
		&updatedAtRaw,
		&finishedAtRaw,
		&errorCode,
		&errorMessage,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobStatusRecord{}, ErrJobNotFound
		}

		return JobStatusRecord{}, fmt.Errorf("query job status by session: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return JobStatusRecord{}, fmt.Errorf("parse created_at %q: %w", createdAtRaw, err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
	if err != nil {
		return JobStatusRecord{}, fmt.Errorf("parse updated_at %q: %w", updatedAtRaw, err)
	}

	record.Stage = nullableString(stage)
	record.CreatedAt = createdAt
	record.UpdatedAt = updatedAt

	if finishedAtRaw.Valid {
		finishedAt, err := time.Parse(time.RFC3339Nano, finishedAtRaw.String)
		if err != nil {
			return JobStatusRecord{}, fmt.Errorf("parse finished_at %q: %w", finishedAtRaw.String, err)
		}
		record.FinishedAt = &finishedAt
	}

	record.ErrorCode = nullableString(errorCode)
	record.ErrorMessage = nullableString(errorMessage)

	return record, nil
}

// ListPokemonResultsBySession returns accepted appraisal results for one session.
func (s *sqliteStore) ListPokemonResultsBySession(ctx context.Context, sessionID string) ([]PokemonResultRecord, error) {
	const query = `
SELECT id, job_id, upload_id, session_id, species_name, cp, hp,
       power_up_stardust_cost, iv_attack, iv_defense, iv_stamina,
       level_estimate, level_confidence, level_method, source_type,
       start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
FROM appraisal_results
WHERE session_id = ?
ORDER BY created_at ASC, id ASC;`

	rows, err := s.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query pokemon results by session: %w", err)
	}
	defer rows.Close()

	results := make([]PokemonResultRecord, 0)
	for rows.Next() {
		var record PokemonResultRecord
		var levelEstimate sql.NullFloat64
		var levelConfidence sql.NullFloat64
		var startMS sql.NullInt64
		var endMS sql.NullInt64
		var frameTimestampMS sql.NullInt64
		var extractionConfidence sql.NullFloat64
		var createdAtRaw string

		if err := rows.Scan(
			&record.ID,
			&record.JobID,
			&record.UploadID,
			&record.SessionID,
			&record.SpeciesName,
			&record.CP,
			&record.HP,
			&record.PowerUpStardustCost,
			&record.IVAttack,
			&record.IVDefense,
			&record.IVStamina,
			&levelEstimate,
			&levelConfidence,
			&record.LevelMethod,
			&record.SourceType,
			&startMS,
			&endMS,
			&frameTimestampMS,
			&extractionConfidence,
			&createdAtRaw,
		); err != nil {
			return nil, fmt.Errorf("scan pokemon result row: %w", err)
		}

		createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse appraisal_result created_at %q: %w", createdAtRaw, err)
		}

		record.LevelEstimate = nullableFloat64(levelEstimate)
		record.LevelConfidence = nullableFloat64(levelConfidence)
		record.StartMS = nullableInt64(startMS)
		record.EndMS = nullableInt64(endMS)
		record.FrameTimestampMS = nullableInt64(frameTimestampMS)
		record.ExtractionConfidence = nullableFloat64(extractionConfidence)
		record.CreatedAt = createdAt

		results = append(results, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pokemon results rows: %w", err)
	}

	return results, nil
}

// ListPendingReadingsBySession returns unresolved pending readings with ranked options for one session.
func (s *sqliteStore) ListPendingReadingsBySession(ctx context.Context, sessionID string) ([]PendingSpeciesReadingRecord, error) {
	const queryPendingReadings = `
SELECT id, job_id, upload_id, session_id, cp, hp, iv_attack, iv_defense, iv_stamina,
       level_estimate, level_confidence, level_method, source_type, frame_timestamp_ms,
       extraction_confidence, status, locked, selected_species_name, resolved_at, created_at
FROM appraisal_pending_readings
WHERE session_id = ? AND status = ? AND locked = 0
ORDER BY created_at ASC, id ASC;`

	rows, err := s.db.QueryContext(ctx, queryPendingReadings, sessionID, JobStatusPendingUserDedup)
	if err != nil {
		return nil, fmt.Errorf("query pending readings by session: %w", err)
	}
	defer rows.Close()

	readings := make([]PendingSpeciesReadingRecord, 0)
	for rows.Next() {
		var reading PendingSpeciesReadingRecord
		var levelEstimate sql.NullFloat64
		var levelConfidence sql.NullFloat64
		var frameTimestampMS sql.NullInt64
		var extractionConfidence sql.NullFloat64
		var selectedSpeciesName sql.NullString
		var resolvedAtRaw sql.NullString
		var createdAtRaw string
		var lockedRaw int

		if err := rows.Scan(
			&reading.ID,
			&reading.JobID,
			&reading.UploadID,
			&reading.SessionID,
			&reading.CP,
			&reading.HP,
			&reading.IVAttack,
			&reading.IVDefense,
			&reading.IVStamina,
			&levelEstimate,
			&levelConfidence,
			&reading.LevelMethod,
			&reading.SourceType,
			&frameTimestampMS,
			&extractionConfidence,
			&reading.Status,
			&lockedRaw,
			&selectedSpeciesName,
			&resolvedAtRaw,
			&createdAtRaw,
		); err != nil {
			return nil, fmt.Errorf("scan pending reading row: %w", err)
		}

		createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse pending reading created_at %q: %w", createdAtRaw, err)
		}

		reading.LevelEstimate = nullableFloat64(levelEstimate)
		reading.LevelConfidence = nullableFloat64(levelConfidence)
		reading.FrameTimestampMS = nullableInt64(frameTimestampMS)
		reading.ExtractionConfidence = nullableFloat64(extractionConfidence)
		reading.SelectedSpeciesName = nullableString(selectedSpeciesName)
		reading.Locked = lockedRaw == 1
		reading.CreatedAt = createdAt

		if resolvedAtRaw.Valid {
			resolvedAt, err := time.Parse(time.RFC3339Nano, resolvedAtRaw.String)
			if err != nil {
				return nil, fmt.Errorf("parse pending reading resolved_at %q: %w", resolvedAtRaw.String, err)
			}
			reading.ResolvedAt = &resolvedAt
		}

		readings = append(readings, reading)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending readings rows: %w", err)
	}

	for i := range readings {
		options, err := s.listPendingOptions(ctx, readings[i].ID)
		if err != nil {
			return nil, err
		}
		readings[i].Options = options
	}

	return readings, nil
}

func (s *sqliteStore) listPendingOptions(ctx context.Context, pendingReadingID string) ([]PendingSpeciesOptionRecord, error) {
	const queryPendingOptions = `
SELECT id, pending_reading_id, species_name, species_name_normalized, match_mode, match_distance, option_rank, created_at
FROM appraisal_pending_species_options
WHERE pending_reading_id = ?
ORDER BY option_rank ASC, id ASC;`

	rows, err := s.db.QueryContext(ctx, queryPendingOptions, pendingReadingID)
	if err != nil {
		return nil, fmt.Errorf("query pending options by reading: %w", err)
	}
	defer rows.Close()

	options := make([]PendingSpeciesOptionRecord, 0)
	for rows.Next() {
		var option PendingSpeciesOptionRecord
		var createdAtRaw string

		if err := rows.Scan(
			&option.ID,
			&option.PendingReadingID,
			&option.SpeciesName,
			&option.SpeciesNameNormalized,
			&option.MatchMode,
			&option.MatchDistance,
			&option.OptionRank,
			&createdAtRaw,
		); err != nil {
			return nil, fmt.Errorf("scan pending option row: %w", err)
		}

		createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("parse pending option created_at %q: %w", createdAtRaw, err)
		}
		option.CreatedAt = createdAt

		options = append(options, option)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending option rows: %w", err)
	}

	return options, nil
}

// ResolvePendingReading finalizes one pending reading into an immutable appraisal result row.
func (s *sqliteStore) ResolvePendingReading(ctx context.Context, params ResolvePendingReadingParams) (PokemonResultRecord, error) {
	readingID := params.ReadingID
	optionID := params.OptionID
	sessionID := params.SessionID
	if readingID == "" {
		return PokemonResultRecord{}, fmt.Errorf("readingID is required")
	}
	if optionID == "" {
		return PokemonResultRecord{}, fmt.Errorf("optionID is required")
	}
	if sessionID == "" {
		return PokemonResultRecord{}, fmt.Errorf("sessionID is required")
	}

	now := params.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowRaw := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("begin resolve pending tx: %w", err)
	}
	defer tx.Rollback()

	const queryPendingReading = `
SELECT r.id, r.job_id, r.upload_id, r.session_id, r.cp, r.hp, r.iv_attack, r.iv_defense, r.iv_stamina,
       r.level_estimate, r.level_confidence, r.level_method, r.source_type, r.frame_timestamp_ms,
       r.extraction_confidence, r.status, r.locked, r.selected_species_name, r.resolved_at, r.created_at,
       j.status
FROM appraisal_pending_readings r
JOIN jobs j ON j.id = r.job_id
WHERE r.id = ? AND r.session_id = ?;`

	var reading PendingSpeciesReadingRecord
	var levelEstimate sql.NullFloat64
	var levelConfidence sql.NullFloat64
	var frameTimestampMS sql.NullInt64
	var extractionConfidence sql.NullFloat64
	var selectedSpeciesName sql.NullString
	var resolvedAtRaw sql.NullString
	var createdAtRaw string
	var jobStatus string
	var lockedRaw int

	if err := tx.QueryRowContext(ctx, queryPendingReading, readingID, sessionID).Scan(
		&reading.ID,
		&reading.JobID,
		&reading.UploadID,
		&reading.SessionID,
		&reading.CP,
		&reading.HP,
		&reading.IVAttack,
		&reading.IVDefense,
		&reading.IVStamina,
		&levelEstimate,
		&levelConfidence,
		&reading.LevelMethod,
		&reading.SourceType,
		&frameTimestampMS,
		&extractionConfidence,
		&reading.Status,
		&lockedRaw,
		&selectedSpeciesName,
		&resolvedAtRaw,
		&createdAtRaw,
		&jobStatus,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PokemonResultRecord{}, ErrPendingReadingNotFound
		}

		return PokemonResultRecord{}, fmt.Errorf("query pending reading by session: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("parse pending reading created_at %q: %w", createdAtRaw, err)
	}
	reading.CreatedAt = createdAt
	reading.LevelEstimate = nullableFloat64(levelEstimate)
	reading.LevelConfidence = nullableFloat64(levelConfidence)
	reading.FrameTimestampMS = nullableInt64(frameTimestampMS)
	reading.ExtractionConfidence = nullableFloat64(extractionConfidence)
	reading.SelectedSpeciesName = nullableString(selectedSpeciesName)
	reading.Locked = lockedRaw == 1
	if resolvedAtRaw.Valid {
		resolvedAt, err := time.Parse(time.RFC3339Nano, resolvedAtRaw.String)
		if err != nil {
			return PokemonResultRecord{}, fmt.Errorf("parse pending reading resolved_at %q: %w", resolvedAtRaw.String, err)
		}
		reading.ResolvedAt = &resolvedAt
	}

	if reading.Locked || reading.Status != JobStatusPendingUserDedup || jobStatus != JobStatusPendingUserDedup {
		return PokemonResultRecord{}, ErrPendingReadingLocked
	}

	const querySelectedOption = `
SELECT species_name
FROM appraisal_pending_species_options
WHERE id = ? AND pending_reading_id = ?;`

	var selectedSpeciesNameValue string
	if err := tx.QueryRowContext(ctx, querySelectedOption, optionID, readingID).Scan(&selectedSpeciesNameValue); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PokemonResultRecord{}, ErrPendingOptionNotFound
		}

		return PokemonResultRecord{}, fmt.Errorf("query selected pending option: %w", err)
	}

	const updatePendingReading = `
UPDATE appraisal_pending_readings
SET status = ?, locked = 1, selected_species_name = ?, resolved_at = ?
WHERE id = ? AND session_id = ? AND status = ? AND locked = 0;`

	result, err := tx.ExecContext(
		ctx,
		updatePendingReading,
		"RESOLVED",
		selectedSpeciesNameValue,
		nowRaw,
		readingID,
		sessionID,
		JobStatusPendingUserDedup,
	)
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("update pending reading lock state: %w", err)
	}

	updatedRows, err := result.RowsAffected()
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("read pending reading rows affected: %w", err)
	}
	if updatedRows != 1 {
		return PokemonResultRecord{}, ErrPendingReadingLocked
	}

	const updateJobStatus = `
UPDATE jobs
SET status = ?, progress = 100, stage = NULL, error_code = NULL, error_message = NULL, updated_at = ?, finished_at = ?
WHERE id = ? AND session_id = ? AND status = ?;`

	result, err = tx.ExecContext(
		ctx,
		updateJobStatus,
		JobStatusSucceeded,
		nowRaw,
		nowRaw,
		reading.JobID,
		sessionID,
		JobStatusPendingUserDedup,
	)
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("update job status to succeeded from pending: %w", err)
	}

	updatedRows, err = result.RowsAffected()
	if err != nil {
		return PokemonResultRecord{}, fmt.Errorf("read job rows affected: %w", err)
	}
	if updatedRows != 1 {
		return PokemonResultRecord{}, ErrPendingReadingLocked
	}

	resultID, err := newID()
	if err != nil {
		return PokemonResultRecord{}, err
	}

	const insertResultQuery = `
INSERT INTO appraisal_results(
	id, job_id, upload_id, session_id, species_name, cp, hp, power_up_stardust_cost,
	iv_attack, iv_defense, iv_stamina, level_estimate, level_confidence, level_method,
	source_type, start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.ExecContext(
		ctx,
		insertResultQuery,
		resultID,
		reading.JobID,
		reading.UploadID,
		reading.SessionID,
		selectedSpeciesNameValue,
		reading.CP,
		reading.HP,
		0,
		reading.IVAttack,
		reading.IVDefense,
		reading.IVStamina,
		nullableFloat64Value(reading.LevelEstimate),
		nullableFloat64Value(reading.LevelConfidence),
		reading.LevelMethod,
		reading.SourceType,
		nil,
		nil,
		nullableInt64Value(reading.FrameTimestampMS),
		nullableFloat64Value(reading.ExtractionConfidence),
		nowRaw,
	); err != nil {
		return PokemonResultRecord{}, fmt.Errorf("insert resolved appraisal result: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return PokemonResultRecord{}, fmt.Errorf("commit resolve pending tx: %w", err)
	}

	return PokemonResultRecord{
		ID:                   resultID,
		JobID:                reading.JobID,
		UploadID:             reading.UploadID,
		SessionID:            reading.SessionID,
		SpeciesName:          selectedSpeciesNameValue,
		CP:                   reading.CP,
		HP:                   reading.HP,
		PowerUpStardustCost:  0,
		IVAttack:             reading.IVAttack,
		IVDefense:            reading.IVDefense,
		IVStamina:            reading.IVStamina,
		LevelEstimate:        reading.LevelEstimate,
		LevelConfidence:      reading.LevelConfidence,
		LevelMethod:          reading.LevelMethod,
		SourceType:           reading.SourceType,
		StartMS:              nil,
		EndMS:                nil,
		FrameTimestampMS:     reading.FrameTimestampMS,
		ExtractionConfidence: reading.ExtractionConfidence,
		CreatedAt:            now,
	}, nil
}

func nullableInt64Value(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat64Value(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	copy := value.String
	return &copy
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	copy := value.Int64
	return &copy
}

func nullableFloat64(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}

	copy := value.Float64
	return &copy
}
