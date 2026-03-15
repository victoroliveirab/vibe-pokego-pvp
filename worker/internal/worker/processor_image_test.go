package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/tursodatabase/go-libsql"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/imageproc"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/ocr"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/videoproc"
)

type staticOCREngine struct {
	texts   []string
	index   int
	respond func(ocr.ExtractRequest) string
}

type staticVideoSampler struct {
	samples []videoproc.FrameSample
	err     error
}

var (
	testSpeciesCatalogOnce sync.Once
	testSpeciesCatalog     species.Catalog
	testSpeciesCatalogErr  error
)

func (e *staticOCREngine) ExtractText(_ context.Context, request ocr.ExtractRequest) (string, error) {
	if e.respond != nil {
		return e.respond(request), nil
	}

	if len(e.texts) == 0 {
		return "", nil
	}
	if e.index >= len(e.texts) {
		return e.texts[len(e.texts)-1], nil
	}

	result := e.texts[e.index]
	e.index++
	return result, nil
}

func (s staticVideoSampler) SampleFrames(context.Context, string) ([]videoproc.FrameSample, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]videoproc.FrameSample(nil), s.samples...), nil
}

func TestImageProcessorReportsStagesAndPersistsSpeciesCandidate(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	uploadDir := filepath.Join(tempDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("expected upload directory to be created: %v", err)
	}

	imagePath := filepath.Join(uploadDir, "test.png")
	createTestPNG(t, imagePath)

	db := newTestDB(t, databasePath)
	seedUploadAndJob(
		t,
		db,
		"upload-image-1",
		"job-image-1",
		"session-image-1",
		"image",
		"local://uploads/test.png",
		time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC),
	)

	processor := newImageProcessor(
		databasePath,
		config.StorageConfig{Mode: config.UploadStorageModeLocal, LocalDir: uploadDir},
		0,
		&staticOCREngine{
			respond: func(request ocr.ExtractRequest) string {
				switch {
				case strings.Contains(request.CharWhitelist, "CPcp"):
					return "cp 824"
				case strings.Contains(request.CharWhitelist, "HP/"):
					return "141/141 HP"
				case strings.Contains(request.CharWhitelist, "/|!:-"):
					return "Attack 15 Defense 14 HP 13"
				default:
					return "  mR.   mime  "
				}
			},
		},
		mustLoadTestSpeciesCatalog(t),
		nil,
	)

	type progressUpdate struct {
		stage    string
		progress int
	}
	var updates []progressUpdate

	err := processor.Process(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-image-1",
			UploadID:  "upload-image-1",
			SessionID: "session-image-1",
		},
		func(stage string, progress int) error {
			updates = append(updates, progressUpdate{stage: stage, progress: progress})
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected image processor terminal no-appraisal failure")
	}

	var processingErr ProcessingError
	if !errors.As(err, &processingErr) {
		t.Fatalf("expected ProcessingError, got %T", err)
	}
	if processingErr.Code != errorCodeNoAppraisals {
		t.Fatalf("expected code %q, got %q", errorCodeNoAppraisals, processingErr.Code)
	}

	expected := []progressUpdate{
		{stage: jobqueue.StageDownloadingMedia, progress: progressDownloadingMedia},
		{stage: jobqueue.StageDecodingImage, progress: progressDecodingImage},
		{stage: jobqueue.StageExtractingAppraisal, progress: progressExtractingAppraisal},
		{stage: jobqueue.StagePostprocessing, progress: progressPostprocessing},
		{stage: jobqueue.StagePersistingResults, progress: progressPersistingResults},
	}

	if len(updates) != len(expected) {
		t.Fatalf("expected %d progress updates, got %d", len(expected), len(updates))
	}

	for i := range expected {
		if updates[i] != expected[i] {
			t.Fatalf("expected update %d to be %#v, got %#v", i, expected[i], updates[i])
		}
	}

	const candidateQuery = `
SELECT species_name_raw, species_name_normalized, cp_raw, hp_raw, iv_attack_raw, iv_defense_raw, iv_stamina_raw
FROM appraisal_candidates
WHERE job_id = ?;`

	var speciesNameRaw sql.NullString
	var speciesNameNormalized sql.NullString
	var cpRaw sql.NullString
	var hpRaw sql.NullString
	var ivAttackRaw sql.NullString
	var ivDefenseRaw sql.NullString
	var ivStaminaRaw sql.NullString
	if err := db.QueryRowContext(context.Background(), candidateQuery, "job-image-1").Scan(
		&speciesNameRaw,
		&speciesNameNormalized,
		&cpRaw,
		&hpRaw,
		&ivAttackRaw,
		&ivDefenseRaw,
		&ivStaminaRaw,
	); err != nil {
		t.Fatalf("expected persisted candidate row, got: %v", err)
	}

	if !speciesNameRaw.Valid || speciesNameRaw.String != "Mr. Mime" {
		t.Fatalf("expected species_name_raw %q, got %#v", "Mr. Mime", speciesNameRaw)
	}
	if !speciesNameNormalized.Valid || speciesNameNormalized.String != "mr. mime" {
		t.Fatalf("expected species_name_normalized %q, got %#v", "mr. mime", speciesNameNormalized)
	}
	if !cpRaw.Valid || cpRaw.String != "824" {
		t.Fatalf("expected cp_raw %q, got %#v", "824", cpRaw)
	}
	if !hpRaw.Valid || hpRaw.String != "141" {
		t.Fatalf("expected hp_raw %q, got %#v", "141", hpRaw)
	}
	if ivAttackRaw.Valid {
		t.Fatalf("expected iv_attack_raw to be unreadable for minimal test image, got %#v", ivAttackRaw)
	}
	if ivDefenseRaw.Valid {
		t.Fatalf("expected iv_defense_raw to be unreadable for minimal test image, got %#v", ivDefenseRaw)
	}
	if ivStaminaRaw.Valid {
		t.Fatalf("expected iv_stamina_raw to be unreadable for minimal test image, got %#v", ivStaminaRaw)
	}

	var jobKind string
	var processingStartedAt sql.NullString
	var processingFinishedAt sql.NullString
	var speciesFinishedAt sql.NullString
	var cpFinishedAt sql.NullString
	var hpFinishedAt sql.NullString
	var ivFinishedAt sql.NullString
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT kind, processing_started_at, processing_finished_at, species_finished_at, cp_finished_at, hp_finished_at, iv_finished_at
		 FROM job_debug_jobs
		 WHERE job_id = ?;`,
		"job-image-1",
	).Scan(
		&jobKind,
		&processingStartedAt,
		&processingFinishedAt,
		&speciesFinishedAt,
		&cpFinishedAt,
		&hpFinishedAt,
		&ivFinishedAt,
	); err != nil {
		t.Fatalf("expected job_debug_jobs row, got: %v", err)
	}
	if jobKind != "image" {
		t.Fatalf("expected debug kind %q, got %q", "image", jobKind)
	}
	if !processingStartedAt.Valid || !processingFinishedAt.Valid {
		t.Fatalf("expected processing start/finish timestamps, got start=%#v finish=%#v", processingStartedAt, processingFinishedAt)
	}
	if !speciesFinishedAt.Valid || !cpFinishedAt.Valid || !hpFinishedAt.Valid || !ivFinishedAt.Valid {
		t.Fatalf(
			"expected species/cp/hp/iv job milestones to be populated, got species=%#v cp=%#v hp=%#v iv=%#v",
			speciesFinishedAt,
			cpFinishedAt,
			hpFinishedAt,
			ivFinishedAt,
		)
	}

	var frameSourceType string
	var frameIndex int
	var frameTimestamp sql.NullInt64
	var frameStatus string
	var layoutMeta sql.NullString
	var speciesMeta sql.NullString
	var cpMeta sql.NullString
	var hpMeta sql.NullString
	var ivMeta sql.NullString
	var ivBarMeta sql.NullString
	var selectionMeta sql.NullString
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT source_type, frame_index, frame_timestamp_ms, frame_status, layout_meta_json, species_meta_json, cp_meta_json, hp_meta_json, iv_meta_json, iv_bar_measurement_meta_json, selection_meta_json
		 FROM job_debug_frames
		 WHERE job_id = ?;`,
		"job-image-1",
	).Scan(
		&frameSourceType,
		&frameIndex,
		&frameTimestamp,
		&frameStatus,
		&layoutMeta,
		&speciesMeta,
		&cpMeta,
		&hpMeta,
		&ivMeta,
		&ivBarMeta,
		&selectionMeta,
	); err != nil {
		t.Fatalf("expected one frame debug row for image job, got: %v", err)
	}
	if frameSourceType != appraisal.SourceTypeImage {
		t.Fatalf("expected frame source_type %q, got %q", appraisal.SourceTypeImage, frameSourceType)
	}
	if frameIndex != 1 {
		t.Fatalf("expected image frame_index=1, got %d", frameIndex)
	}
	if frameTimestamp.Valid {
		t.Fatalf("expected image frame_timestamp_ms to be NULL, got %#v", frameTimestamp)
	}
	if frameStatus != "processed" {
		t.Fatalf("expected image frame_status %q, got %q", "processed", frameStatus)
	}
	assertValidJSONColumn(t, "layout_meta_json", layoutMeta)
	assertValidJSONColumn(t, "species_meta_json", speciesMeta)
	assertValidJSONColumn(t, "cp_meta_json", cpMeta)
	assertValidJSONColumn(t, "hp_meta_json", hpMeta)
	assertValidJSONColumn(t, "iv_meta_json", ivMeta)
	assertValidJSONColumn(t, "iv_bar_measurement_meta_json", ivBarMeta)
	assertValidJSONColumn(t, "selection_meta_json", selectionMeta)
	assertStructuredMetaEntries(t, "species_meta_json", speciesMeta, "species_attempt_meta_texts")
	assertStructuredMetaEntries(t, "cp_meta_json", cpMeta, "cp_attempt_meta_texts")
	assertStructuredMetaEntries(t, "hp_meta_json", hpMeta, "hp_attempt_meta_texts")
	assertStructuredMetaEntries(t, "selection_meta_json", selectionMeta, "all_meta_texts")

	artifactsDir := filepath.Join(tempDir, "worker-image-debug", "job-image-1")
	entries, err := os.ReadDir(artifactsDir)
	if err != nil {
		t.Fatalf("expected artifacts directory %s, got error: %v", artifactsDir, err)
	}

	pngCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".png") {
			pngCount++
		}
	}

	if pngCount < 3 {
		t.Fatalf("expected at least 3 artifact png files, got %d", pngCount)
	}
}

