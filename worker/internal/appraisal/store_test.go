package appraisal

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInsertCandidatePersistsRowWithProvidedTimestamp(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 18, 0, 0, 0, time.UTC)

	seedUploadAndJob(t, store.db, "upload-candidate-1", "job-candidate-1", "session-candidate-1", now)

	frameTimestampMS := int64(1337)
	speciesRaw := "Pikachu"
	confidence := 0.86

	inserted, err := store.InsertCandidate(ctx, InsertCandidateParams{
		ID:                   "candidate-1",
		JobID:                "job-candidate-1",
		UploadID:             "upload-candidate-1",
		SessionID:            "session-candidate-1",
		SourceType:           SourceTypeImage,
		FrameTimestampMS:     &frameTimestampMS,
		SpeciesNameRaw:       &speciesRaw,
		SpeciesIsCanonical:   true,
		ExtractionConfidence: &confidence,
		CreatedAt:            now,
	})
	if err != nil {
		t.Fatalf("expected candidate insert to succeed, got: %v", err)
	}

	if inserted.ID != "candidate-1" {
		t.Fatalf("expected candidate id %q, got %q", "candidate-1", inserted.ID)
	}
	if !inserted.CreatedAt.Equal(now) {
		t.Fatalf("expected created_at %v, got %v", now, inserted.CreatedAt)
	}

	const query = `
SELECT id, source_type, frame_timestamp_ms, species_name_raw, species_is_canonical, extraction_confidence, created_at
FROM appraisal_candidates
WHERE id = ?;`

	var id string
	var sourceType string
	var frameTimestamp sql.NullInt64
	var persistedSpeciesRaw sql.NullString
	var speciesIsCanonical int
	var extractionConfidence sql.NullFloat64
	var createdAtRaw string
	if err := store.db.QueryRowContext(ctx, query, inserted.ID).Scan(
		&id,
		&sourceType,
		&frameTimestamp,
		&persistedSpeciesRaw,
		&speciesIsCanonical,
		&extractionConfidence,
		&createdAtRaw,
	); err != nil {
		t.Fatalf("expected inserted candidate row, got: %v", err)
	}

	if id != inserted.ID {
		t.Fatalf("expected row id %q, got %q", inserted.ID, id)
	}
	if sourceType != SourceTypeImage {
		t.Fatalf("expected source_type %q, got %q", SourceTypeImage, sourceType)
	}
	if !frameTimestamp.Valid || frameTimestamp.Int64 != frameTimestampMS {
		t.Fatalf("expected frame_timestamp_ms %d, got %#v", frameTimestampMS, frameTimestamp)
	}
	if !persistedSpeciesRaw.Valid || persistedSpeciesRaw.String != speciesRaw {
		t.Fatalf("expected species_name_raw %q, got %#v", speciesRaw, persistedSpeciesRaw)
	}
	if speciesIsCanonical != 1 {
		t.Fatalf("expected species_is_canonical=1, got %d", speciesIsCanonical)
	}
	if !extractionConfidence.Valid || extractionConfidence.Float64 != confidence {
		t.Fatalf("expected extraction_confidence %.2f, got %#v", confidence, extractionConfidence)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		t.Fatalf("expected valid created_at timestamp, got parse error: %v", err)
	}
	if !createdAt.Equal(now) {
		t.Fatalf("expected persisted created_at %v, got %v", now, createdAt)
	}
}

