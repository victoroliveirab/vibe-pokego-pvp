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

func TestEnqueueResultForPvPEvaluationCreatesUniquePendingQueueRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 8, 1, 0, 0, 0, time.UTC)

	seedUploadAndJob(t, store.db, "upload-pvp-queue-1", "job-pvp-queue-1", "session-pvp-queue-1", now)
	if _, err := store.InsertResult(ctx, InsertResultParams{
		ID:                  "result-pvp-queue-1",
		JobID:               "job-pvp-queue-1",
		UploadID:            "upload-pvp-queue-1",
		SessionID:           "session-pvp-queue-1",
		SpeciesName:         "Bulbasaur",
		CP:                  800,
		HP:                  80,
		PowerUpStardustCost: 2500,
		IVAttack:            10,
		IVDefense:           10,
		IVStamina:           10,
		LevelMethod:         LevelMethodUnknown,
		SourceType:          SourceTypeImage,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("expected result insert to succeed, got: %v", err)
	}

	if err := store.EnqueueResultForPvPEvaluation(ctx, "result-pvp-queue-1", now); err != nil {
		t.Fatalf("expected enqueue to succeed, got: %v", err)
	}
	if err := store.EnqueueResultForPvPEvaluation(ctx, "result-pvp-queue-1", now.Add(time.Minute)); err != nil {
		t.Fatalf("expected duplicate enqueue to be ignored, got: %v", err)
	}

	assertRowCount(t, store.db, "appraisal_result_pvp_eval_queue", 1)

	const query = `
SELECT status, retry_count, last_error, locked, next_retry_at
FROM appraisal_result_pvp_eval_queue
WHERE appraisal_result_id = ?;`

	var status string
	var retryCount int
	var lastError sql.NullString
	var locked int
	var nextRetryAt sql.NullString
	if err := store.db.QueryRowContext(ctx, query, "result-pvp-queue-1").Scan(
		&status,
		&retryCount,
		&lastError,
		&locked,
		&nextRetryAt,
	); err != nil {
		t.Fatalf("expected queue row query to succeed, got: %v", err)
	}

	if status != PvPEvalQueueStatusPending {
		t.Fatalf("expected queue status %q, got %q", PvPEvalQueueStatusPending, status)
	}
	if retryCount != 0 {
		t.Fatalf("expected retry_count 0, got %d", retryCount)
	}
	if lastError.Valid {
		t.Fatalf("expected last_error NULL, got %#v", lastError)
	}
	if locked != 0 {
		t.Fatalf("expected locked=0, got %d", locked)
	}
	if nextRetryAt.Valid {
		t.Fatalf("expected next_retry_at NULL, got %#v", nextRetryAt)
	}
}