func TestImageProcessorVideoPathReportsStagesAndPersistsFrameTimestamps(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	uploadDir := filepath.Join(tempDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("expected upload directory to be created: %v", err)
	}

	videoPath := filepath.Join(uploadDir, "test.mp4")
	if err := os.WriteFile(videoPath, []byte("video-fixture-placeholder"), 0o644); err != nil {
		t.Fatalf("expected video fixture placeholder to be created: %v", err)
	}

	db := newTestDB(t, databasePath)
	seedUploadAndJob(
		t,
		db,
		"upload-video-1",
		"job-video-1",
		"session-video-1",
		"video",
		"local://uploads/test.mp4",
		time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC),
	)

	type progressUpdate struct {
		stage    string
		progress int
	}
	var updates []progressUpdate

	processor := newImageProcessor(
		databasePath,
		config.StorageConfig{Mode: config.UploadStorageModeLocal, LocalDir: uploadDir},
		0,
		ocr.NewNoopEngine(),
		mustLoadTestSpeciesCatalog(t),
		staticVideoSampler{
			samples: []videoproc.FrameSample{
				{TimestampMS: 0, Image: syntheticVideoFrameImageWithCardShift(220)},
				{TimestampMS: 300, Image: syntheticVideoFrameImage()},
				{TimestampMS: 600, Image: syntheticVideoFrameImage()},
				{TimestampMS: 900, Image: syntheticVideoFrameImage()},
			},
		},
	)

	err := processor.Process(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-video-1",
			UploadID:  "upload-video-1",
			SessionID: "session-video-1",
		},
		func(stage string, progress int) error {
			updates = append(updates, progressUpdate{stage: stage, progress: progress})
			return nil
		},
	)
	assertNoAppraisalsProcessingError(t, err)

	expected := []progressUpdate{
		{stage: jobqueue.StageDownloadingMedia, progress: progressDownloadingMedia},
		{stage: jobqueue.StageDecodingVideo, progress: progressDecodingVideo},
		{stage: jobqueue.StageSamplingFrames, progress: progressSamplingFrames},
		{stage: jobqueue.StageExtractingAppraisal, progress: progressExtractingAppraisal},
		{stage: jobqueue.StagePostprocessing, progress: progressPostprocessing},
		{stage: jobqueue.StagePersistingResults, progress: progressPersistingResults},
	}
	if len(updates) != len(expected) {
		t.Fatalf("expected %d progress updates, got %d", len(expected), len(updates))
	}
	for i := range expected {
		if updates[i] != expected[i] {
			t.Fatalf("expected update %d to be %#v, got %#v", i, expected[i], updates[i])
		}
	}

	rows, queryErr := db.QueryContext(
		context.Background(),
		`SELECT source_type, frame_timestamp_ms
		 FROM appraisal_candidates
		 WHERE job_id = ?
		 ORDER BY frame_timestamp_ms ASC;`,
		"job-video-1",
	)
	if queryErr != nil {
		t.Fatalf("expected video candidate query to succeed: %v", queryErr)
	}
	defer rows.Close()

	var timestamps []int64
	for rows.Next() {
		var sourceType string
		var frameTimestamp sql.NullInt64
		if err := rows.Scan(&sourceType, &frameTimestamp); err != nil {
			t.Fatalf("expected candidate row scan to succeed: %v", err)
		}
		if sourceType != string(appraisal.SourceTypeVideo) {
			t.Fatalf("expected source_type VIDEO, got %q", sourceType)
		}
		if !frameTimestamp.Valid {
			t.Fatal("expected frame_timestamp_ms to be non-null for video candidates")
		}
		timestamps = append(timestamps, frameTimestamp.Int64)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("expected rows iteration to succeed: %v", err)
	}
	if len(timestamps) != 2 {
		t.Fatalf("expected 2 video candidate rows from stable subset, got %d", len(timestamps))
	}
	if timestamps[0] != 600 || timestamps[1] != 900 {
		t.Fatalf("expected frame timestamps [600 900], got %v", timestamps)
	}

	var debugKind string
	var videoProcessingStartedAt sql.NullString
	var videoProcessingFinishedAt sql.NullString
	var videoCPFinishedAt sql.NullString
	var videoHPFinishedAt sql.NullString
	var videoIVFinishedAt sql.NullString
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT kind, processing_started_at, processing_finished_at, cp_finished_at, hp_finished_at, iv_finished_at
		 FROM job_debug_jobs
		 WHERE job_id = ?;`,
		"job-video-1",
	).Scan(
		&debugKind,
		&videoProcessingStartedAt,
		&videoProcessingFinishedAt,
		&videoCPFinishedAt,
		&videoHPFinishedAt,
		&videoIVFinishedAt,
	); err != nil {
		t.Fatalf("expected job_debug_jobs row for video, got: %v", err)
	}
	if debugKind != "video" {
		t.Fatalf("expected debug kind %q, got %q", "video", debugKind)
	}
	if !videoProcessingStartedAt.Valid || !videoProcessingFinishedAt.Valid {
		t.Fatalf("expected video processing start/finish timestamps, got start=%#v finish=%#v", videoProcessingStartedAt, videoProcessingFinishedAt)
	}
	if !videoCPFinishedAt.Valid || !videoHPFinishedAt.Valid || !videoIVFinishedAt.Valid {
		t.Fatalf(
			"expected cp/hp/iv milestones for processed video frames, got cp=%#v hp=%#v iv=%#v",
			videoCPFinishedAt,
			videoHPFinishedAt,
			videoIVFinishedAt,
		)
	}

	frameRows, err := db.QueryContext(
		context.Background(),
		`SELECT frame_index, frame_timestamp_ms, frame_status, frame_stability_meta_json, species_meta_json
		 FROM job_debug_frames
		 WHERE job_id = ?
		 ORDER BY frame_index ASC;`,
		"job-video-1",
	)
	if err != nil {
		t.Fatalf("expected job_debug_frames query to succeed: %v", err)
	}
	defer frameRows.Close()

	type debugFrameRow struct {
		index         int
		timestampMS   sql.NullInt64
		status        string
		stabilityMeta sql.NullString
		speciesMeta   sql.NullString
	}
	var debugFrames []debugFrameRow
	for frameRows.Next() {
		var row debugFrameRow
		if err := frameRows.Scan(&row.index, &row.timestampMS, &row.status, &row.stabilityMeta, &row.speciesMeta); err != nil {
			t.Fatalf("expected job_debug_frames scan to succeed: %v", err)
		}
		debugFrames = append(debugFrames, row)
	}
	if err := frameRows.Err(); err != nil {
		t.Fatalf("expected job_debug_frames rows iteration to succeed: %v", err)
	}
	if len(debugFrames) != 4 {
		t.Fatalf("expected one debug frame row per sampled frame (4), got %d", len(debugFrames))
	}

	expectedStatuses := []string{"skipped_stability", "skipped_stability", "processed", "processed"}
	expectedTimestamps := []int64{0, 300, 600, 900}
	for i, row := range debugFrames {
		if row.index != i+1 {
			t.Fatalf("expected frame_index=%d, got %d", i+1, row.index)
		}
		if !row.timestampMS.Valid || row.timestampMS.Int64 != expectedTimestamps[i] {
			t.Fatalf("expected frame_timestamp_ms=%d, got %#v", expectedTimestamps[i], row.timestampMS)
		}
		if row.status != expectedStatuses[i] {
			t.Fatalf("expected frame_status=%q for frame %d, got %q", expectedStatuses[i], i+1, row.status)
		}
		assertValidJSONColumn(t, "frame_stability_meta_json", row.stabilityMeta)
		if row.status == "processed" {
			assertValidJSONColumn(t, "species_meta_json", row.speciesMeta)
		} else if row.speciesMeta.Valid {
			t.Fatalf("expected skipped-stability frame species_meta_json to be NULL, got %#v", row.speciesMeta)
		}
	}

	artifactsDir := filepath.Join(tempDir, "worker-image-debug", "job-video-1")
	expectedFrameArtifacts := []string{
		"frame_1_jittered.png",
		"frame_2_duplicate.png",
		"frame_3_processed.png",
		"frame_4_processed.png",
	}
	for _, expected := range expectedFrameArtifacts {
		artifactPath := filepath.Join(artifactsDir, expected)
		if _, err := os.Stat(artifactPath); err != nil {
			t.Fatalf("expected artifact %q to exist: %v", expected, err)
		}
	}
}

func TestImageProcessorVideoPathAllUnstableFramesPersistNoCandidates(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	uploadDir := filepath.Join(tempDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("expected upload directory to be created: %v", err)
	}

	videoPath := filepath.Join(uploadDir, "test-unstable.mp4")
	if err := os.WriteFile(videoPath, []byte("video-fixture-placeholder"), 0o644); err != nil {
		t.Fatalf("expected video fixture placeholder to be created: %v", err)
	}

	db := newTestDB(t, databasePath)
	seedUploadAndJob(
		t,
		db,
		"upload-video-all-unstable",
		"job-video-all-unstable",
		"session-video-all-unstable",
		"video",
		"local://uploads/test-unstable.mp4",
		time.Date(2026, time.March, 5, 12, 5, 0, 0, time.UTC),
	)

	processor := newImageProcessor(
		databasePath,
		config.StorageConfig{Mode: config.UploadStorageModeLocal, LocalDir: uploadDir},
		0,
		ocr.NewNoopEngine(),
		mustLoadTestSpeciesCatalog(t),
		staticVideoSampler{
			samples: []videoproc.FrameSample{
				{TimestampMS: 0, Image: syntheticVideoFrameImageWithCardShift(-230)},
				{TimestampMS: 300, Image: syntheticVideoFrameImageWithCardShift(220)},
				{TimestampMS: 600, Image: syntheticVideoFrameImageWithCardShift(-220)},
			},
		},
	)

	err := processor.Process(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-video-all-unstable",
			UploadID:  "upload-video-all-unstable",
			SessionID: "session-video-all-unstable",
		},
		func(string, int) error { return nil },
	)
	assertNoAppraisalsProcessingError(t, err)

	assertTableRowCount(t, db, "appraisal_candidates", 0)
	assertTableRowCount(t, db, "appraisal_results", 0)
	assertTableRowCount(t, db, "appraisal_pending_readings", 0)
	assertTableRowCount(t, db, "job_debug_frames", 3)

	rows, err := db.QueryContext(
		context.Background(),
		`SELECT frame_status, frame_stability_meta_json
		 FROM job_debug_frames
		 WHERE job_id = ?
		 ORDER BY frame_index ASC;`,
		"job-video-all-unstable",
	)
	if err != nil {
		t.Fatalf("expected unstable-frame debug rows query to succeed: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var frameStabilityMeta sql.NullString
		if err := rows.Scan(&status, &frameStabilityMeta); err != nil {
			t.Fatalf("expected unstable debug row scan to succeed: %v", err)
		}
		if status != "skipped_stability" {
			t.Fatalf("expected skipped_stability status for unstable frame, got %q", status)
		}
		assertValidJSONColumn(t, "frame_stability_meta_json", frameStabilityMeta)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("expected unstable debug rows iteration to succeed: %v", err)
	}
}

func TestPersistVideoFramesDeduplicatesAcceptedResultsAndReturnsSuccess(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	seedUploadAndJob(
		t,
		db,
		"upload-video-dedup",
		"job-video-dedup",
		"session-video-dedup",
		"video",
		"local://uploads/video-dedup.mp4",
		time.Date(2026, time.March, 5, 12, 20, 0, 0, time.UTC),
	)

	processor := imageProcessor{databasePath: databasePath}
	frames := []processedFrame{
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Darumaka"),
				SpeciesNameNormalized: testStringPtr("darumaka"),
				CPRaw:                 testStringPtr("712"),
				HPRaw:                 testStringPtr("120"),
				IVAttackRaw:           testStringPtr("10"),
				IVDefenseRaw:          testStringPtr("11"),
				IVStaminaRaw:          testStringPtr("12"),
			},
			rawOCRText: "Darumaka",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
				},
			},
			timestampMS: int64Ptr(600),
		},
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Darumaka"),
				SpeciesNameNormalized: testStringPtr("darumaka"),
				CPRaw:                 testStringPtr("712"),
				HPRaw:                 testStringPtr("120"),
				IVAttackRaw:           testStringPtr("10"),
				IVDefenseRaw:          testStringPtr("11"),
				IVStaminaRaw:          testStringPtr("12"),
			},
			rawOCRText: "Darumaka",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
				},
			},
			timestampMS: int64Ptr(0),
		},
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Munna"),
				SpeciesNameNormalized: testStringPtr("munna"),
				CPRaw:                 testStringPtr("824"),
				HPRaw:                 testStringPtr("141"),
				IVAttackRaw:           testStringPtr("0"),
				IVDefenseRaw:          testStringPtr("13"),
				IVStaminaRaw:          testStringPtr("13"),
			},
			rawOCRText: "Munna",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Munna", 824, 141, 0, 13, 13),
				},
			},
			timestampMS: int64Ptr(300),
		},
	}

	hasPending, acceptedCount, err := processor.persistVideoFrames(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-video-dedup",
			UploadID:  "upload-video-dedup",
			SessionID: "session-video-dedup",
		},
		frames,
	)
	if err != nil {
		t.Fatalf("expected persistVideoFrames to succeed: %v", err)
	}
	if hasPending {
		t.Fatal("expected no pending ambiguity for unambiguous video frames")
	}
	if acceptedCount != 2 {
		t.Fatalf("expected 2 deduplicated accepted results, got %d", acceptedCount)
	}

	if err := finalizeProcessingOutcome(hasPending, acceptedCount); err != nil {
		t.Fatalf("expected successful terminal outcome for accepted video results, got %v", err)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 3)
	assertTableRowCount(t, db, "appraisal_results", 2)
	assertTableRowCount(t, db, "appraisal_pending_readings", 0)

	rows, queryErr := db.QueryContext(
		context.Background(),
		`SELECT species_name, source_type, frame_timestamp_ms
		 FROM appraisal_results
		 WHERE job_id = ?
		 ORDER BY frame_timestamp_ms ASC;`,
		"job-video-dedup",
	)
	if queryErr != nil {
		t.Fatalf("expected appraisal_results query to succeed: %v", queryErr)
	}
	defer rows.Close()

	var got []struct {
		speciesName string
		sourceType  string
		timestampMS sql.NullInt64
	}
	for rows.Next() {
		var row struct {
			speciesName string
			sourceType  string
			timestampMS sql.NullInt64
		}
		if err := rows.Scan(&row.speciesName, &row.sourceType, &row.timestampMS); err != nil {
			t.Fatalf("expected result row scan to succeed: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("expected result rows iteration to succeed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 persisted result rows, got %d", len(got))
	}

	if got[0].sourceType != appraisal.SourceTypeVideo || !got[0].timestampMS.Valid || got[0].timestampMS.Int64 != 0 {
		t.Fatalf("expected first deduped result at timestamp 0 with VIDEO source, got %#v", got[0])
	}
	if got[1].sourceType != appraisal.SourceTypeVideo || !got[1].timestampMS.Valid || got[1].timestampMS.Int64 != 300 {
		t.Fatalf("expected second deduped result at timestamp 300 with VIDEO source, got %#v", got[1])
	}
}

func TestPersistVideoFramesAmbiguousOutcomesReturnPendingSignal(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	seedUploadAndJob(
		t,
		db,
		"upload-video-ambiguous",
		"job-video-ambiguous",
		"session-video-ambiguous",
		"video",
		"local://uploads/video-ambiguous.mp4",
		time.Date(2026, time.March, 5, 12, 25, 0, 0, time.UTC),
	)

	processor := imageProcessor{databasePath: databasePath}
	frames := []processedFrame{
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Darumaka"),
				SpeciesNameNormalized: testStringPtr("darumaka"),
				CPRaw:                 testStringPtr("712"),
				HPRaw:                 testStringPtr("120"),
				IVAttackRaw:           testStringPtr("10"),
				IVDefenseRaw:          testStringPtr("11"),
				IVStaminaRaw:          testStringPtr("12"),
			},
			rawOCRText: "Darumaka",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
					makeAcceptedResultCandidate("Darumaka (Galarian)", 712, 120, 10, 11, 12),
				},
			},
			timestampMS: int64Ptr(300),
		},
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Darumaka"),
				SpeciesNameNormalized: testStringPtr("darumaka"),
				CPRaw:                 testStringPtr("712"),
				HPRaw:                 testStringPtr("120"),
				IVAttackRaw:           testStringPtr("10"),
				IVDefenseRaw:          testStringPtr("11"),
				IVStaminaRaw:          testStringPtr("12"),
			},
			rawOCRText: "Darumaka",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
					makeAcceptedResultCandidate("Darumaka (Galarian)", 712, 120, 10, 11, 12),
				},
			},
			timestampMS: int64Ptr(600),
		},
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Darumaka"),
				SpeciesNameNormalized: testStringPtr("darumaka"),
				CPRaw:                 testStringPtr("712"),
				HPRaw:                 testStringPtr("120"),
				IVAttackRaw:           testStringPtr("10"),
				IVDefenseRaw:          testStringPtr("11"),
				IVStaminaRaw:          testStringPtr("12"),
			},
			rawOCRText: "Darumaka",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
					makeAcceptedResultCandidate("Darumaka (Galarian)", 712, 120, 10, 11, 12),
				},
			},
			timestampMS: int64Ptr(900),
		},
	}

	hasPending, acceptedCount, err := processor.persistVideoFrames(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-video-ambiguous",
			UploadID:  "upload-video-ambiguous",
			SessionID: "session-video-ambiguous",
		},
		frames,
	)
	if err != nil {
		t.Fatalf("expected persistVideoFrames to succeed: %v", err)
	}
	if !hasPending {
		t.Fatal("expected pending ambiguity flag for ambiguous video readings")
	}
	if acceptedCount != 0 {
		t.Fatalf("expected 0 accepted results for ambiguous-only video frames, got %d", acceptedCount)
	}

	outcomeErr := finalizeProcessingOutcome(hasPending, acceptedCount)
	var pendingSignal pendingUserDedupSignal
	if !errors.As(outcomeErr, &pendingSignal) {
		t.Fatalf("expected pendingUserDedupSignal terminal outcome, got %v", outcomeErr)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 3)
	assertTableRowCount(t, db, "appraisal_results", 0)
	assertTableRowCount(t, db, "appraisal_pending_readings", 1)
	assertTableRowCount(t, db, "appraisal_pending_species_options", 2)

	var sourceType string
	var frameTimestamp sql.NullInt64
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT source_type, frame_timestamp_ms
		 FROM appraisal_pending_readings
		 WHERE job_id = ?;`,
		"job-video-ambiguous",
	).Scan(&sourceType, &frameTimestamp); err != nil {
		t.Fatalf("expected pending reading query to succeed: %v", err)
	}
	if sourceType != appraisal.SourceTypeVideo {
		t.Fatalf("expected pending reading source_type VIDEO, got %q", sourceType)
	}
	if !frameTimestamp.Valid || frameTimestamp.Int64 != 300 {
		t.Fatalf("expected pending reading frame_timestamp_ms 300, got %#v", frameTimestamp)
	}
}

