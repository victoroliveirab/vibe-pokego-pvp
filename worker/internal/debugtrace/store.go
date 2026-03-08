package debugtrace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

// Store persists worker debug telemetry for job and frame processing.
type Store interface {
	Close() error
	UpsertJobDebug(ctx context.Context, params UpsertJobDebugParams) error
	UpdateJobDebugMilestone(ctx context.Context, params UpdateJobDebugMilestoneParams) (bool, error)
	InsertFrameDebug(ctx context.Context, params InsertFrameDebugParams) (FrameDebug, error)
	MarkJobDebugTerminal(ctx context.Context, params MarkJobDebugTerminalParams) (bool, error)
}

type sqliteStore struct {
	db *sql.DB
}

type milestoneColumn struct {
	finishedAtColumn string
	metaColumn       string
}

var milestoneColumnsByName = map[string]milestoneColumn{
	MilestoneDownloading: {
		finishedAtColumn: "downloading_finished_at",
		metaColumn:       "download_meta_json",
	},
	MilestoneDecoding: {
		finishedAtColumn: "decoding_finished_at",
		metaColumn:       "decode_meta_json",
	},
	MilestoneSampling: {
		finishedAtColumn: "sampling_finished_at",
		metaColumn:       "sampling_meta_json",
	},
	MilestoneExtracting: {
		finishedAtColumn: "extracting_finished_at",
	},
	MilestonePostprocessing: {
		finishedAtColumn: "postprocessing_finished_at",
		metaColumn:       "postprocessing_meta_json",
	},
	MilestonePersisting: {
		finishedAtColumn: "persisting_finished_at",
		metaColumn:       "persisting_meta_json",
	},
	MilestoneProcessing: {
		finishedAtColumn: "processing_finished_at",
	},
	MilestoneSpecies: {
		finishedAtColumn: "species_finished_at",
	},
	MilestoneCP: {
		finishedAtColumn: "cp_finished_at",
	},
	MilestoneHP: {
		finishedAtColumn: "hp_finished_at",
	},
	MilestoneIV: {
		finishedAtColumn: "iv_finished_at",
	},
}

// NewSQLiteStore opens a SQLite-backed debugtrace store.
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

func (s *sqliteStore) UpsertJobDebug(ctx context.Context, params UpsertJobDebugParams) error {
	if params.JobID == "" {
		return fmt.Errorf("jobID is required")
	}
	if params.UploadID == "" {
		return fmt.Errorf("uploadID is required")
	}
	if params.SessionID == "" {
		return fmt.Errorf("sessionID is required")
	}
	if params.Kind == "" {
		return fmt.Errorf("kind is required")
	}

	processingStartedAt := normalizeNow(params.ProcessingStartedAt)
	createdAt := normalizeNow(params.CreatedAt)
	updatedAt := normalizeNow(params.UpdatedAt)

	const query = `
INSERT INTO job_debug_jobs(
	job_id, upload_id, session_id, kind, processing_started_at,
	downloading_finished_at, decoding_finished_at, sampling_finished_at, extracting_finished_at,
	postprocessing_finished_at, persisting_finished_at, processing_finished_at, species_finished_at,
	cp_finished_at, hp_finished_at, iv_finished_at, download_meta_json, decode_meta_json,
	sampling_meta_json, postprocessing_meta_json, persisting_meta_json, terminal_meta_json,
	error_code, error_message, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(job_id) DO UPDATE SET
	upload_id = excluded.upload_id,
	session_id = excluded.session_id,
	kind = excluded.kind,
	processing_started_at = excluded.processing_started_at,
	downloading_finished_at = COALESCE(excluded.downloading_finished_at, job_debug_jobs.downloading_finished_at),
	decoding_finished_at = COALESCE(excluded.decoding_finished_at, job_debug_jobs.decoding_finished_at),
	sampling_finished_at = COALESCE(excluded.sampling_finished_at, job_debug_jobs.sampling_finished_at),
	extracting_finished_at = COALESCE(excluded.extracting_finished_at, job_debug_jobs.extracting_finished_at),
	postprocessing_finished_at = COALESCE(excluded.postprocessing_finished_at, job_debug_jobs.postprocessing_finished_at),
	persisting_finished_at = COALESCE(excluded.persisting_finished_at, job_debug_jobs.persisting_finished_at),
	processing_finished_at = COALESCE(excluded.processing_finished_at, job_debug_jobs.processing_finished_at),
	species_finished_at = COALESCE(excluded.species_finished_at, job_debug_jobs.species_finished_at),
	cp_finished_at = COALESCE(excluded.cp_finished_at, job_debug_jobs.cp_finished_at),
	hp_finished_at = COALESCE(excluded.hp_finished_at, job_debug_jobs.hp_finished_at),
	iv_finished_at = COALESCE(excluded.iv_finished_at, job_debug_jobs.iv_finished_at),
	download_meta_json = COALESCE(excluded.download_meta_json, job_debug_jobs.download_meta_json),
	decode_meta_json = COALESCE(excluded.decode_meta_json, job_debug_jobs.decode_meta_json),
	sampling_meta_json = COALESCE(excluded.sampling_meta_json, job_debug_jobs.sampling_meta_json),
	postprocessing_meta_json = COALESCE(excluded.postprocessing_meta_json, job_debug_jobs.postprocessing_meta_json),
	persisting_meta_json = COALESCE(excluded.persisting_meta_json, job_debug_jobs.persisting_meta_json),
	terminal_meta_json = COALESCE(excluded.terminal_meta_json, job_debug_jobs.terminal_meta_json),
	error_code = COALESCE(excluded.error_code, job_debug_jobs.error_code),
	error_message = COALESCE(excluded.error_message, job_debug_jobs.error_message),
	updated_at = excluded.updated_at;`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		params.JobID,
		params.UploadID,
		params.SessionID,
		params.Kind,
		processingStartedAt.Format(time.RFC3339Nano),
		nullableTimestamp(params.DownloadingFinishedAt),
		nullableTimestamp(params.DecodingFinishedAt),
		nullableTimestamp(params.SamplingFinishedAt),
		nullableTimestamp(params.ExtractingFinishedAt),
		nullableTimestamp(params.PostprocessingFinishedAt),
		nullableTimestamp(params.PersistingFinishedAt),
		nullableTimestamp(params.ProcessingFinishedAt),
		nullableTimestamp(params.SpeciesFinishedAt),
		nullableTimestamp(params.CPFinishedAt),
		nullableTimestamp(params.HPFinishedAt),
		nullableTimestamp(params.IVFinishedAt),
		nullableString(params.DownloadMetaJSON),
		nullableString(params.DecodeMetaJSON),
		nullableString(params.SamplingMetaJSON),
		nullableString(params.PostprocessingMetaJSON),
		nullableString(params.PersistingMetaJSON),
		nullableString(params.TerminalMetaJSON),
		nullableString(params.ErrorCode),
		nullableString(params.ErrorMessage),
		createdAt.Format(time.RFC3339Nano),
		updatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("upsert job debug row: %w", err)
	}

	return nil
}