func TestInsertResultPersistsRowAndDefaultsTimestampWhenUnset(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 18, 5, 0, 0, time.UTC)
	seedUploadAndJob(t, store.db, "upload-result-1", "job-result-1", "session-result-1", now)

	levelEstimate := 23.5
	levelConfidence := 0.74
	frameTimestampMS := int64(4500)

	beforeInsert := time.Now().UTC()
	inserted, err := store.InsertResult(ctx, InsertResultParams{
		ID:                   "result-1",
		JobID:                "job-result-1",
		UploadID:             "upload-result-1",
		SessionID:            "session-result-1",
		SpeciesName:          "Machop",
		CP:                   512,
		HP:                   64,
		PowerUpStardustCost:  2500,
		IVAttack:             12,
		IVDefense:            15,
		IVStamina:            13,
		LevelEstimate:        &levelEstimate,
		LevelConfidence:      &levelConfidence,
		LevelMethod:          LevelMethodArcPosition,
		SourceType:           SourceTypeImage,
		FrameTimestampMS:     &frameTimestampMS,
		ExtractionConfidence: &levelConfidence,
	})
	afterInsert := time.Now().UTC()
	if err != nil {
		t.Fatalf("expected result insert to succeed, got: %v", err)
	}

	if inserted.ID != "result-1" {
		t.Fatalf("expected result id %q, got %q", "result-1", inserted.ID)
	}
	if inserted.CreatedAt.Before(beforeInsert) || inserted.CreatedAt.After(afterInsert) {
		t.Fatalf("expected created_at to default to current time between %v and %v, got %v", beforeInsert, afterInsert, inserted.CreatedAt)
	}

	const query = `
SELECT species_name, cp, hp, level_method, source_type, created_at
FROM appraisal_results
WHERE id = ?;`

	var speciesName string
	var cp int
	var hp int
	var levelMethod string
	var sourceType string
	var createdAtRaw string
	if err := store.db.QueryRowContext(ctx, query, inserted.ID).Scan(
		&speciesName,
		&cp,
		&hp,
		&levelMethod,
		&sourceType,
		&createdAtRaw,
	); err != nil {
		t.Fatalf("expected inserted result row, got: %v", err)
	}

	if speciesName != "Machop" {
		t.Fatalf("expected species_name %q, got %q", "Machop", speciesName)
	}
	if cp != 512 || hp != 64 {
		t.Fatalf("expected cp/hp 512/64, got %d/%d", cp, hp)
	}
	if levelMethod != LevelMethodArcPosition {
		t.Fatalf("expected level_method %q, got %q", LevelMethodArcPosition, levelMethod)
	}
	if sourceType != SourceTypeImage {
		t.Fatalf("expected source_type %q, got %q", SourceTypeImage, sourceType)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		t.Fatalf("expected valid created_at timestamp, got parse error: %v", err)
	}
	if createdAt.Before(beforeInsert) || createdAt.After(afterInsert) {
		t.Fatalf("expected persisted created_at between %v and %v, got %v", beforeInsert, afterInsert, createdAt)
	}
}

func TestInsertCandidateCanPersistWithoutAcceptedResultInsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 18, 10, 0, 0, time.UTC)
	seedUploadAndJob(t, store.db, "upload-candidate-only-1", "job-candidate-only-1", "session-candidate-only-1", now)

	_, err := store.InsertCandidate(ctx, InsertCandidateParams{
		ID:         "candidate-only-1",
		JobID:      "job-candidate-only-1",
		UploadID:   "upload-candidate-only-1",
		SessionID:  "session-candidate-only-1",
		SourceType: SourceTypeImage,
		CreatedAt:  now,
	})
	if err != nil {
		t.Fatalf("expected candidate-only insert to succeed, got: %v", err)
	}

	assertRowCount(t, store.db, "appraisal_candidates", 1)
	assertRowCount(t, store.db, "appraisal_results", 0)
}