func TestPersistVideoFramesCollapsesAdjacentIVDriftForSameSpeciesCPHP(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	seedUploadAndJob(
		t,
		db,
		"upload-video-iv-drift",
		"job-video-iv-drift",
		"session-video-iv-drift",
		"video",
		"local://uploads/video-iv-drift.mp4",
		time.Date(2026, time.March, 5, 12, 27, 0, 0, time.UTC),
	)

	processor := imageProcessor{databasePath: databasePath}
	frames := []processedFrame{
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Feebas"),
				SpeciesNameNormalized: testStringPtr("feebas"),
				CPRaw:                 testStringPtr("12"),
				HPRaw:                 testStringPtr("21"),
				IVAttackRaw:           testStringPtr("0"),
				IVDefenseRaw:          testStringPtr("10"),
				IVStaminaRaw:          testStringPtr("8"),
			},
			rawOCRText: "Feebas",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Feebas", 12, 21, 0, 10, 8),
				},
			},
			timestampMS: int64Ptr(21300),
		},
		{
			parsed: appraisal.ParsedCandidate{
				SpeciesNameRaw:        testStringPtr("Feebas"),
				SpeciesNameNormalized: testStringPtr("feebas"),
				CPRaw:                 testStringPtr("12"),
				HPRaw:                 testStringPtr("21"),
				IVAttackRaw:           testStringPtr("0"),
				IVDefenseRaw:          testStringPtr("7"),
				IVStaminaRaw:          testStringPtr("14"),
			},
			rawOCRText: "Feebas",
			validation: appraisal.ValidationDecision{
				SpeciesIsCanonical: true,
				AcceptedResults: []appraisal.AcceptedResultCandidate{
					makeAcceptedResultCandidate("Feebas", 12, 21, 0, 7, 14),
				},
			},
			timestampMS: int64Ptr(21600),
		},
	}

	hasPending, acceptedCount, err := processor.persistVideoFrames(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-video-iv-drift",
			UploadID:  "upload-video-iv-drift",
			SessionID: "session-video-iv-drift",
		},
		frames,
	)
	if err != nil {
		t.Fatalf("expected persistVideoFrames to succeed: %v", err)
	}
	if hasPending {
		t.Fatal("expected no pending ambiguity for unambiguous iv-drift frames")
	}
	if acceptedCount != 1 {
		t.Fatalf("expected 1 accepted result after adjacent-frame iv drift collapse, got %d", acceptedCount)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 2)
	assertTableRowCount(t, db, "appraisal_results", 1)
}

func TestImageProcessorRespectsContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	uploadDir := filepath.Join(tempDir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		t.Fatalf("expected upload directory to be created: %v", err)
	}

	imagePath := filepath.Join(uploadDir, "test.png")
	createTestPNG(t, imagePath)

	db := newTestDB(t, databasePath)
	seedUploadAndJob(
		t,
		db,
		"upload-image-2",
		"job-image-2",
		"session-image-2",
		"image",
		"local://uploads/test.png",
		time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC),
	)

	processor := newImageProcessor(
		databasePath,
		config.StorageConfig{Mode: config.UploadStorageModeLocal, LocalDir: uploadDir},
		10*time.Millisecond,
		ocr.NewNoopEngine(),
		mustLoadTestSpeciesCatalog(t),
		nil,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := processor.Process(
		ctx,
		jobqueue.ClaimedJob{
			ID:        "job-image-2",
			UploadID:  "upload-image-2",
			SessionID: "session-image-2",
		},
		func(string, int) error { return nil },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestPersistCandidateAndAcceptedPersistsAcceptedResult(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	now := time.Date(2026, time.March, 4, 12, 0, 0, 0, time.UTC)
	seedUploadAndJob(
		t,
		db,
		"upload-valid-accept",
		"job-valid-accept",
		"session-valid-accept",
		"image",
		"local://uploads/valid.png",
		now,
	)

	catalog := mustLoadTestSpeciesCatalog(t)
	entry, ok := catalog.EntryForNormalized("munna")
	if !ok {
		t.Fatal("expected munna in species catalog")
	}
	cp, hp, ok := catalog.ComputeCPHP(entry, 20.0, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation for munna level 20")
	}

	parsed := appraisal.ParseCandidateFromOCR("Munna")
	parsed.CPRaw = testStringPtr(strconv.Itoa(cp))
	parsed.HPRaw = testStringPtr(strconv.Itoa(hp))
	parsed.IVAttackRaw = testStringPtr("10")
	parsed.IVDefenseRaw = testStringPtr("12")
	parsed.IVStaminaRaw = testStringPtr("13")
	decision := appraisal.ValidateCandidate(parsed, catalog, nil)
	if len(decision.AcceptedResults) != 1 {
		t.Fatal("expected validation to accept candidate")
	}

	processor := imageProcessor{
		databasePath:   databasePath,
		speciesCatalog: catalog,
	}
	acceptedCount, err := processor.persistCandidateAndAccepted(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-valid-accept",
			UploadID:  "upload-valid-accept",
			SessionID: "session-valid-accept",
		},
		parsed,
		"Munna",
		decision,
		appraisal.SourceTypeImage,
		nil,
	)
	if err != nil {
		t.Fatalf("expected persistCandidateAndAccepted to succeed: %v", err)
	}
	if acceptedCount != 1 {
		t.Fatalf("expected 1 accepted result, got %d", acceptedCount)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 1)
	assertTableRowCount(t, db, "appraisal_results", 1)

	var canonical int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT species_is_canonical FROM appraisal_candidates WHERE job_id = ?;`,
		"job-valid-accept",
	).Scan(&canonical); err != nil {
		t.Fatalf("expected candidate canonical flag row: %v", err)
	}
	if canonical != 1 {
		t.Fatalf("expected species_is_canonical=1, got %d", canonical)
	}
}

func TestPersistCandidateAndAcceptedRejectsImpossibleButKeepsRawCandidate(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	now := time.Date(2026, time.March, 4, 12, 5, 0, 0, time.UTC)
	seedUploadAndJob(
		t,
		db,
		"upload-invalid-reject",
		"job-invalid-reject",
		"session-invalid-reject",
		"image",
		"local://uploads/invalid.png",
		now,
	)

	catalog := mustLoadTestSpeciesCatalog(t)
	parsed := appraisal.ParseCandidateFromOCR("Munna")
	parsed.CPRaw = testStringPtr("9999")
	parsed.HPRaw = testStringPtr("999")
	parsed.IVAttackRaw = testStringPtr("15")
	parsed.IVDefenseRaw = testStringPtr("15")
	parsed.IVStaminaRaw = testStringPtr("15")
	decision := appraisal.ValidateCandidate(parsed, catalog, nil)
	if !decision.SpeciesIsCanonical {
		t.Fatal("expected canonical species match")
	}
	if len(decision.AcceptedResults) != 0 {
		t.Fatal("expected impossible tuple to be rejected")
	}

	processor := imageProcessor{
		databasePath:   databasePath,
		speciesCatalog: catalog,
	}
	acceptedCount, err := processor.persistCandidateAndAccepted(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-invalid-reject",
			UploadID:  "upload-invalid-reject",
			SessionID: "session-invalid-reject",
		},
		parsed,
		"Munna",
		decision,
		appraisal.SourceTypeImage,
		nil,
	)
	if err != nil {
		t.Fatalf("expected persistCandidateAndAccepted to succeed for rejected candidate: %v", err)
	}
	if acceptedCount != 0 {
		t.Fatalf("expected 0 accepted results, got %d", acceptedCount)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 1)
	assertTableRowCount(t, db, "appraisal_results", 0)
}

func TestPersistCandidateAndAcceptedPersistsPendingReadingAndOptionsForAmbiguousSpecies(t *testing.T) {
	tempDir := t.TempDir()
	databasePath := filepath.Join(tempDir, "worker.db")
	db := newTestDB(t, databasePath)

	now := time.Date(2026, time.March, 4, 12, 10, 0, 0, time.UTC)
	seedUploadAndJob(
		t,
		db,
		"upload-ambiguous-pending",
		"job-ambiguous-pending",
		"session-ambiguous-pending",
		"image",
		"local://uploads/ambiguous.png",
		now,
	)

	catalog := mustBuildAmbiguousTestCatalog(t)
	entry, ok := catalog.EntryForNormalized("mr. mime")
	if !ok {
		t.Fatal("expected Mr. Mime in species catalog")
	}
	cp, hp, ok := catalog.ComputeCPHP(entry, 20.0, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation to succeed")
	}

	parsed := appraisal.ParseCandidateFromOCR("Mr. Mime")
	parsed.CPRaw = testStringPtr(strconv.Itoa(cp))
	parsed.HPRaw = testStringPtr(strconv.Itoa(hp))
	parsed.IVAttackRaw = testStringPtr("10")
	parsed.IVDefenseRaw = testStringPtr("12")
	parsed.IVStaminaRaw = testStringPtr("13")

	decision := appraisal.ValidateCandidate(parsed, catalog, nil)
	if len(decision.AcceptedResults) != 2 {
		t.Fatalf("expected 2 accepted options, got %d", len(decision.AcceptedResults))
	}

	processor := imageProcessor{
		databasePath:   databasePath,
		speciesCatalog: catalog,
	}
	acceptedCount, err := processor.persistCandidateAndAccepted(
		context.Background(),
		jobqueue.ClaimedJob{
			ID:        "job-ambiguous-pending",
			UploadID:  "upload-ambiguous-pending",
			SessionID: "session-ambiguous-pending",
		},
		parsed,
		"Mr. Mime",
		decision,
		appraisal.SourceTypeImage,
		nil,
	)
	if err != nil {
		t.Fatalf("expected persistCandidateAndAccepted to succeed: %v", err)
	}
	if acceptedCount != 2 {
		t.Fatalf("expected 2 accepted results, got %d", acceptedCount)
	}

	assertTableRowCount(t, db, "appraisal_candidates", 1)
	assertTableRowCount(t, db, "appraisal_results", 0)
	assertTableRowCount(t, db, "appraisal_pending_readings", 1)
	assertTableRowCount(t, db, "appraisal_pending_species_options", 2)

	var readingStatus string
	var readingLocked int
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT status, locked FROM appraisal_pending_readings WHERE job_id = ?;`,
		"job-ambiguous-pending",
	).Scan(&readingStatus, &readingLocked); err != nil {
		t.Fatalf("expected pending reading row, got: %v", err)
	}
	if readingStatus != jobqueue.JobStatusPendingUserDedup {
		t.Fatalf("expected pending reading status %q, got %q", jobqueue.JobStatusPendingUserDedup, readingStatus)
	}
	if readingLocked != 0 {
		t.Fatalf("expected pending reading locked=0, got %d", readingLocked)
	}

	rows, err := db.QueryContext(
		context.Background(),
		`SELECT species_name, match_mode, match_distance, option_rank
FROM appraisal_pending_species_options
ORDER BY option_rank ASC;`,
	)
	if err != nil {
		t.Fatalf("expected pending options query to succeed, got: %v", err)
	}
	defer rows.Close()

	type optionRow struct {
		speciesName   string
		matchMode     string
		matchDistance int
		optionRank    int
	}
	var options []optionRow
	for rows.Next() {
		var row optionRow
		if err := rows.Scan(&row.speciesName, &row.matchMode, &row.matchDistance, &row.optionRank); err != nil {
			t.Fatalf("expected pending option scan to succeed, got: %v", err)
		}
		options = append(options, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("expected pending options rows iteration to succeed, got: %v", err)
	}
	if len(options) != 2 {
		t.Fatalf("expected 2 pending options, got %d", len(options))
	}
	if options[0].speciesName != "Mr. Mime" || options[0].matchMode != "exact" || options[0].optionRank != 1 {
		t.Fatalf("expected first pending option to be exact Mr. Mime rank 1, got %#v", options[0])
	}
	if options[1].speciesName != "Mr. Mime (Galarian)" || options[1].matchMode != "prefix" || options[1].optionRank != 2 {
		t.Fatalf("expected second pending option to be prefix Mr. Mime (Galarian) rank 2, got %#v", options[1])
	}
}

func TestBuildSpeciesNameAttemptsUsesAnchorRelativeBoundedRegions(t *testing.T) {
	preprocessedImage := image.NewGray(image.Rect(0, 0, 2412, 5244))
	anchor := appraisal.NameAnchor{
		Band:       image.Rect(0, 2366, 2412, 2392),
		CenterY:    2379,
		Confidence: 0.95,
	}

	attempts := buildSpeciesNameAttempts(preprocessedImage, []appraisal.NameAnchor{anchor})
	if len(attempts) == 0 {
		t.Fatal("expected species-name attempts")
	}

	for _, attempt := range attempts {
		if attempt.Region.Empty() {
			t.Fatal("expected bounded region for each attempt")
		}
		if attempt.Region == preprocessedImage.Bounds() {
			t.Fatalf("expected no full-image fallback region, got %v", attempt.Region)
		}
		if attempt.Region.Max.Y > anchor.CenterY+8 {
			t.Fatalf("expected anchor-relative name region above HP anchor center, got region %v and anchor center %d", attempt.Region, anchor.CenterY)
		}
	}
}

func TestBuildSpeciesNameAttemptsAddsGuardedFallbackForLowConfidenceAnchor(t *testing.T) {
	preprocessedImage := image.NewGray(image.Rect(0, 0, 2412, 5244))
	anchor := appraisal.NameAnchor{
		Band:       image.Rect(0, 2366, 2412, 2392),
		CenterY:    2379,
		Confidence: 0.20,
	}

	attempts := buildSpeciesNameAttempts(preprocessedImage, []appraisal.NameAnchor{anchor})
	if len(attempts) == 0 {
		t.Fatal("expected species-name attempts")
	}

	hasWideRegion := false
	for _, attempt := range attempts {
		if attempt.Region.Empty() {
			t.Fatal("expected bounded region for each attempt")
		}
		if attempt.Region == preprocessedImage.Bounds() {
			t.Fatalf("expected no full-image fallback region, got %v", attempt.Region)
		}
		if attempt.Region.Dx() >= int(float64(preprocessedImage.Bounds().Dx())*0.70) {
			hasWideRegion = true
		}
	}

	if !hasWideRegion {
		t.Fatal("expected a guarded wide fallback region for low-confidence anchor")
	}
}

func TestBuildDeterministicSpeciesNameAttemptsUsesPrimaryROIOnly(t *testing.T) {
	preprocessedImage := image.NewGray(image.Rect(0, 0, 2412, 5244))
	nameROI := appraisal.NameROI{
		Band:   image.Rect(500, 1800, 1900, 2200),
		Region: image.Rect(700, 1980, 1700, 2120),
	}

	attempts := buildDeterministicSpeciesNameAttempts(preprocessedImage, nameROI)
	if len(attempts) != 2 {
		t.Fatalf("expected 2 deterministic attempts (psm7/6), got %d", len(attempts))
	}

	if attempts[0].Region != nameROI.Region || attempts[1].Region != nameROI.Region {
		t.Fatalf("expected both attempts to use primary ROI region %v, got %v and %v", nameROI.Region, attempts[0].Region, attempts[1].Region)
	}
}

func TestDeriveCPRegionsPrioritizesUpperImageArea(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
		CardBounds:  image.Rect(66, 1740, 2346, 5042),
		NameBand:    image.Rect(338, 2004, 2074, 2274),
	}

	regions := deriveCPRegions(layout, bounds)
	if len(regions) == 0 {
		t.Fatal("expected cp regions")
	}

	primary := regions[0]
	if primary.Min.Y >= int(float64(bounds.Dy())*0.08) {
		t.Fatalf("expected primary CP region near top of image, got %v", primary)
	}
	if primary.Dy() > int(float64(bounds.Dy())*0.12) {
		t.Fatalf("expected primary CP region to be vertically tight, got %v", primary)
	}
	if primary.Max.Y >= layout.NameBand.Min.Y {
		t.Fatalf("expected primary CP region above name band, got %v (name band %v)", primary, layout.NameBand)
	}
}