func TestClaimPvPEvaluationQueueItemsClaimsEligibleRowsByLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 8, 2, 0, 0, 0, time.UTC)

	for idx, resultID := range []string{"result-pvp-claim-1", "result-pvp-claim-2", "result-pvp-claim-3"} {
		uploadID := fmt.Sprintf("upload-pvp-claim-%d", idx+1)
		jobID := fmt.Sprintf("job-pvp-claim-%d", idx+1)
		sessionID := fmt.Sprintf("session-pvp-claim-%d", idx+1)
		seedUploadAndJob(t, store.db, uploadID, jobID, sessionID, now)

		if _, err := store.InsertResult(ctx, InsertResultParams{
			ID:                  resultID,
			JobID:               jobID,
			UploadID:            uploadID,
			SessionID:           sessionID,
			SpeciesName:         "Bulbasaur",
			CP:                  700 + idx,
			HP:                  70 + idx,
			PowerUpStardustCost: 2500,
			IVAttack:            10,
			IVDefense:           10,
			IVStamina:           10,
			LevelMethod:         LevelMethodUnknown,
			SourceType:          SourceTypeImage,
			CreatedAt:           now,
		}); err != nil {
			t.Fatalf("expected result insert to succeed, got: %v", err)
		}
		if err := store.EnqueueResultForPvPEvaluation(ctx, resultID, now.Add(time.Duration(idx)*time.Second)); err != nil {
			t.Fatalf("expected enqueue to succeed, got: %v", err)
		}
	}

	futureRetryAt := now.Add(30 * time.Minute).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(
		ctx,
		`UPDATE appraisal_result_pvp_eval_queue
SET status = ?, retry_count = 2, next_retry_at = ?
WHERE appraisal_result_id = ?;`,
		PvPEvalQueueStatusFailed,
		futureRetryAt,
		"result-pvp-claim-3",
	); err != nil {
		t.Fatalf("expected queue update to failed future to succeed, got: %v", err)
	}

	claimed, err := store.ClaimPvPEvaluationQueueItems(ctx, 2, now)
	if err != nil {
		t.Fatalf("expected claim to succeed, got: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("expected 2 claimed items, got %d", len(claimed))
	}

	for _, item := range claimed {
		if item.Status != PvPEvalQueueStatusProcessing {
			t.Fatalf("expected claimed item status %q, got %q", PvPEvalQueueStatusProcessing, item.Status)
		}
		if !item.Locked {
			t.Fatalf("expected claimed item to be locked, got %#v", item)
		}
	}

	const query = `
SELECT status, locked
FROM appraisal_result_pvp_eval_queue
WHERE appraisal_result_id = ?;`

	var status string
	var locked int
	if err := store.db.QueryRowContext(ctx, query, "result-pvp-claim-3").Scan(&status, &locked); err != nil {
		t.Fatalf("expected queue row query for unclaimed item to succeed, got: %v", err)
	}
	if status != PvPEvalQueueStatusFailed {
		t.Fatalf("expected unclaimable row status %q, got %q", PvPEvalQueueStatusFailed, status)
	}
	if locked != 0 {
		t.Fatalf("expected unclaimable row locked=0, got %d", locked)
	}
}

