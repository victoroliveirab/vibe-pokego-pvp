package appraisal

import (
	"context"
	"database/sql"
	"errors"
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
	GetResultByID(ctx context.Context, resultID string) (Result, bool, error)
	InsertPendingReadingWithOptions(ctx context.Context, params InsertPendingReadingWithOptionsParams) (string, error)
	EnqueueResultForPvPEvaluation(ctx context.Context, appraisalResultID string, now time.Time) error
	ClaimPvPEvaluationQueueItems(ctx context.Context, limit int, now time.Time) ([]PvPEvaluationQueueItem, error)
	UpsertResultPvPEvaluations(
		ctx context.Context,
		appraisalResultID string,
		evaluations []UpsertPvPEvaluationParams,
		now time.Time,
	) error
	MarkPvPEvaluationQueueItemSucceeded(ctx context.Context, queueItemID string, now time.Time) (bool, error)
	MarkPvPEvaluationQueueItemFailed(
		ctx context.Context,
		queueItemID string,
		retryCount int,
		lastError string,
		nextRetryAt *time.Time,
		now time.Time,
	) (bool, error)
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

func (s *sqliteStore) GetResultByID(ctx context.Context, resultID string) (Result, bool, error) {
	if strings.TrimSpace(resultID) == "" {
		return Result{}, false, fmt.Errorf("resultID is required")
	}

	const query = `
SELECT id, job_id, upload_id, session_id, species_name, cp, hp, power_up_stardust_cost,
       iv_attack, iv_defense, iv_stamina, level_estimate, level_confidence, level_method,
       source_type, start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
FROM appraisal_results
WHERE id = ?;`

	var row Result
	var levelEstimate sql.NullFloat64
	var levelConfidence sql.NullFloat64
	var startMS sql.NullInt64
	var endMS sql.NullInt64
	var frameTimestampMS sql.NullInt64
	var extractionConfidence sql.NullFloat64
	var createdAtRaw string
	if err := s.db.QueryRowContext(ctx, query, resultID).Scan(
		&row.ID,
		&row.JobID,
		&row.UploadID,
		&row.SessionID,
		&row.SpeciesName,
		&row.CP,
		&row.HP,
		&row.PowerUpStardustCost,
		&row.IVAttack,
		&row.IVDefense,
		&row.IVStamina,
		&levelEstimate,
		&levelConfidence,
		&row.LevelMethod,
		&row.SourceType,
		&startMS,
		&endMS,
		&frameTimestampMS,
		&extractionConfidence,
		&createdAtRaw,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Result{}, false, nil
		}
		return Result{}, false, fmt.Errorf("query appraisal result by id: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return Result{}, false, fmt.Errorf("parse appraisal result created_at %q: %w", createdAtRaw, err)
	}

	row.LevelEstimate = nullableFloat64FromSQL(levelEstimate)
	row.LevelConfidence = nullableFloat64FromSQL(levelConfidence)
	row.StartMS = nullableInt64FromSQL(startMS)
	row.EndMS = nullableInt64FromSQL(endMS)
	row.FrameTimestampMS = nullableInt64FromSQL(frameTimestampMS)
	row.ExtractionConfidence = nullableFloat64FromSQL(extractionConfidence)
	row.CreatedAt = createdAt

	return row, true, nil
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

func (s *sqliteStore) EnqueueResultForPvPEvaluation(
	ctx context.Context,
	appraisalResultID string,
	now time.Time,
) error {
	if appraisalResultID == "" {
		return fmt.Errorf("appraisalResultID is required")
	}

	queueID, err := newID()
	if err != nil {
		return err
	}

	createdAt := normalizeNow(now)
	timestamp := createdAt.Format(time.RFC3339Nano)

	const query = `
INSERT OR IGNORE INTO appraisal_result_pvp_eval_queue(
	id, appraisal_result_id, status, retry_count, last_error, locked, next_retry_at, created_at, updated_at
) VALUES (?, ?, ?, 0, NULL, 0, NULL, ?, ?);`

	if _, err := s.db.ExecContext(
		ctx,
		query,
		queueID,
		appraisalResultID,
		PvPEvalQueueStatusPending,
		timestamp,
		timestamp,
	); err != nil {
		return fmt.Errorf("enqueue appraisal result pvp evaluation: %w", err)
	}

	return nil
}

func (s *sqliteStore) ClaimPvPEvaluationQueueItems(
	ctx context.Context,
	limit int,
	now time.Time,
) ([]PvPEvaluationQueueItem, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}

	claimTime := normalizeNow(now)
	claimTimestamp := claimTime.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin pvp eval claim tx: %w", err)
	}
	defer tx.Rollback()

	const selectQuery = `
SELECT id, appraisal_result_id, status, retry_count, last_error, locked, next_retry_at, created_at, updated_at
FROM appraisal_result_pvp_eval_queue
WHERE locked = 0
  AND status IN (?, ?)
  AND (next_retry_at IS NULL OR next_retry_at <= ?)
ORDER BY created_at ASC, id ASC
LIMIT ?;`

	rows, err := tx.QueryContext(
		ctx,
		selectQuery,
		PvPEvalQueueStatusPending,
		PvPEvalQueueStatusFailed,
		claimTimestamp,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query pvp eval queue claim candidates: %w", err)
	}

	candidates := make([]PvPEvaluationQueueItem, 0, limit)
	for rows.Next() {
		var item PvPEvaluationQueueItem
		var lastError sql.NullString
		var lockedRaw int
		var nextRetryAtRaw sql.NullString
		var createdAtRaw string
		var updatedAtRaw string

		if err := rows.Scan(
			&item.ID,
			&item.AppraisalResultID,
			&item.Status,
			&item.RetryCount,
			&lastError,
			&lockedRaw,
			&nextRetryAtRaw,
			&createdAtRaw,
			&updatedAtRaw,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan pvp eval queue claim candidate: %w", err)
		}

		createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("parse pvp eval queue created_at %q: %w", createdAtRaw, err)
		}
		updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtRaw)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("parse pvp eval queue updated_at %q: %w", updatedAtRaw, err)
		}

		item.CreatedAt = createdAt
		item.UpdatedAt = updatedAt
		item.Locked = lockedRaw == 1

		if lastError.Valid {
			value := lastError.String
			item.LastError = &value
		}
		if nextRetryAtRaw.Valid {
			nextRetryAt, err := time.Parse(time.RFC3339Nano, nextRetryAtRaw.String)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("parse pvp eval queue next_retry_at %q: %w", nextRetryAtRaw.String, err)
			}
			item.NextRetryAt = &nextRetryAt
		}

		candidates = append(candidates, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate pvp eval queue claim candidates: %w", err)
	}
	rows.Close()

	const updateClaimQuery = `
UPDATE appraisal_result_pvp_eval_queue
SET status = ?, locked = 1, last_error = NULL, updated_at = ?
WHERE id = ? AND locked = 0 AND status IN (?, ?);`

	claimed := make([]PvPEvaluationQueueItem, 0, len(candidates))
	for _, candidate := range candidates {
		updated, err := rowsAffectedBool(tx.ExecContext(
			ctx,
			updateClaimQuery,
			PvPEvalQueueStatusProcessing,
			claimTimestamp,
			candidate.ID,
			PvPEvalQueueStatusPending,
			PvPEvalQueueStatusFailed,
		))
		if err != nil {
			return nil, fmt.Errorf("claim pvp eval queue item: %w", err)
		}
		if !updated {
			continue
		}

		candidate.Status = PvPEvalQueueStatusProcessing
		candidate.Locked = true
		candidate.LastError = nil
		candidate.UpdatedAt = claimTime
		claimed = append(claimed, candidate)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit pvp eval claim tx: %w", err)
	}

	return claimed, nil
}