func TestDeriveCPRegionReturnsFirstCandidate(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
		NameBand:    image.Rect(338, 2004, 2074, 2274),
	}

	regions := deriveCPRegions(layout, bounds)
	if len(regions) == 0 {
		t.Fatal("expected cp regions")
	}

	region := deriveCPRegion(layout, bounds)
	if region != regions[0] {
		t.Fatalf("expected deriveCPRegion to return first derived region %v, got %v", regions[0], region)
	}
}

func TestDeriveCPRegionsIncludesAlternativeHorizontalCoverage(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
		NameBand:    image.Rect(338, 2004, 2074, 2274),
	}

	regions := deriveCPRegions(layout, bounds)
	if len(regions) < 3 {
		t.Fatalf("expected at least 3 CP regions for resilient extraction, got %d", len(regions))
	}

	primary := regions[0]
	hasFurtherLeft := false
	hasFurtherRight := false
	for _, region := range regions[1:] {
		if region.Min.X < primary.Min.X {
			hasFurtherLeft = true
		}
		if region.Max.X > primary.Max.X {
			hasFurtherRight = true
		}
	}

	if !hasFurtherLeft || !hasFurtherRight {
		t.Fatalf("expected alternate CP regions to extend both sides of primary %v, got %v", primary, regions)
	}
}

func TestBuildCPStrictTextStrategyImageForFloodedNearWhiteRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 420, 140))
	fillRGBA(img, img.Bounds(), color.RGBA{R: 16, G: 16, B: 16, A: 255})

	// Simulate bright, low-chroma background near CP that can flood near-white detection.
	flood := image.Rect(20, 22, 400, 118)
	fillRGBA(img, flood, color.RGBA{R: 190, G: 190, B: 190, A: 255})

	// Simulate CP glyph strokes with brighter pixels.
	fillRGBA(img, image.Rect(142, 56, 154, 84), color.RGBA{R: 246, G: 246, B: 246, A: 255})
	fillRGBA(img, image.Rect(170, 56, 182, 84), color.RGBA{R: 246, G: 246, B: 246, A: 255})
	fillRGBA(img, image.Rect(198, 56, 212, 84), color.RGBA{R: 246, G: 246, B: 246, A: 255})
	fillRGBA(img, image.Rect(228, 56, 246, 84), color.RGBA{R: 246, G: 246, B: 246, A: 255})

	mask, ok := buildCPStrictTextStrategyImage(img)
	if !ok {
		t.Fatal("expected strict strategy image to be generated")
	}
	nonZero := countNonZeroGrayPixels(mask)
	if nonZero == 0 {
		t.Fatal("expected strict strategy to recover CP foreground pixels")
	}

	// Ensure fallback did not keep the entire flooded background.
	if mask.GrayAt(40, 40).Y != 0 {
		t.Fatalf("expected flooded background to be removed, got %d", mask.GrayAt(40, 40).Y)
	}

	if mask.GrayAt(145, 70).Y == 0 || mask.GrayAt(205, 70).Y == 0 || mask.GrayAt(235, 70).Y == 0 {
		t.Fatal("expected bright CP glyph pixels to remain in strict mask")
	}
}

func TestBuildCPTextFocusedImageKeepsStandardMaskWhenNearWhiteComponentsAreAlreadyValid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 280, 120))
	fillRGBA(img, img.Bounds(), color.RGBA{R: 8, G: 8, B: 8, A: 255})

	// Build isolated near-white components that should pass the standard filter.
	fillRGBA(img, image.Rect(82, 46, 92, 78), color.RGBA{R: 236, G: 236, B: 236, A: 255})
	fillRGBA(img, image.Rect(102, 46, 112, 78), color.RGBA{R: 236, G: 236, B: 236, A: 255})
	fillRGBA(img, image.Rect(122, 46, 142, 76), color.RGBA{R: 236, G: 236, B: 236, A: 255})

	mask := buildCPTextFocusedImage(img)
	if mask.GrayAt(85, 60).Y == 0 || mask.GrayAt(126, 60).Y == 0 {
		t.Fatal("expected valid near-white components to be preserved")
	}
	if mask.GrayAt(20, 20).Y != 0 {
		t.Fatalf("expected dark background to remain filtered out, got %d", mask.GrayAt(20, 20).Y)
	}
}