func (s *sqliteStore) UpdateJobDebugMilestone(
	ctx context.Context,
	params UpdateJobDebugMilestoneParams,
) (bool, error) {
	if params.JobID == "" {
		return false, fmt.Errorf("jobID is required")
	}

	columns, ok := milestoneColumnsByName[params.Milestone]
	if !ok {
		return false, fmt.Errorf("unsupported milestone %q", params.Milestone)
	}
	if columns.metaColumn == "" && params.MetaJSON != nil {
		return false, fmt.Errorf("milestone %q does not support metadata", params.Milestone)
	}

	updatedAt := normalizeNow(params.UpdatedAt)
	finishedAt := normalizeNow(params.FinishedAt)
	if params.FinishedAt.IsZero() {
		finishedAt = updatedAt
	}

	if columns.metaColumn == "" {
		query := fmt.Sprintf(
			"UPDATE job_debug_jobs SET %s = ?, updated_at = ? WHERE job_id = ?;",
			columns.finishedAtColumn,
		)
		return rowsAffectedBool(s.db.ExecContext(
			ctx,
			query,
			finishedAt.Format(time.RFC3339Nano),
			updatedAt.Format(time.RFC3339Nano),
			params.JobID,
		))
	}

	query := fmt.Sprintf(
		"UPDATE job_debug_jobs SET %s = ?, %s = COALESCE(?, %s), updated_at = ? WHERE job_id = ?;",
		columns.finishedAtColumn,
		columns.metaColumn,
		columns.metaColumn,
	)
	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		query,
		finishedAt.Format(time.RFC3339Nano),
		nullableString(params.MetaJSON),
		updatedAt.Format(time.RFC3339Nano),
		params.JobID,
	))
}