func TestInsertPendingReadingWithOptionsPersistsRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 2, 18, 15, 0, 0, time.UTC)
	seedUploadAndJob(t, store.db, "upload-pending-1", "job-pending-1", "session-pending-1", now)

	levelEstimate := 18.5
	levelConfidence := 0.66

	pendingReadingID, err := store.InsertPendingReadingWithOptions(ctx, InsertPendingReadingWithOptionsParams{
		PendingReadingID: "pending-reading-1",
		JobID:            "job-pending-1",
		UploadID:         "upload-pending-1",
		SessionID:        "session-pending-1",
		CP:               824,
		HP:               141,
		IVAttack:         10,
		IVDefense:        12,
		IVStamina:        13,
		LevelEstimate:    &levelEstimate,
		LevelConfidence:  &levelConfidence,
		LevelMethod:      LevelMethodUnknown,
		SourceType:       SourceTypeImage,
		Status:           "PENDING_USER_DEDUP",
		Locked:           false,
		Options: []InsertPendingSpeciesOptionParams{
			{
				ID:                    "pending-option-1",
				SpeciesName:           "Mr. Mime",
				SpeciesNameNormalized: "mr. mime",
				MatchMode:             "exact",
				MatchDistance:         0,
				OptionRank:            1,
			},
			{
				ID:                    "pending-option-2",
				SpeciesName:           "Mr. Mime (Galarian)",
				SpeciesNameNormalized: "mr. mime (galarian)",
				MatchMode:             "prefix",
				MatchDistance:         0,
				OptionRank:            2,
			},
		},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("expected pending insert to succeed, got: %v", err)
	}
	if pendingReadingID != "pending-reading-1" {
		t.Fatalf("expected pending reading id %q, got %q", "pending-reading-1", pendingReadingID)
	}

	const readingQuery = `
SELECT status, locked, selected_species_name, resolved_at, created_at
FROM appraisal_pending_readings
WHERE id = ?;`

	var status string
	var locked int
	var selectedSpeciesName sql.NullString
	var resolvedAt sql.NullString
	var createdAtRaw string
	if err := store.db.QueryRowContext(ctx, readingQuery, "pending-reading-1").Scan(
		&status,
		&locked,
		&selectedSpeciesName,
		&resolvedAt,
		&createdAtRaw,
	); err != nil {
		t.Fatalf("expected pending reading row, got: %v", err)
	}
	if status != "PENDING_USER_DEDUP" {
		t.Fatalf("expected status %q, got %q", "PENDING_USER_DEDUP", status)
	}
	if locked != 0 {
		t.Fatalf("expected locked=0, got %d", locked)
	}
	if selectedSpeciesName.Valid {
		t.Fatalf("expected selected species to be NULL, got %#v", selectedSpeciesName)
	}
	if resolvedAt.Valid {
		t.Fatalf("expected resolved_at to be NULL, got %#v", resolvedAt)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		t.Fatalf("expected valid created_at timestamp, got parse error: %v", err)
	}
	if !createdAt.Equal(now) {
		t.Fatalf("expected persisted created_at %v, got %v", now, createdAt)
	}

	const optionsQuery = `
SELECT species_name, species_name_normalized, match_mode, match_distance, option_rank
FROM appraisal_pending_species_options
WHERE pending_reading_id = ?
ORDER BY option_rank ASC;`

	rows, err := store.db.QueryContext(ctx, optionsQuery, "pending-reading-1")
	if err != nil {
		t.Fatalf("expected pending options query to succeed, got: %v", err)
	}
	defer rows.Close()

	type optionRow struct {
		speciesName           string
		speciesNameNormalized string
		matchMode             string
		matchDistance         int
		optionRank            int
	}
	var options []optionRow
	for rows.Next() {
		var option optionRow
		if err := rows.Scan(
			&option.speciesName,
			&option.speciesNameNormalized,
			&option.matchMode,
			&option.matchDistance,
			&option.optionRank,
		); err != nil {
			t.Fatalf("expected pending option scan to succeed, got: %v", err)
		}
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("expected pending options rows to iterate cleanly, got: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected 2 pending options, got %d", len(options))
	}
	if options[0].speciesName != "Mr. Mime" || options[0].optionRank != 1 || options[0].matchMode != "exact" {
		t.Fatalf("expected first option to be exact Mr. Mime rank 1, got %#v", options[0])
	}
	if options[1].speciesName != "Mr. Mime (Galarian)" || options[1].optionRank != 2 || options[1].matchMode != "prefix" {
		t.Fatalf("expected second option to be prefix Mr. Mime (Galarian) rank 2, got %#v", options[1])
	}
}

func newTestStore(t *testing.T) *sqliteStore {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "appraisal.db")
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

func seedUploadAndJob(t *testing.T, db *sql.DB, uploadID string, jobID string, sessionID string, now time.Time) {
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
		t.Fatalf("expected upload seed insert to succeed, got: %v", err)
	}

	const insertJob = `
INSERT INTO jobs(
	id, upload_id, session_id, parent_job_id, status, progress, stage,
	worker_id, claimed_at, heartbeat_at, error_code, error_message,
	created_at, updated_at, finished_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`
	if _, err := db.ExecContext(
		context.Background(),
		insertJob,
		jobID,
		uploadID,
		sessionID,
		nil,
		"PROCESSING",
		0,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		now.UTC().Format(time.RFC3339Nano),
		now.UTC().Format(time.RFC3339Nano),
		nil,
	); err != nil {
		t.Fatalf("expected job seed insert to succeed, got: %v", err)
	}
}

func assertRowCount(t *testing.T, db *sql.DB, table string, expected int) {
	t.Helper()

	var count int
	query := "SELECT COUNT(*) FROM " + table + ";"
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("expected row count query for %s, got: %v", table, err)
	}
	if count != expected {
		t.Fatalf("expected %d rows in %s, got %d", expected, table, count)
	}
}