func TestBuildCPStrictTextStrategyImageSkippedWhenStandardMaskIsValid(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 280, 120))
	fillRGBA(img, img.Bounds(), color.RGBA{R: 8, G: 8, B: 8, A: 255})
	fillRGBA(img, image.Rect(82, 46, 92, 78), color.RGBA{R: 236, G: 236, B: 236, A: 255})
	fillRGBA(img, image.Rect(102, 46, 112, 78), color.RGBA{R: 236, G: 236, B: 236, A: 255})

	strictMask, ok := buildCPStrictTextStrategyImage(img)
	if ok || strictMask != nil {
		t.Fatal("expected strict CP strategy to be skipped when standard mask already has valid components")
	}
}

func TestBuildCPExtractionAttemptsIncludesStrictStrategyWithSameWeight(t *testing.T) {
	cleaned := image.NewGray(image.Rect(0, 0, 40, 20))
	raw := image.NewGray(image.Rect(0, 0, 40, 20))
	strict := image.NewGray(image.Rect(0, 0, 40, 20))

	attempts := buildCPExtractionAttempts(1, cleaned, raw, strict)
	if len(attempts) != 12 {
		t.Fatalf("expected 12 attempts with strict strategy enabled, got %d", len(attempts))
	}

	countByLabel := map[string]int{}
	for _, attempt := range attempts {
		countByLabel[attempt.AttemptLabel]++
	}

	if countByLabel["cleaned"] != 3 || countByLabel["cleaned_digits_only"] != 1 {
		t.Fatalf("expected cleaned strategy weight 4 attempts, got %#v", countByLabel)
	}
	if countByLabel["raw_decoded"] != 3 || countByLabel["raw_decoded_digits_only"] != 1 {
		t.Fatalf("expected raw strategy weight 4 attempts, got %#v", countByLabel)
	}
	if countByLabel["strict_cleaned"] != 3 || countByLabel["strict_cleaned_digits_only"] != 1 {
		t.Fatalf("expected strict strategy weight 4 attempts, got %#v", countByLabel)
	}
}

func TestSelectBestCPAttemptPrefersPlausibleCandidate(t *testing.T) {
	cpA := "38"
	cpB := "387"
	results := []cpOCRAttemptResult{
		{
			AttemptNumber: 1,
			RawText:       "cP38",
			ParsedCPRaw:   &cpA,
		},
		{
			AttemptNumber: 2,
			RawText:       "387",
			ParsedCPRaw:   &cpB,
		},
	}

	best, ok := selectBestCPAttempt(results)
	if !ok {
		t.Fatal("expected best CP attempt")
	}
	if best.AttemptNumber != 2 {
		t.Fatalf("expected attempt 2 to win, got attempt %d", best.AttemptNumber)
	}
}

func TestSelectBestCPAttemptReturnsFalseForNoParsedValues(t *testing.T) {
	results := []cpOCRAttemptResult{
		{AttemptNumber: 1, RawText: "C"},
		{AttemptNumber: 2, RawText: ""},
	}

	_, ok := selectBestCPAttempt(results)
	if ok {
		t.Fatal("expected no best CP attempt when all parses are nil")
	}
}

func TestRankCPAttemptCandidatesPrefersCrossRegionConsensus(t *testing.T) {
	cpHigh := "980"
	cpLow := "280"
	results := []cpOCRAttemptResult{
		{RegionNumber: 1, AttemptNumber: 1, RawText: "CP 280", ParsedCPRaw: &cpLow},
		{RegionNumber: 1, AttemptNumber: 2, RawText: "CP 280", ParsedCPRaw: &cpLow},
		{RegionNumber: 1, AttemptNumber: 3, RawText: "CP 280", ParsedCPRaw: &cpLow},
		{RegionNumber: 2, AttemptNumber: 1, RawText: "CP 980", ParsedCPRaw: &cpHigh},
		{RegionNumber: 2, AttemptNumber: 2, RawText: "CP 980", ParsedCPRaw: &cpHigh},
		{RegionNumber: 3, AttemptNumber: 1, RawText: "CP 980", ParsedCPRaw: &cpHigh},
		{RegionNumber: 3, AttemptNumber: 2, RawText: "CP 980", ParsedCPRaw: &cpHigh},
	}

	ranked := rankCPAttemptCandidates(results)
	if len(ranked) < 2 {
		t.Fatalf("expected at least two ranked CP candidates, got %v", ranked)
	}
	if ranked[0] != cpHigh {
		t.Fatalf("expected top ranked CP to be %q, got %q", cpHigh, ranked[0])
	}
}

func TestSelectPreferredCPAttemptUsesRankedConsensusValue(t *testing.T) {
	cpHigh := "980"
	cpLow := "280"
	results := []cpOCRAttemptResult{
		{RegionNumber: 1, AttemptNumber: 1, RawText: "CP 280", ParsedCPRaw: &cpLow},
		{RegionNumber: 2, AttemptNumber: 1, RawText: "CP 980", ParsedCPRaw: &cpHigh},
		{RegionNumber: 2, AttemptNumber: 2, RawText: "980", ParsedCPRaw: &cpHigh},
	}
	ranked := []string{cpHigh, cpLow}

	selected, ok := selectPreferredCPAttempt(results, ranked)
	if !ok {
		t.Fatal("expected preferred CP attempt")
	}
	if selected.ParsedCPRaw == nil || *selected.ParsedCPRaw != cpHigh {
		t.Fatalf("expected selected cp %q, got %#v", cpHigh, selected.ParsedCPRaw)
	}
	if selected.RegionNumber != 2 {
		t.Fatalf("expected selected region 2 for consensus cp %q, got region %d", cpHigh, selected.RegionNumber)
	}
}

func TestBuildCPRetryCandidatesKeepsTopWhenCurrentIsNotTop(t *testing.T) {
	current := "280"
	ranked := []string{"980", "280", "9280"}

	retries := buildCPRetryCandidates(&current, ranked)
	if len(retries) != 2 {
		t.Fatalf("expected 2 retries, got %d (%v)", len(retries), retries)
	}
	if retries[0] != "980" || retries[1] != "9280" {
		t.Fatalf("expected retry order [980 9280], got %v", retries)
	}
}

func TestBuildCPRetryCandidatesSkipsEmptyAndCurrent(t *testing.T) {
	current := "980"
	ranked := []string{"", "980", "980", "280"}

	retries := buildCPRetryCandidates(&current, ranked)
	if len(retries) != 1 || retries[0] != "280" {
		t.Fatalf("expected retries [280], got %v", retries)
	}
}

func TestDeriveHPRegionTargetsAreaBelowDetectedHPBar(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
		CardBounds:  image.Rect(66, 1740, 2346, 5042),
		HPBarY:      2240,
	}

	regions := deriveHPRegions(layout, bounds)
	if len(regions) == 0 {
		t.Fatal("expected hp regions")
	}

	region := regions[0]
	if region.Min.Y <= layout.HPBarY {
		t.Fatalf("expected HP region below HP bar (hpY=%d), got %v", layout.HPBarY, region)
	}
	if region.Max.Y >= layout.CardBounds.Max.Y {
		t.Fatalf("expected HP region to stay inside card bounds, got %v (card %v)", region, layout.CardBounds)
	}
}

func TestDeriveHPRegionFallsBackWhenCardMissing(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
	}

	region := deriveHPRegion(layout, bounds)
	if region.Empty() {
		t.Fatal("expected fallback HP region")
	}
	if region.Min.Y < int(float64(bounds.Dy())*0.30) || region.Max.Y > int(float64(bounds.Dy())*0.52) {
		t.Fatalf("expected fallback HP region in middle-lower band, got %v", region)
	}
}

func TestDeriveIVRegionTargetsLowerCardArea(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	layout := appraisal.NameLayout{
		ImageBounds: bounds,
		CardBounds:  image.Rect(66, 1740, 2346, 5042),
	}

	regions := deriveIVRegions(layout, bounds)
	if len(regions) == 0 {
		t.Fatal("expected iv regions")
	}

	region := regions[0]
	if region.Min.Y < layout.CardBounds.Min.Y+int(float64(layout.CardBounds.Dy())*0.50) {
		t.Fatalf("expected IV region in lower half of card, got %v", region)
	}
	if region.Max.Y < layout.CardBounds.Min.Y+int(float64(layout.CardBounds.Dy())*0.90) {
		t.Fatalf("expected IV region to include the lower HP bar, got %v", region)
	}
	if region.Max.Y > layout.CardBounds.Max.Y {
		t.Fatalf("expected IV region to stay inside card bounds, got %v (card %v)", region, layout.CardBounds)
	}
	if region.Max.X > layout.CardBounds.Min.X+int(float64(layout.CardBounds.Dx())*0.72) {
		t.Fatalf("expected IV region to stay left-focused on appraisal bars, got %v", region)
	}
}

func TestEstimateIVRawFromBarsMeasuresRelativeFill(t *testing.T) {
	panel := syntheticIVBarsImage(15, 10, 10)

	parsed, measurements, _ := estimateIVRawFromBars(panel)
	if len(measurements) != 3 {
		t.Fatalf("expected 3 measured bars, got %d", len(measurements))
	}
	if parsed.AttackRaw == nil || *parsed.AttackRaw != "15" {
		t.Fatalf("expected attack 15 from bar ratio, got %s", ivRawString(parsed.AttackRaw))
	}
	if !assertIVApproxValue(t, parsed.DefenseRaw, 10) {
		t.Fatalf("expected defense approximately %d from bar ratio, got %s", 10, ivRawString(parsed.DefenseRaw))
	}
	if !assertIVApproxValue(t, parsed.StaminaRaw, 10) {
		t.Fatalf("expected stamina approximately %d from bar ratio, got %s", 10, ivRawString(parsed.StaminaRaw))
	}
}