func (s *sqliteStore) InsertFrameDebug(ctx context.Context, params InsertFrameDebugParams) (FrameDebug, error) {
	if params.JobID == "" {
		return FrameDebug{}, fmt.Errorf("jobID is required")
	}
	if params.UploadID == "" {
		return FrameDebug{}, fmt.Errorf("uploadID is required")
	}
	if params.SessionID == "" {
		return FrameDebug{}, fmt.Errorf("sessionID is required")
	}
	if params.SourceType == "" {
		return FrameDebug{}, fmt.Errorf("sourceType is required")
	}
	if params.FrameIndex <= 0 {
		return FrameDebug{}, fmt.Errorf("frameIndex must be greater than zero")
	}
	if params.FrameStatus == "" {
		return FrameDebug{}, fmt.Errorf("frameStatus is required")
	}
	if params.FrameDurationMS < 0 {
		return FrameDebug{}, fmt.Errorf("frameDurationMS must be greater than or equal to zero")
	}

	id := params.ID
	if id == "" {
		generatedID, err := newID()
		if err != nil {
			return FrameDebug{}, err
		}
		id = generatedID
	}

	createdAt := normalizeNow(params.CreatedAt)
	frameFinishedAt := normalizeNow(params.FrameFinishedAt)
	if params.FrameFinishedAt.IsZero() {
		frameFinishedAt = createdAt
	}

	const query = `
INSERT INTO job_debug_frames(
	id, job_id, upload_id, session_id, source_type, frame_index, frame_timestamp_ms, frame_status,
	frame_started_at, frame_finished_at, frame_duration_ms, species_finished_at, cp_finished_at,
	hp_finished_at, iv_finished_at, layout_meta_json, species_meta_json, cp_meta_json, hp_meta_json,
	iv_meta_json, iv_bar_measurement_meta_json, frame_stability_meta_json, selection_meta_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		id,
		params.JobID,
		params.UploadID,
		params.SessionID,
		params.SourceType,
		params.FrameIndex,
		nullableInt64(params.FrameTimestampMS),
		params.FrameStatus,
		nullableTimestamp(params.FrameStartedAt),
		frameFinishedAt.Format(time.RFC3339Nano),
		params.FrameDurationMS,
		nullableTimestamp(params.SpeciesFinishedAt),
		nullableTimestamp(params.CPFinishedAt),
		nullableTimestamp(params.HPFinishedAt),
		nullableTimestamp(params.IVFinishedAt),
		nullableString(params.LayoutMetaJSON),
		nullableString(params.SpeciesMetaJSON),
		nullableString(params.CPMetaJSON),
		nullableString(params.HPMetaJSON),
		nullableString(params.IVMetaJSON),
		nullableString(params.IVBarMeasurementMetaJSON),
		nullableString(params.FrameStabilityMetaJSON),
		nullableString(params.SelectionMetaJSON),
		createdAt.Format(time.RFC3339Nano),
	); err != nil {
		return FrameDebug{}, fmt.Errorf("insert frame debug row: %w", err)
	}

	return FrameDebug{
		ID:                       id,
		JobID:                    params.JobID,
		UploadID:                 params.UploadID,
		SessionID:                params.SessionID,
		SourceType:               params.SourceType,
		FrameIndex:               params.FrameIndex,
		FrameTimestampMS:         params.FrameTimestampMS,
		FrameStatus:              params.FrameStatus,
		FrameStartedAt:           params.FrameStartedAt,
		FrameFinishedAt:          frameFinishedAt,
		FrameDurationMS:          params.FrameDurationMS,
		SpeciesFinishedAt:        params.SpeciesFinishedAt,
		CPFinishedAt:             params.CPFinishedAt,
		HPFinishedAt:             params.HPFinishedAt,
		IVFinishedAt:             params.IVFinishedAt,
		LayoutMetaJSON:           params.LayoutMetaJSON,
		SpeciesMetaJSON:          params.SpeciesMetaJSON,
		CPMetaJSON:               params.CPMetaJSON,
		HPMetaJSON:               params.HPMetaJSON,
		IVMetaJSON:               params.IVMetaJSON,
		IVBarMeasurementMetaJSON: params.IVBarMeasurementMetaJSON,
		FrameStabilityMetaJSON:   params.FrameStabilityMetaJSON,
		SelectionMetaJSON:        params.SelectionMetaJSON,
		CreatedAt:                createdAt,
	}, nil
}

func (s *sqliteStore) MarkJobDebugTerminal(
	ctx context.Context,
	params MarkJobDebugTerminalParams,
) (bool, error) {
	if params.JobID == "" {
		return false, fmt.Errorf("jobID is required")
	}

	updatedAt := normalizeNow(params.UpdatedAt)
	processingFinishedAt := normalizeNow(params.ProcessingFinishedAt)
	if params.ProcessingFinishedAt.IsZero() {
		processingFinishedAt = updatedAt
	}

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE job_debug_jobs
SET processing_finished_at = ?, terminal_meta_json = ?, error_code = ?, error_message = ?, updated_at = ?
WHERE job_id = ?;`,
		processingFinishedAt.Format(time.RFC3339Nano),
		nullableString(params.TerminalMetaJSON),
		nullableString(params.ErrorCode),
		nullableString(params.ErrorMessage),
		updatedAt.Format(time.RFC3339Nano),
		params.JobID,
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

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTimestamp(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.UTC().Format(time.RFC3339Nano)
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