func (s *sqliteStore) UpsertResultPvPEvaluations(
	ctx context.Context,
	appraisalResultID string,
	evaluations []UpsertPvPEvaluationParams,
	now time.Time,
) error {
	if appraisalResultID == "" {
		return fmt.Errorf("appraisalResultID is required")
	}
	if len(evaluations) == 0 {
		return fmt.Errorf("evaluations are required")
	}

	defaultCreatedAt := normalizeNow(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin pvp evaluation upsert tx: %w", err)
	}
	defer tx.Rollback()

	const upsertQuery = `
INSERT INTO appraisal_result_pvp_evaluations(
	id, appraisal_result_id, max_cp, evaluated_species_id, best_level, best_cp,
	stat_product, rank_position, percentage, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(appraisal_result_id, max_cp, evaluated_species_id) DO UPDATE SET
	best_level = excluded.best_level,
	best_cp = excluded.best_cp,
	stat_product = excluded.stat_product,
	rank_position = excluded.rank_position,
	percentage = excluded.percentage;`

	for _, evaluation := range evaluations {
		if evaluation.MaxCP <= 0 {
			return fmt.Errorf("maxCP must be positive")
		}
		if strings.TrimSpace(evaluation.EvaluatedSpeciesID) == "" {
			return fmt.Errorf("evaluatedSpeciesID is required")
		}
		if evaluation.RankPosition <= 0 {
			return fmt.Errorf("rankPosition must be greater than 0")
		}

		evaluationID := evaluation.ID
		if evaluationID == "" {
			generatedID, err := newID()
			if err != nil {
				return err
			}
			evaluationID = generatedID
		}

		createdAt := normalizeNow(evaluation.CreatedAt)
		if evaluation.CreatedAt.IsZero() {
			createdAt = defaultCreatedAt
		}

		if _, err := tx.ExecContext(
			ctx,
			upsertQuery,
			evaluationID,
			appraisalResultID,
			evaluation.MaxCP,
			strings.TrimSpace(evaluation.EvaluatedSpeciesID),
			evaluation.BestLevel,
			evaluation.BestCP,
			evaluation.StatProduct,
			evaluation.RankPosition,
			evaluation.Percentage,
			createdAt.Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("upsert appraisal result pvp evaluation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit pvp evaluation upsert tx: %w", err)
	}

	return nil
}

func (s *sqliteStore) MarkPvPEvaluationQueueItemSucceeded(
	ctx context.Context,
	queueItemID string,
	now time.Time,
) (bool, error) {
	if queueItemID == "" {
		return false, fmt.Errorf("queueItemID is required")
	}

	timestamp := normalizeNow(now).Format(time.RFC3339Nano)

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE appraisal_result_pvp_eval_queue
SET status = ?, locked = 0, last_error = NULL, next_retry_at = NULL, updated_at = ?
WHERE id = ? AND status = ? AND locked = 1;`,
		PvPEvalQueueStatusSucceeded,
		timestamp,
		queueItemID,
		PvPEvalQueueStatusProcessing,
	))
}

func (s *sqliteStore) MarkPvPEvaluationQueueItemFailed(
	ctx context.Context,
	queueItemID string,
	retryCount int,
	lastError string,
	nextRetryAt *time.Time,
	now time.Time,
) (bool, error) {
	if queueItemID == "" {
		return false, fmt.Errorf("queueItemID is required")
	}
	if retryCount < 0 {
		return false, fmt.Errorf("retryCount must be greater than or equal to 0")
	}

	updatedAt := normalizeNow(now)
	updatedAtRaw := updatedAt.Format(time.RFC3339Nano)

	var lastErrorValue *string
	trimmedError := strings.TrimSpace(lastError)
	if trimmedError != "" {
		lastErrorValue = &trimmedError
	}

	return rowsAffectedBool(s.db.ExecContext(
		ctx,
		`UPDATE appraisal_result_pvp_eval_queue
SET status = ?, retry_count = ?, last_error = ?, locked = 0, next_retry_at = ?, updated_at = ?
WHERE id = ? AND status = ? AND locked = 1;`,
		PvPEvalQueueStatusFailed,
		retryCount,
		nullableString(lastErrorValue),
		nullableTimestamp(nextRetryAt),
		updatedAtRaw,
		queueItemID,
		PvPEvalQueueStatusProcessing,
	))
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

func nullableInt64FromSQL(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}

func nullableFloat64FromSQL(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	copy := value.Float64
	return &copy
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