func TestEstimateIVRawFromBarsOnKnownFixtures(t *testing.T) {
	cases := []struct {
		filePath string
		attack   int
		defense  int
		stamina  int
	}{
		{
			filePath: filepath.Join("..", "..", "testdata", "images", "valid__species-munna__cp-824__hp-141__iv-0-13-13__lvl-290.jpeg"),
			attack:   0,
			defense:  13,
			stamina:  13,
		},
		{
			filePath: filepath.Join("..", "..", "testdata", "images", "valid__species-zacian__cp-2223__hp-123__iv-15-13-13__lvl-205.PNG"),
			attack:   15,
			defense:  13,
			stamina:  13,
		},
		{
			filePath: filepath.Join("..", "..", "testdata", "images", "valid__species-grookey__cp-615__hp-89__iv-14-9-9__lvl-210.jpeg"),
			attack:   14,
			defense:  9,
			stamina:  9,
		},
	}

	for _, tc := range cases {
		t.Run(filepath.Base(tc.filePath), func(t *testing.T) {
			decoded, err := imageproc.DecodeFile(tc.filePath)
			if err != nil {
				t.Fatalf("expected fixture decode to succeed: %v", err)
			}

			layout := appraisal.DetectNameLayout(decoded.Image)
			ivRegion := deriveIVRegion(layout, decoded.Image.Bounds())
			if ivRegion.Empty() {
				t.Fatal("expected non-empty IV region")
			}

			rawCrop, err := ocrInputImage(ocr.ExtractRequest{
				Image:  decoded.Image,
				Region: ivRegion,
			})
			if err != nil {
				t.Fatalf("expected IV crop to succeed: %v", err)
			}

			parsed, _, _ := estimateIVRawFromBars(rawCrop)
			if !assertIVApproxWithTolerance(t, parsed.AttackRaw, tc.attack, 2) {
				t.Fatalf("expected attack approximately %d (+/-2), got %s", tc.attack, ivRawString(parsed.AttackRaw))
			}
			if !assertIVApproxWithTolerance(t, parsed.DefenseRaw, tc.defense, 2) {
				t.Fatalf("expected defense approximately %d (+/-2), got %s", tc.defense, ivRawString(parsed.DefenseRaw))
			}
			if !assertIVApproxWithTolerance(t, parsed.StaminaRaw, tc.stamina, 2) {
				t.Fatalf("expected stamina approximately %d (+/-2), got %s", tc.stamina, ivRawString(parsed.StaminaRaw))
			}
		})
	}
}

func assertIVApproxValue(t *testing.T, raw *string, expected int) bool {
	t.Helper()
	return assertIVApproxWithTolerance(t, raw, expected, 1)
}

func assertIVApproxWithTolerance(t *testing.T, raw *string, expected int, tolerance int) bool {
	t.Helper()
	if raw == nil {
		return false
	}
	value, err := strconv.Atoi(*raw)
	if err != nil {
		return false
	}
	delta := value - expected
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}

func ivRawString(raw *string) string {
	if raw == nil {
		return "<nil>"
	}
	return *raw
}

func TestSelectCanonicalSpeciesAttemptPrefersHigherModeRank(t *testing.T) {
	mkParsed := func(species string) appraisal.ParsedCandidate {
		raw := species
		normalized := strings.ToLower(species)
		return appraisal.ParsedCandidate{
			SpeciesNameRaw:        &raw,
			SpeciesNameNormalized: &normalized,
		}
	}

	results := []speciesOCRAttemptResult{
		{AttemptNumber: 1, RawText: "Nidoqveen", Parsed: mkParsed("Nidoqveen"), Score: 55},
		{AttemptNumber: 2, RawText: "Rhyhorn", Parsed: mkParsed("Rhyhorn"), Score: 10},
	}

	winner, ok := selectCanonicalSpeciesAttempt(results, []string{"nidoqueen", "rhyhorn"})
	if !ok {
		t.Fatal("expected winner to be selected")
	}

	if winner.Parsed.SpeciesNameNormalized == nil || *winner.Parsed.SpeciesNameNormalized != "rhyhorn" {
		t.Fatalf("expected exact-match winner species rhyhorn, got %#v", winner.Parsed.SpeciesNameNormalized)
	}
	if winner.AttemptNumber != 2 {
		t.Fatalf("expected second attempt to win due to exact canonical mode, got attempt %d", winner.AttemptNumber)
	}
}

func TestSelectCanonicalSpeciesAttemptReturnsFalseWhenNoParsedSpecies(t *testing.T) {
	results := []speciesOCRAttemptResult{
		{AttemptNumber: 1, RawText: "", Score: 0},
		{AttemptNumber: 2, RawText: "", Score: 0},
	}

	_, ok := selectCanonicalSpeciesAttempt(results, []string{"zacian"})
	if ok {
		t.Fatal("expected no winner when no parsed species exists")
	}
}

func TestSelectCanonicalSpeciesAttemptPrefersEarlierAttemptForSameMode(t *testing.T) {
	mkParsed := func(species string) appraisal.ParsedCandidate {
		raw := species
		normalized := strings.ToLower(species)
		return appraisal.ParsedCandidate{
			SpeciesNameRaw:        &raw,
			SpeciesNameNormalized: &normalized,
		}
	}

	results := []speciesOCRAttemptResult{
		{AttemptNumber: 1, RawText: "Nidoqveen", Parsed: mkParsed("Nidoqveen"), Score: 55},
		{AttemptNumber: 2, RawText: "Nidoqveen", Parsed: mkParsed("Nidoqveen"), Score: 10},
	}

	winner, ok := selectCanonicalSpeciesAttempt(results, []string{"nidoqueen"})
	if !ok {
		t.Fatal("expected winner to be selected")
	}
	if winner.AttemptNumber != 1 {
		t.Fatalf("expected first attempt to win for equal mode/distance, got attempt %d", winner.AttemptNumber)
	}
}

func TestSelectCanonicalSpeciesAttemptReturnsFalseWhenNoCatalogMatch(t *testing.T) {
	mkParsed := func(species string) appraisal.ParsedCandidate {
		raw := species
		normalized := strings.ToLower(species)
		return appraisal.ParsedCandidate{
			SpeciesNameRaw:        &raw,
			SpeciesNameNormalized: &normalized,
		}
	}

	results := []speciesOCRAttemptResult{
		{AttemptNumber: 1, RawText: "Lallall", Parsed: mkParsed("Lallall"), Score: 40},
		{AttemptNumber: 2, RawText: "Et S I Al", Parsed: mkParsed("Et S I Al"), Score: 7},
	}

	_, ok := selectCanonicalSpeciesAttempt(results, []string{"zacian", "rhyhorn"})
	if ok {
		t.Fatal("expected no winner when no candidate matches catalog")
	}
}

func TestSelectCanonicalSpeciesAttemptCanSelectFuzzyCanonicalMatch(t *testing.T) {
	mkParsed := func(species string) appraisal.ParsedCandidate {
		raw := species
		normalized := strings.ToLower(species)
		return appraisal.ParsedCandidate{
			SpeciesNameRaw:        &raw,
			SpeciesNameNormalized: &normalized,
		}
	}

	results := []speciesOCRAttemptResult{
		{AttemptNumber: 1, RawText: "Nidoqveen", Parsed: mkParsed("Nidoqveen"), Score: 55},
		{AttemptNumber: 2, RawText: "Nidoqveen", Parsed: mkParsed("Nidoqveen"), Score: 52},
	}

	winner, ok := selectCanonicalSpeciesAttempt(results, []string{"nidoqueen", "rhyhorn"})
	if !ok {
		t.Fatal("expected fuzzy canonical winner")
	}
	if winner.Canonical == nil {
		t.Fatal("expected canonical metadata on winner")
	}
	if winner.Canonical.SpeciesNormalized != "nidoqueen" {
		t.Fatalf("expected canonical species nidoqueen, got %q", winner.Canonical.SpeciesNormalized)
	}
	if winner.Canonical.Mode != "fuzzy" {
		t.Fatalf("expected fuzzy mode, got %q", winner.Canonical.Mode)
	}
}

func TestResolveMediaForProcessingUploadThingDownloadsHTTPSURL(t *testing.T) {
	expectedBody := []byte("downloadable-media")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		_, _ = w.Write(expectedBody)
	}))
	defer server.Close()

	processor := imageProcessor{
		storage: config.StorageConfig{
			Mode:                           config.UploadStorageModeUploadThing,
			UploadThingDownloadTimeoutSecs: 5,
			UploadThingDownloadRetryCount:  0,
			UploadThingDownloadTempDir:     t.TempDir(),
		},
	}

	localPath, cleanup, err := processor.resolveMediaForProcessing(context.Background(), server.URL+"/media.bin")
	if err != nil {
		t.Fatalf("expected uploadthing media to download successfully, got: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup callback")
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("expected downloaded media file to be readable: %v", err)
	}
	if string(content) != string(expectedBody) {
		t.Fatalf("expected downloaded media %q, got %q", string(expectedBody), string(content))
	}

	if err := cleanup(); err != nil {
		t.Fatalf("expected downloaded media cleanup to succeed, got: %v", err)
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected downloaded media file to be removed, stat err: %v", err)
	}
}

