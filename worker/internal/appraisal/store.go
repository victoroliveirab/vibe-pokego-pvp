package appraisal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

// Store persists raw appraisal candidates and accepted appraisal results.
type Store interface {
	Close() error
	InsertCandidate(ctx context.Context, params InsertCandidateParams) (Candidate, error)
	InsertResult(ctx context.Context, params InsertResultParams) (Result, error)
	InsertPendingReadingWithOptions(ctx context.Context, params InsertPendingReadingWithOptionsParams) (string, error)
}

type sqliteStore struct {
	db *sql.DB
}

// InsertCandidateParams carries data to persist a raw appraisal candidate row.
type InsertCandidateParams struct {
	ID                    string
	JobID                 string
	UploadID              string
	SessionID             string
	SourceType            string
	FrameTimestampMS      *int64
	SpeciesNameRaw        *string
	SpeciesNameNormalized *string
	SpeciesIsCanonical    bool
	CPRaw                 *string
	HPRaw                 *string
	StardustRaw           *string
	IVAttackRaw           *string
	IVDefenseRaw          *string
	IVStaminaRaw          *string
	ExtractionConfidence  *float64
	RawText               *string
	CreatedAt             time.Time
}

// InsertResultParams carries data to persist an accepted appraisal result row.
type InsertResultParams struct {
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

// InsertPendingReadingWithOptionsParams carries data to persist a pending reading
// and its ranked species option rows.
type InsertPendingReadingWithOptionsParams struct {
	PendingReadingID     string
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
	Options              []InsertPendingSpeciesOptionParams
	CreatedAt            time.Time
}

// InsertPendingSpeciesOptionParams carries one pending species option row.
type InsertPendingSpeciesOptionParams struct {
	ID                    string
	SpeciesName           string
	SpeciesNameNormalized string
	MatchMode             string
	MatchDistance         int
	OptionRank            int
	CreatedAt             time.Time
}

// NewSQLiteStore opens a SQLite-backed appraisal persistence store.
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

func (s *sqliteStore) InsertCandidate(ctx context.Context, params InsertCandidateParams) (Candidate, error) {
	if params.JobID == "" {
		return Candidate{}, fmt.Errorf("jobID is required")
	}
	if params.UploadID == "" {
		return Candidate{}, fmt.Errorf("uploadID is required")
	}
	if params.SessionID == "" {
		return Candidate{}, fmt.Errorf("sessionID is required")
	}
	if params.SourceType == "" {
		return Candidate{}, fmt.Errorf("sourceType is required")
	}

	id := params.ID
	if id == "" {
		generatedID, err := newID()
		if err != nil {
			return Candidate{}, err
		}
		id = generatedID
	}

	createdAt := normalizeNow(params.CreatedAt)
	timestamp := createdAt.Format(time.RFC3339Nano)

	const query = `
INSERT INTO appraisal_candidates(
	id, job_id, upload_id, session_id, source_type, frame_timestamp_ms,
	species_name_raw, species_name_normalized, species_is_canonical,
	cp_raw, hp_raw, stardust_raw, iv_attack_raw, iv_defense_raw, iv_stamina_raw,
	extraction_confidence, raw_text, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		id,
		params.JobID,
		params.UploadID,
		params.SessionID,
		params.SourceType,
		nullableInt64(params.FrameTimestampMS),
		nullableString(params.SpeciesNameRaw),
		nullableString(params.SpeciesNameNormalized),
		boolToInt(params.SpeciesIsCanonical),
		nullableString(params.CPRaw),
		nullableString(params.HPRaw),
		nullableString(params.StardustRaw),
		nullableString(params.IVAttackRaw),
		nullableString(params.IVDefenseRaw),
		nullableString(params.IVStaminaRaw),
		nullableFloat64(params.ExtractionConfidence),
		nullableString(params.RawText),
		timestamp,
	); err != nil {
		return Candidate{}, fmt.Errorf("insert appraisal candidate: %w", err)
	}

	return Candidate{
		ID:                    id,
		JobID:                 params.JobID,
		UploadID:              params.UploadID,
		SessionID:             params.SessionID,
		SourceType:            params.SourceType,
		FrameTimestampMS:      params.FrameTimestampMS,
		SpeciesNameRaw:        params.SpeciesNameRaw,
		SpeciesNameNormalized: params.SpeciesNameNormalized,
		SpeciesIsCanonical:    params.SpeciesIsCanonical,
		CPRaw:                 params.CPRaw,
		HPRaw:                 params.HPRaw,
		StardustRaw:           params.StardustRaw,
		IVAttackRaw:           params.IVAttackRaw,
		IVDefenseRaw:          params.IVDefenseRaw,
		IVStaminaRaw:          params.IVStaminaRaw,
		ExtractionConfidence:  params.ExtractionConfidence,
		RawText:               params.RawText,
		CreatedAt:             createdAt,
	}, nil
}

func (s *sqliteStore) InsertResult(ctx context.Context, params InsertResultParams) (Result, error) {
	if params.JobID == "" {
		return Result{}, fmt.Errorf("jobID is required")
	}
	if params.UploadID == "" {
		return Result{}, fmt.Errorf("uploadID is required")
	}
	if params.SessionID == "" {
		return Result{}, fmt.Errorf("sessionID is required")
	}
	if params.SpeciesName == "" {
		return Result{}, fmt.Errorf("speciesName is required")
	}
	if params.LevelMethod == "" {
		return Result{}, fmt.Errorf("levelMethod is required")
	}
	if params.SourceType == "" {
		return Result{}, fmt.Errorf("sourceType is required")
	}

	id := params.ID
	if id == "" {
		generatedID, err := newID()
		if err != nil {
			return Result{}, err
		}
		id = generatedID
	}

	createdAt := normalizeNow(params.CreatedAt)
	timestamp := createdAt.Format(time.RFC3339Nano)

	const query = `
INSERT INTO appraisal_results(
	id, job_id, upload_id, session_id, species_name, cp, hp, power_up_stardust_cost,
	iv_attack, iv_defense, iv_stamina, level_estimate, level_confidence, level_method,
	source_type, start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		id,
		params.JobID,
		params.UploadID,
		params.SessionID,
		params.SpeciesName,
		params.CP,
		params.HP,
		params.PowerUpStardustCost,
		params.IVAttack,
		params.IVDefense,
		params.IVStamina,
		nullableFloat64(params.LevelEstimate),
		nullableFloat64(params.LevelConfidence),
		params.LevelMethod,
		params.SourceType,
		nullableInt64(params.StartMS),
		nullableInt64(params.EndMS),
		nullableInt64(params.FrameTimestampMS),
		nullableFloat64(params.ExtractionConfidence),
		timestamp,
	); err != nil {
		return Result{}, fmt.Errorf("insert appraisal result: %w", err)
	}

	return Result{
		ID:                   id,
		JobID:                params.JobID,
		UploadID:             params.UploadID,
		SessionID:            params.SessionID,
		SpeciesName:          params.SpeciesName,
		CP:                   params.CP,
		HP:                   params.HP,
		PowerUpStardustCost:  params.PowerUpStardustCost,
		IVAttack:             params.IVAttack,
		IVDefense:            params.IVDefense,
		IVStamina:            params.IVStamina,
		LevelEstimate:        params.LevelEstimate,
		LevelConfidence:      params.LevelConfidence,
		LevelMethod:          params.LevelMethod,
		SourceType:           params.SourceType,
		StartMS:              params.StartMS,
		EndMS:                params.EndMS,
		FrameTimestampMS:     params.FrameTimestampMS,
		ExtractionConfidence: params.ExtractionConfidence,
		CreatedAt:            createdAt,
	}, nil
}

func (s *sqliteStore) InsertPendingReadingWithOptions(
	ctx context.Context,
	params InsertPendingReadingWithOptionsParams,
) (string, error) {
	if params.JobID == "" {
		return "", fmt.Errorf("jobID is required")
	}
	if params.UploadID == "" {
		return "", fmt.Errorf("uploadID is required")
	}
	if params.SessionID == "" {
		return "", fmt.Errorf("sessionID is required")
	}
	if params.LevelMethod == "" {
		return "", fmt.Errorf("levelMethod is required")
	}
	if params.SourceType == "" {
		return "", fmt.Errorf("sourceType is required")
	}
	if params.Status == "" {
		return "", fmt.Errorf("status is required")
	}
	if len(params.Options) == 0 {
		return "", fmt.Errorf("options are required")
	}

	pendingReadingID := params.PendingReadingID
	if pendingReadingID == "" {
		generatedID, err := newID()
		if err != nil {
			return "", err
		}
		pendingReadingID = generatedID
	}

	createdAt := normalizeNow(params.CreatedAt)
	createdTimestamp := createdAt.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin pending insert tx: %w", err)
	}
	defer tx.Rollback()

	const insertPendingReadingQuery = `
INSERT INTO appraisal_pending_readings(
	id, job_id, upload_id, session_id, cp, hp, iv_attack, iv_defense, iv_stamina,
	level_estimate, level_confidence, level_method, source_type, frame_timestamp_ms,
	extraction_confidence, status, locked, selected_species_name, resolved_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := tx.ExecContext(
		ctx,
		insertPendingReadingQuery,
		pendingReadingID,
		params.JobID,
		params.UploadID,
		params.SessionID,
		params.CP,
		params.HP,
		params.IVAttack,
		params.IVDefense,
		params.IVStamina,
		nullableFloat64(params.LevelEstimate),
		nullableFloat64(params.LevelConfidence),
		params.LevelMethod,
		params.SourceType,
		nullableInt64(params.FrameTimestampMS),
		nullableFloat64(params.ExtractionConfidence),
		params.Status,
		boolToInt(params.Locked),
		nullableString(params.SelectedSpeciesName),
		nullableTimestamp(params.ResolvedAt),
		createdTimestamp,
	); err != nil {
		return "", fmt.Errorf("insert appraisal pending reading: %w", err)
	}

	const insertOptionQuery = `
INSERT INTO appraisal_pending_species_options(
	id, pending_reading_id, species_name, species_name_normalized,
	match_mode, match_distance, option_rank, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`

	for _, option := range params.Options {
		if option.SpeciesName == "" {
			return "", fmt.Errorf("speciesName is required")
		}
		if option.SpeciesNameNormalized == "" {
			return "", fmt.Errorf("speciesNameNormalized is required")
		}
		if option.MatchMode == "" {
			return "", fmt.Errorf("matchMode is required")
		}
		if option.OptionRank <= 0 {
			return "", fmt.Errorf("optionRank must be greater than 0")
		}

		optionID := option.ID
		if optionID == "" {
			generatedID, err := newID()
			if err != nil {
				return "", err
			}
			optionID = generatedID
		}

		optionCreatedAt := normalizeNow(option.CreatedAt)
		if option.CreatedAt.IsZero() {
			optionCreatedAt = createdAt
		}

		if _, err := tx.ExecContext(
			ctx,
			insertOptionQuery,
			optionID,
			pendingReadingID,
			option.SpeciesName,
			option.SpeciesNameNormalized,
			option.MatchMode,
			option.MatchDistance,
			option.OptionRank,
			optionCreatedAt.Format(time.RFC3339Nano),
		); err != nil {
			return "", fmt.Errorf("insert appraisal pending species option: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit pending insert tx: %w", err)
	}

	return pendingReadingID, nil
}

func normalizeNow(now time.Time) time.Time {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func nullableFloat64(value *float64) any {
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