func TestMarkPvPEvaluationQueueItemSucceededAndFailed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 8, 3, 0, 0, 0, time.UTC)

	seedUploadAndJob(t, store.db, "upload-pvp-mark-1", "job-pvp-mark-1", "session-pvp-mark-1", now)
	if _, err := store.InsertResult(ctx, InsertResultParams{
		ID:                  "result-pvp-mark-1",
		JobID:               "job-pvp-mark-1",
		UploadID:            "upload-pvp-mark-1",
		SessionID:           "session-pvp-mark-1",
		SpeciesName:         "Bulbasaur",
		CP:                  720,
		HP:                  80,
		PowerUpStardustCost: 2500,
		IVAttack:            10,
		IVDefense:           10,
		IVStamina:           10,
		LevelMethod:         LevelMethodUnknown,
		SourceType:          SourceTypeImage,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("expected result insert to succeed, got: %v", err)
	}
	if err := store.EnqueueResultForPvPEvaluation(ctx, "result-pvp-mark-1", now); err != nil {
		t.Fatalf("expected enqueue to succeed, got: %v", err)
	}

	claimed, err := store.ClaimPvPEvaluationQueueItems(ctx, 1, now)
	if err != nil {
		t.Fatalf("expected claim to succeed, got: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed queue row, got %d", len(claimed))
	}

	updated, err := store.MarkPvPEvaluationQueueItemSucceeded(ctx, claimed[0].ID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expected mark succeeded to succeed, got: %v", err)
	}
	if !updated {
		t.Fatal("expected mark succeeded to update one row")
	}

	var status string
	var locked int
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT status, locked FROM appraisal_result_pvp_eval_queue WHERE id = ?;`,
		claimed[0].ID,
	).Scan(&status, &locked); err != nil {
		t.Fatalf("expected queue row query to succeed, got: %v", err)
	}
	if status != PvPEvalQueueStatusSucceeded {
		t.Fatalf("expected status %q, got %q", PvPEvalQueueStatusSucceeded, status)
	}
	if locked != 0 {
		t.Fatalf("expected locked=0, got %d", locked)
	}

	seedUploadAndJob(t, store.db, "upload-pvp-mark-2", "job-pvp-mark-2", "session-pvp-mark-2", now)
	if _, err := store.InsertResult(ctx, InsertResultParams{
		ID:                  "result-pvp-mark-2",
		JobID:               "job-pvp-mark-2",
		UploadID:            "upload-pvp-mark-2",
		SessionID:           "session-pvp-mark-2",
		SpeciesName:         "Ivysaur",
		CP:                  1250,
		HP:                  110,
		PowerUpStardustCost: 3000,
		IVAttack:            12,
		IVDefense:           13,
		IVStamina:           14,
		LevelMethod:         LevelMethodUnknown,
		SourceType:          SourceTypeImage,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("expected second result insert to succeed, got: %v", err)
	}
	if err := store.EnqueueResultForPvPEvaluation(ctx, "result-pvp-mark-2", now); err != nil {
		t.Fatalf("expected second enqueue to succeed, got: %v", err)
	}

	claimed, err = store.ClaimPvPEvaluationQueueItems(ctx, 1, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expected second claim to succeed, got: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected one claimed queue row, got %d", len(claimed))
	}

	nextRetry := now.Add(10 * time.Minute)
	updated, err = store.MarkPvPEvaluationQueueItemFailed(
		ctx,
		claimed[0].ID,
		3,
		"temporary failure",
		&nextRetry,
		now.Add(3*time.Minute),
	)
	if err != nil {
		t.Fatalf("expected mark failed to succeed, got: %v", err)
	}
	if !updated {
		t.Fatal("expected mark failed to update one row")
	}

	var retryCount int
	var lastError sql.NullString
	var nextRetryAtRaw sql.NullString
	if err := store.db.QueryRowContext(
		ctx,
		`SELECT status, retry_count, last_error, locked, next_retry_at
FROM appraisal_result_pvp_eval_queue
WHERE id = ?;`,
		claimed[0].ID,
	).Scan(&status, &retryCount, &lastError, &locked, &nextRetryAtRaw); err != nil {
		t.Fatalf("expected failed queue row query to succeed, got: %v", err)
	}
	if status != PvPEvalQueueStatusFailed {
		t.Fatalf("expected failed status %q, got %q", PvPEvalQueueStatusFailed, status)
	}
	if retryCount != 3 {
		t.Fatalf("expected retry_count 3, got %d", retryCount)
	}
	if !lastError.Valid || lastError.String != "temporary failure" {
		t.Fatalf("expected last_error %q, got %#v", "temporary failure", lastError)
	}
	if locked != 0 {
		t.Fatalf("expected locked=0 after failure, got %d", locked)
	}
	if !nextRetryAtRaw.Valid {
		t.Fatal("expected next_retry_at to be set")
	}
}

func TestUpsertResultPvPEvaluationsUpdatesExistingUniqueRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 8, 4, 0, 0, 0, time.UTC)

	seedUploadAndJob(t, store.db, "upload-pvp-upsert-1", "job-pvp-upsert-1", "session-pvp-upsert-1", now)
	if _, err := store.InsertResult(ctx, InsertResultParams{
		ID:                  "result-pvp-upsert-1",
		JobID:               "job-pvp-upsert-1",
		UploadID:            "upload-pvp-upsert-1",
		SessionID:           "session-pvp-upsert-1",
		SpeciesName:         "Ivysaur",
		CP:                  1300,
		HP:                  120,
		PowerUpStardustCost: 3000,
		IVAttack:            11,
		IVDefense:           14,
		IVStamina:           13,
		LevelMethod:         LevelMethodUnknown,
		SourceType:          SourceTypeImage,
		CreatedAt:           now,
	}); err != nil {
		t.Fatalf("expected result insert to succeed, got: %v", err)
	}

	if err := store.UpsertResultPvPEvaluations(ctx, "result-pvp-upsert-1", []UpsertPvPEvaluationParams{
		{
			ID:                 "pvp-eval-1",
			MaxCP:              1500,
			EvaluatedSpeciesID: "ivysaur",
			BestLevel:          36.5,
			BestCP:             1497,
			StatProduct:        1850.12,
			RankPosition:       42,
			Percentage:         97.21,
			CreatedAt:          now,
		},
	}, now); err != nil {
		t.Fatalf("expected first upsert to succeed, got: %v", err)
	}

	if err := store.UpsertResultPvPEvaluations(ctx, "result-pvp-upsert-1", []UpsertPvPEvaluationParams{
		{
			ID:                 "pvp-eval-2",
			MaxCP:              1500,
			EvaluatedSpeciesID: "ivysaur",
			BestLevel:          37.0,
			BestCP:             1499,
			StatProduct:        1862.40,
			RankPosition:       35,
			Percentage:         98.31,
			CreatedAt:          now.Add(time.Minute),
		},
	}, now.Add(time.Minute)); err != nil {
		t.Fatalf("expected conflicting upsert to succeed, got: %v", err)
	}

	assertRowCount(t, store.db, "appraisal_result_pvp_evaluations", 1)

	const query = `
SELECT best_level, best_cp, stat_product, rank_position, percentage
FROM appraisal_result_pvp_evaluations
WHERE appraisal_result_id = ? AND max_cp = ? AND evaluated_species_id = ?;`

	var bestLevel float64
	var bestCP int
	var statProduct float64
	var rankPosition int
	var percentage float64
	if err := store.db.QueryRowContext(ctx, query, "result-pvp-upsert-1", 1500, "ivysaur").Scan(
		&bestLevel,
		&bestCP,
		&statProduct,
		&rankPosition,
		&percentage,
	); err != nil {
		t.Fatalf("expected evaluation row query to succeed, got: %v", err)
	}

	if bestLevel != 37.0 || bestCP != 1499 {
		t.Fatalf("expected updated best level/cp 37.0/1499, got %.1f/%d", bestLevel, bestCP)
	}
	if statProduct != 1862.40 {
		t.Fatalf("expected updated stat_product 1862.40, got %.2f", statProduct)
	}
	if rankPosition != 35 {
		t.Fatalf("expected updated rank_position 35, got %d", rankPosition)
	}
	if percentage != 98.31 {
		t.Fatalf("expected updated percentage 98.31, got %.2f", percentage)
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

CREATE TABLE IF NOT EXISTS appraisal_result_pvp_evaluations (
	id TEXT PRIMARY KEY,
	appraisal_result_id TEXT NOT NULL,
	max_cp INTEGER NOT NULL,
	evaluated_species_id TEXT NOT NULL,
	best_level REAL NOT NULL,
	best_cp INTEGER NOT NULL,
	stat_product REAL NOT NULL,
	rank_position INTEGER NOT NULL,
	percentage REAL NOT NULL,
	created_at TEXT NOT NULL,
	UNIQUE(appraisal_result_id, max_cp, evaluated_species_id),
	FOREIGN KEY(appraisal_result_id) REFERENCES appraisal_results(id)
);

CREATE TABLE IF NOT EXISTS appraisal_result_pvp_eval_queue (
	id TEXT PRIMARY KEY,
	appraisal_result_id TEXT NOT NULL,
	status TEXT NOT NULL,
	retry_count INTEGER NOT NULL DEFAULT 0,
	last_error TEXT NULL,
	locked INTEGER NOT NULL DEFAULT 0,
	next_retry_at TEXT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(appraisal_result_id),
	FOREIGN KEY(appraisal_result_id) REFERENCES appraisal_results(id)
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
CREATE INDEX IF NOT EXISTS idx_job_debug_frames_job_id_created_at ON job_debug_frames(job_id, created_at);
CREATE INDEX IF NOT EXISTS idx_appraisal_result_pvp_evals_result_id ON appraisal_result_pvp_evaluations(appraisal_result_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_result_pvp_evals_species_id ON appraisal_result_pvp_evaluations(evaluated_species_id);
CREATE INDEX IF NOT EXISTS idx_appraisal_result_pvp_eval_queue_status_next_retry ON appraisal_result_pvp_eval_queue(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_appraisal_result_pvp_eval_queue_result_id ON appraisal_result_pvp_eval_queue(appraisal_result_id);`

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