func TestResolveMediaForProcessingUploadThingRetriesTransientFailures(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("temporary upstream issue"))
			return
		}
		_, _ = w.Write([]byte("retry-succeeded"))
	}))
	defer server.Close()

	processor := imageProcessor{
		storage: config.StorageConfig{
			Mode:                           config.UploadStorageModeUploadThing,
			UploadThingDownloadTimeoutSecs: 5,
			UploadThingDownloadRetryCount:  1,
			UploadThingDownloadTempDir:     t.TempDir(),
		},
	}

	localPath, cleanup, err := processor.resolveMediaForProcessing(context.Background(), server.URL+"/media.bin")
	if err != nil {
		t.Fatalf("expected uploadthing media retry path to succeed, got: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 download attempts, got %d", attempts)
	}
	_ = cleanup()
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove downloaded file, stat err: %v", err)
	}
}

func TestResolveMediaForProcessingUploadThingRejectsOversizedDownloads(t *testing.T) {
	originalMaxRemoteMediaBytes := maxRemoteMediaBytes
	maxRemoteMediaBytes = 8
	t.Cleanup(func() {
		maxRemoteMediaBytes = originalMaxRemoteMediaBytes
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer server.Close()

	processor := imageProcessor{
		storage: config.StorageConfig{
			Mode:                           config.UploadStorageModeUploadThing,
			UploadThingDownloadTimeoutSecs: 5,
			UploadThingDownloadRetryCount:  0,
			UploadThingDownloadTempDir:     t.TempDir(),
		},
	}

	_, _, err := processor.resolveMediaForProcessing(context.Background(), server.URL+"/oversized.bin")
	if err == nil {
		t.Fatal("expected oversized uploadthing download to fail")
	}
	if !strings.Contains(err.Error(), "exceeds max size") {
		t.Fatalf("expected max-size error, got: %v", err)
	}
}

func newTestDB(t *testing.T, databasePath string) *sql.DB {
	t.Helper()

	db, err := sql.Open("libsql", "file:"+databasePath)
	if err != nil {
		t.Fatalf("expected sqlite open to succeed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

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
);`

	if err := execSchemaStatements(context.Background(), db, schema); err != nil {
		t.Fatalf("expected schema bootstrap to succeed: %v", err)
	}

	return db
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

func assertTableRowCount(t *testing.T, db *sql.DB, tableName string, expected int) {
	t.Helper()

	query := "SELECT COUNT(*) FROM " + tableName
	var count int
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("expected row count query for %s to succeed: %v", tableName, err)
	}
	if count != expected {
		t.Fatalf("expected %d rows in %s, got %d", expected, tableName, count)
	}
}

func assertValidJSONColumn(t *testing.T, columnName string, value sql.NullString) {
	t.Helper()

	if !value.Valid {
		t.Fatalf("expected %s to be non-null JSON", columnName)
	}
	if !json.Valid([]byte(value.String)) {
		t.Fatalf("expected %s to contain valid JSON, got %q", columnName, value.String)
	}
}

func assertStructuredMetaEntries(
	t *testing.T,
	columnName string,
	columnValue sql.NullString,
	entriesField string,
) {
	t.Helper()

	if !columnValue.Valid {
		t.Fatalf("expected %s to be non-null", columnName)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(columnValue.String), &payload); err != nil {
		t.Fatalf("expected %s to decode as object: %v", columnName, err)
	}

	rawEntries, ok := payload[entriesField]
	if !ok {
		t.Fatalf("expected %s to include %q", columnName, entriesField)
	}
	entries, ok := rawEntries.([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("expected %s.%s to be a non-empty array, got %#v", columnName, entriesField, rawEntries)
	}

	firstEntry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("expected %s.%s[0] to be an object, got %#v", columnName, entriesField, entries[0])
	}
	content, ok := firstEntry["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected %s.%s[0].content to be an object, got %#v", columnName, entriesField, firstEntry["content"])
	}
	if _, ok := content["flat"].(map[string]any); !ok {
		t.Fatalf("expected %s.%s[0].content.flat to be an object, got %#v", columnName, entriesField, content["flat"])
	}
	if _, ok := content["lines"].([]any); !ok {
		t.Fatalf("expected %s.%s[0].content.lines to be an array, got %#v", columnName, entriesField, content["lines"])
	}
}

func seedUploadAndJob(
	t *testing.T,
	db *sql.DB,
	uploadID string,
	jobID string,
	sessionID string,
	kind string,
	mediaURL string,
	now time.Time,
) {
	t.Helper()

	const insertUpload = `
INSERT INTO uploads(id, session_id, kind, uploadthing_url, content_type, byte_size, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?);`
	if _, err := db.ExecContext(
		context.Background(),
		insertUpload,
		uploadID,
		sessionID,
		kind,
		mediaURL,
		"image/png",
		2048,
		now.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected upload seed insert to succeed: %v", err)
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
		jobqueue.JobStatusProcessing,
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
		t.Fatalf("expected job seed insert to succeed: %v", err)
	}
}

func syntheticIVBarsImage(attack int, defense int, stamina int) image.Image {
	const (
		width     = 640
		height    = 420
		barX      = 56
		barWidth  = 420
		barHeight = 16
		rowGap    = 76
		firstRowY = 184
		dividerW  = 2
		panelPadX = 20
		panelPadY = 18
		panelMinY = 70
		panelMaxY = 390
	)

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 22, G: 46, B: 91, A: 255})
		}
	}

	for y := panelMinY; y < panelMaxY; y++ {
		for x := panelPadX; x < width-panelPadX; x++ {
			img.Set(x, y, color.RGBA{R: 248, G: 248, B: 248, A: 255})
		}
	}

	values := []int{attack, defense, stamina}
	for idx, value := range values {
		if value < 0 {
			value = 0
		}
		if value > 15 {
			value = 15
		}

		rowY := firstRowY + idx*rowGap
		for y := rowY; y < rowY+barHeight; y++ {
			for x := barX; x < barX+barWidth; x++ {
				img.Set(x, y, color.RGBA{R: 214, G: 214, B: 214, A: 255})
			}
		}

		fillW := (barWidth*value + 7) / 15
		if fillW > 0 {
			for y := rowY; y < rowY+barHeight; y++ {
				for x := barX; x < barX+fillW; x++ {
					img.Set(x, y, color.RGBA{R: 234, G: 154, B: 63, A: 255})
				}
			}
		}

		firstDivider := barX + barWidth/3
		secondDivider := barX + (barWidth*2)/3
		for y := rowY; y < rowY+barHeight; y++ {
			for x := firstDivider; x < firstDivider+dividerW; x++ {
				img.Set(x, y, color.RGBA{R: 248, G: 248, B: 248, A: 255})
			}
			for x := secondDivider; x < secondDivider+dividerW; x++ {
				img.Set(x, y, color.RGBA{R: 248, G: 248, B: 248, A: 255})
			}
		}
	}

	return img
}

func syntheticVideoFrameImage() image.Image {
	return syntheticVideoFrameImageWithCardShift(0)
}

func syntheticVideoFrameImageWithCardShift(shiftX int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	fillRGBA(img, img.Bounds(), color.RGBA{R: 42, G: 52, B: 70, A: 255})

	card := image.Rect(140+shiftX, 620, 940+shiftX, 1820).Intersect(img.Bounds())
	fillRGBA(img, card, color.RGBA{R: 246, G: 246, B: 246, A: 255})

	hpY := card.Min.Y + int(float64(card.Dy())*0.24)
	hpRect := image.Rect(
		card.Min.X+int(float64(card.Dx())*0.20),
		hpY-2,
		card.Max.X-int(float64(card.Dx())*0.20),
		hpY+3,
	).Intersect(img.Bounds())
	fillRGBA(img, hpRect, color.RGBA{R: 88, G: 208, B: 128, A: 255})

	return img
}

func fillRGBA(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	r := rect.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func createTestPNG(t *testing.T, filePath string) {
	t.Helper()

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("expected test png creation to succeed: %v", err)
	}
	defer file.Close()

	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.Set(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	if err := png.Encode(file, img); err != nil {
		t.Fatalf("expected test png encoding to succeed: %v", err)
	}
}

func mustLoadTestSpeciesCatalog(t *testing.T) species.Catalog {
	t.Helper()

	testSpeciesCatalogOnce.Do(func() {
		_, currentFile, _, ok := runtime.Caller(0)
		if !ok {
			testSpeciesCatalogErr = errors.New("resolve current test path")
			return
		}
		catalogPath := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "pokemon.json"))
		testSpeciesCatalog, testSpeciesCatalogErr = species.LoadCatalogFromFile(catalogPath)
	})

	if testSpeciesCatalogErr != nil {
		t.Fatalf("expected test species catalog to load: %v", testSpeciesCatalogErr)
	}

	return testSpeciesCatalog
}

func mustBuildAmbiguousTestCatalog(t *testing.T) species.Catalog {
	t.Helper()

	sharedStats := species.BaseStats{
		Attack:  120,
		Defense: 120,
		Stamina: 120,
	}

	catalog, err := species.NewCatalog([]species.Entry{
		{
			SpeciesID:         "mr_mime",
			SpeciesName:       "Mr. Mime",
			SpeciesNormalized: "mr. mime",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "mr_mime_galarian",
			SpeciesName:       "Mr. Mime (Galarian)",
			SpeciesNormalized: "mr. mime (galarian)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "mr_mime_shadow",
			SpeciesName:       "Mr. Mime (Shadow)",
			SpeciesNormalized: "mr. mime (shadow)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka",
			SpeciesName:       "Darumaka",
			SpeciesNormalized: "darumaka",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka_galarian",
			SpeciesName:       "Darumaka (Galarian)",
			SpeciesNormalized: "darumaka (galarian)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka_shadow",
			SpeciesName:       "Darumaka (Shadow)",
			SpeciesNormalized: "darumaka (shadow)",
			BaseStats:         sharedStats,
		},
	})
	if err != nil {
		t.Fatalf("expected ambiguous test catalog to build: %v", err)
	}
	return catalog
}

func testStringPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}

func countNonZeroGrayPixels(img *image.Gray) int {
	if img == nil {
		return 0
	}

	count := 0
	for _, px := range img.Pix {
		if px > 0 {
			count++
		}
	}
	return count
}
