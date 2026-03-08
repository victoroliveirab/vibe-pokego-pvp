package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/debugtrace"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/imageproc"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/ocr"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/videoproc"
)

const (
	progressDownloadingMedia    = 6
	progressDecodingImage       = 22
	progressDecodingVideo       = 22
	progressSamplingFrames      = 34
	progressExtractingAppraisal = 74
	progressPostprocessing      = 88
	progressPersistingResults   = 96

	speciesNamePageSegMode = 7
	cpPageSegModePrimary   = 7
	hpPageSegModePrimary   = 7
	cpPageSegModeSecondary = 6

	minStableVideoFrameStreak = 2
)

var maxRemoteMediaBytes int64 = 75 * 1024 * 1024

type imageProcessor struct {
	databasePath   string
	storage        config.StorageConfig
	ocrEngine      ocr.Engine
	stepDelay      time.Duration
	speciesCatalog species.Catalog
	videoSampler   videoproc.Sampler
}

type imageArtifactWriter struct {
	jobDir      string
	seq         int
	textEntries []artifactTextEntry
}

type artifactTextEntry struct {
	Label   string
	Content string
}

type speciesOCRAttemptResult struct {
	AttemptNumber  int
	AttemptLabel   string
	Request        ocr.ExtractRequest
	RawText        string
	Parsed         appraisal.ParsedCandidate
	Score          int
	Canonical      *appraisal.CanonicalSpeciesMatch
	SelectionScore float64
	OCRError       error
}

type cpOCRAttemptResult struct {
	RegionNumber  int
	AttemptNumber int
	AttemptLabel  string
	Request       ocr.ExtractRequest
	RawText       string
	ParsedCPRaw   *string
	OCRError      error
}

type hpOCRAttemptResult struct {
	RegionNumber  int
	AttemptNumber int
	AttemptLabel  string
	Request       ocr.ExtractRequest
	RawText       string
	ParsedHPRaw   *string
	OCRError      error
}

type ivBarMeasurement struct {
	BarIndex   int
	RowTop     int
	RowBottom  int
	TrackStart int
	TrackEnd   int
	FillEnd    int
	TrackWidth int
	FillWidth  int
	Value      int
	Ratio      float64
}

type pendingUserDedupSignal struct{}

func (pendingUserDedupSignal) Error() string {
	return "pending user species dedup required"
}

func newImageProcessor(
	databasePath string,
	storage config.StorageConfig,
	heartbeatInterval time.Duration,
	ocrEngine ocr.Engine,
	speciesCatalog species.Catalog,
	videoSampler videoproc.Sampler,
) Processor {
	if ocrEngine == nil {
		ocrEngine = ocr.NewTesseractEngine()
	}
	if videoSampler == nil {
		videoSampler = videoproc.NewFFmpegSampler(videoproc.DefaultInterval)
	}

	stepDelay := time.Duration(0)
	if heartbeatInterval > 0 {
		stepDelay = heartbeatInterval + 200*time.Millisecond
	}

	return imageProcessor{
		databasePath:   databasePath,
		storage:        storage,
		ocrEngine:      ocrEngine,
		stepDelay:      stepDelay,
		speciesCatalog: speciesCatalog,
		videoSampler:   videoSampler,
	}
}

func (p imageProcessor) Process(
	ctx context.Context,
	job jobqueue.ClaimedJob,
	reportProgress ProgressReporter,
) (processErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}

	processingStartedAt := time.Now().UTC()

	artifactWriter, err := newImageArtifactWriter(p.databasePath, job.ID)
	if err != nil {
		return ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	debugStore, err := debugtrace.NewSQLiteStore(p.databasePath)
	if err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("open debug trace store: %v", err),
		}
	}
	defer debugStore.Close()

	if err := debugStore.UpsertJobDebug(ctx, debugtrace.UpsertJobDebugParams{
		JobID:               job.ID,
		UploadID:            job.UploadID,
		SessionID:           job.SessionID,
		Kind:                "unknown",
		ProcessingStartedAt: processingStartedAt,
		CreatedAt:           processingStartedAt,
		UpdatedAt:           processingStartedAt,
	}); err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("initialize job debug row: %v", err),
		}
	}

	defer func() {
		finishedAt := time.Now().UTC()
		terminalMeta := map[string]any{
			"finished_at": finishedAt.Format(time.RFC3339Nano),
		}

		var errorCode *string
		var errorMessage *string

		if processErr == nil {
			terminalMeta["outcome"] = "succeeded"
		} else {
			var pendingSignal pendingUserDedupSignal
			if errors.As(processErr, &pendingSignal) {
				terminalMeta["outcome"] = "pending_user_dedup"
			} else {
				terminalMeta["outcome"] = "failed"
				code, message := failureDetailsForError(processErr)
				errorCode = &code
				errorMessage = &message
			}
		}

		terminalMetaJSON := marshalDebugJSON(terminalMeta)
		if _, markErr := debugStore.MarkJobDebugTerminal(ctx, debugtrace.MarkJobDebugTerminalParams{
			JobID:                job.ID,
			ProcessingFinishedAt: finishedAt,
			TerminalMetaJSON:     terminalMetaJSON,
			ErrorCode:            errorCode,
			ErrorMessage:         errorMessage,
			UpdatedAt:            finishedAt,
		}); markErr != nil && processErr == nil {
			processErr = ProcessingError{
				Code:    "PERSIST_FAILED",
				Message: fmt.Sprintf("mark job debug terminal: %v", markErr),
			}
		}
	}()

	if err := p.reportStage(ctx, reportProgress, jobqueue.StageDownloadingMedia, progressDownloadingMedia); err != nil {
		return err
	}

	mediaURL, mediaKind, err := lookupUploadMedia(ctx, p.databasePath, job.UploadID)
	if err != nil {
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: err.Error(),
		}
	}

	now := time.Now().UTC()
	if err := debugStore.UpsertJobDebug(ctx, debugtrace.UpsertJobDebugParams{
		JobID:               job.ID,
		UploadID:            job.UploadID,
		SessionID:           job.SessionID,
		Kind:                mediaKind,
		ProcessingStartedAt: processingStartedAt,
		UpdatedAt:           now,
	}); err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("set job debug kind: %v", err),
		}
	}
	if _, err := debugStore.UpdateJobDebugMilestone(ctx, debugtrace.UpdateJobDebugMilestoneParams{
		JobID:      job.ID,
		Milestone:  debugtrace.MilestoneDownloading,
		FinishedAt: now,
		MetaJSON: marshalDebugJSON(map[string]any{
			"media_url":  mediaURL,
			"media_kind": mediaKind,
		}),
		UpdatedAt: now,
	}); err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("update downloading milestone: %v", err),
		}
	}

	switch mediaKind {
	case "image":
		return p.processImage(ctx, job, reportProgress, artifactWriter, debugStore, mediaURL)
	case "video":
		return p.processVideo(ctx, job, reportProgress, artifactWriter, debugStore, mediaURL)
	default:
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: fmt.Sprintf("unsupported upload kind %q", mediaKind),
		}
	}
}

func (p imageProcessor) reportStage(
	ctx context.Context,
	reportProgress ProgressReporter,
	stage string,
	progress int,
) error {
	if err := reportProgress(stage, progress); err != nil {
		return err
	}

	if p.stepDelay <= 0 {
		return nil
	}

	return sleepWithContext(ctx, p.stepDelay)
}

type processedFrame struct {
	parsed                   appraisal.ParsedCandidate
	rawOCRText               string
	validation               appraisal.ValidationDecision
	timestampMS              *int64
	status                   string
	startedAt                time.Time
	finishedAt               time.Time
	speciesFinishedAt        *time.Time
	cpFinishedAt             *time.Time
	hpFinishedAt             *time.Time
	ivFinishedAt             *time.Time
	layoutMetaJSON           *string
	speciesMetaJSON          *string
	cpMetaJSON               *string
	hpMetaJSON               *string
	ivMetaJSON               *string
	ivBarMeasurementMetaJSON *string
	frameStabilityMetaJSON   *string
	selectionMetaJSON        *string
}

func (p imageProcessor) processImage(
	ctx context.Context,
	job jobqueue.ClaimedJob,
	reportProgress ProgressReporter,
	artifactWriter *imageArtifactWriter,
	debugStore debugtrace.Store,
	mediaURL string,
) error {
	if err := p.reportStage(ctx, reportProgress, jobqueue.StageDecodingImage, progressDecodingImage); err != nil {
		return err
	}

	localPath, cleanupMedia, err := p.resolveMediaForProcessing(ctx, mediaURL)
	if err != nil {
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: err.Error(),
		}
	}
	defer cleanupMedia()

	decodedImage, err := imageproc.DecodeFile(localPath)
	if err != nil {
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: err.Error(),
		}
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestoneDecoding,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"local_path":     localPath,
			"decoded_bounds": decodedImage.Image.Bounds(),
		}),
	); err != nil {
		return err
	}

	if err := p.reportStage(ctx, reportProgress, jobqueue.StageExtractingAppraisal, progressExtractingAppraisal); err != nil {
		return err
	}

	frame, err := p.processFrame(ctx, decodedImage.Image, artifactWriter, "")
	if err != nil {
		_ = insertFrameDebugRow(
			ctx,
			debugStore,
			job,
			appraisal.SourceTypeImage,
			1,
			nil,
			frame,
		)
		return err
	}
	frame.status = debugtrace.FrameStatusProcessed

	if err := insertFrameDebugRow(
		ctx,
		debugStore,
		job,
		appraisal.SourceTypeImage,
		1,
		nil,
		frame,
	); err != nil {
		return err
	}
	if err := updateFrameStepMilestones(ctx, debugStore, job.ID, frame); err != nil {
		return err
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestoneExtracting,
		time.Now().UTC(),
		nil,
	); err != nil {
		return err
	}

	if err := p.reportStage(ctx, reportProgress, jobqueue.StagePostprocessing, progressPostprocessing); err != nil {
		return err
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestonePostprocessing,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"frame_rows_written": 1,
		}),
	); err != nil {
		return err
	}
	if err := p.reportStage(ctx, reportProgress, jobqueue.StagePersistingResults, progressPersistingResults); err != nil {
		return err
	}

	acceptedCount, err := p.persistCandidateAndAccepted(
		ctx,
		job,
		frame.parsed,
		frame.rawOCRText,
		frame.validation,
		appraisal.SourceTypeImage,
		nil,
	)
	if err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: err.Error(),
		}
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestonePersisting,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"accepted_count": acceptedCount,
			"has_pending":    acceptedCount > 1,
		}),
	); err != nil {
		return err
	}

	return finalizeProcessingOutcome(acceptedCount > 1, acceptedCount)
}

func (p imageProcessor) processVideo(
	ctx context.Context,
	job jobqueue.ClaimedJob,
	reportProgress ProgressReporter,
	artifactWriter *imageArtifactWriter,
	debugStore debugtrace.Store,
	mediaURL string,
) error {
	if err := p.reportStage(ctx, reportProgress, jobqueue.StageDecodingVideo, progressDecodingVideo); err != nil {
		return err
	}

	localPath, cleanupMedia, err := p.resolveMediaForProcessing(ctx, mediaURL)
	if err != nil {
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: err.Error(),
		}
	}
	defer cleanupMedia()
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestoneDecoding,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"local_path": localPath,
		}),
	); err != nil {
		return err
	}

	if err := p.reportStage(ctx, reportProgress, jobqueue.StageSamplingFrames, progressSamplingFrames); err != nil {
		return err
	}

	samples, err := p.videoSampler.SampleFrames(ctx, localPath)
	if err != nil {
		return ProcessingError{
			Code:    "DECODE_FAILED",
			Message: err.Error(),
		}
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestoneSampling,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"sample_count": len(samples),
		}),
	); err != nil {
		return err
	}

	if err := p.reportStage(ctx, reportProgress, jobqueue.StageExtractingAppraisal, progressExtractingAppraisal); err != nil {
		return err
	}

	frames := make([]processedFrame, 0, len(samples))
	stabilityEvaluator := videoproc.NewFrameStabilityEvaluator()
	var previousSampleImage image.Image
	stableStreak := 0
	for idx, sample := range samples {
		frameStartedAt := time.Now().UTC()
		labelPrefix := fmt.Sprintf("frame_%04d_%06dms", idx+1, sample.TimestampMS)
		stability := stabilityEvaluator.Assess(sample.Image, previousSampleImage)
		previousSampleImage = sample.Image

		nextStableStreak := 0
		acceptedForOCR := false
		disposition := "jittered"
		if stability.Stable {
			nextStableStreak = stableStreak + 1
			acceptedForOCR = nextStableStreak >= minStableVideoFrameStreak
			if acceptedForOCR {
				disposition = "processed"
			} else {
				disposition = "duplicate"
			}
		}
		stabilitySummary := buildVideoFrameStabilitySummary(
			sample.TimestampMS,
			stability,
			nextStableStreak,
			acceptedForOCR,
		)
		stabilityMetaJSON := marshalDebugJSON(map[string]any{
			"timestamp_ms":       sample.TimestampMS,
			"stable":             stability.Stable,
			"accepted_for_ocr":   acceptedForOCR,
			"stable_streak":      nextStableStreak,
			"reason":             stability.Reason,
			"card_centered":      stability.CardCentered,
			"card_confidence":    stability.CardConfidence,
			"white_ratio":        stability.WhiteRatio,
			"motion_delta_score": stability.MotionDeltaScore,
			"summary_text":       strings.TrimSpace(stabilitySummary),
		})

		if err := writeVideoFrameDispositionImage(artifactWriter, idx+1, disposition, sample.Image); err != nil {
			return ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}

		if err := writeVideoFrameStabilitySummary(
			artifactWriter,
			labelPrefix,
			sample.TimestampMS,
			stability,
			nextStableStreak,
			acceptedForOCR,
		); err != nil {
			return ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}

		frameTimestamp := sample.TimestampMS
		if !stability.Stable {
			frame := processedFrame{
				status:                 debugtrace.FrameStatusSkippedStability,
				startedAt:              frameStartedAt,
				finishedAt:             time.Now().UTC(),
				frameStabilityMetaJSON: stabilityMetaJSON,
			}
			if err := insertFrameDebugRow(
				ctx,
				debugStore,
				job,
				appraisal.SourceTypeVideo,
				idx+1,
				&frameTimestamp,
				frame,
			); err != nil {
				return err
			}
			stableStreak = 0
			continue
		}
		stableStreak = nextStableStreak
		if !acceptedForOCR {
			frame := processedFrame{
				status:                 debugtrace.FrameStatusSkippedStability,
				startedAt:              frameStartedAt,
				finishedAt:             time.Now().UTC(),
				frameStabilityMetaJSON: stabilityMetaJSON,
			}
			if err := insertFrameDebugRow(
				ctx,
				debugStore,
				job,
				appraisal.SourceTypeVideo,
				idx+1,
				&frameTimestamp,
				frame,
			); err != nil {
				return err
			}
			continue
		}

		frame, processErr := p.processFrame(ctx, sample.Image, artifactWriter, labelPrefix)
		if processErr != nil {
			frame.frameStabilityMetaJSON = stabilityMetaJSON
			if err := insertFrameDebugRow(
				ctx,
				debugStore,
				job,
				appraisal.SourceTypeVideo,
				idx+1,
				&frameTimestamp,
				frame,
			); err != nil {
				return err
			}
			return processErr
		}

		frame.timestampMS = &frameTimestamp
		frame.frameStabilityMetaJSON = stabilityMetaJSON
		frame.status = debugtrace.FrameStatusProcessed
		if err := insertFrameDebugRow(
			ctx,
			debugStore,
			job,
			appraisal.SourceTypeVideo,
			idx+1,
			&frameTimestamp,
			frame,
		); err != nil {
			return err
		}
		if err := updateFrameStepMilestones(ctx, debugStore, job.ID, frame); err != nil {
			return err
		}

		frames = append(frames, frame)
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestoneExtracting,
		time.Now().UTC(),
		nil,
	); err != nil {
		return err
	}

	if err := p.reportStage(ctx, reportProgress, jobqueue.StagePostprocessing, progressPostprocessing); err != nil {
		return err
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestonePostprocessing,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"processed_frame_count": len(frames),
		}),
	); err != nil {
		return err
	}
	if err := p.reportStage(ctx, reportProgress, jobqueue.StagePersistingResults, progressPersistingResults); err != nil {
		return err
	}

	hasPending, acceptedCount, persistErr := p.persistVideoFrames(ctx, job, frames)
	if persistErr != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: persistErr.Error(),
		}
	}
	if err := updateJobDebugMilestone(
		ctx,
		debugStore,
		job.ID,
		debugtrace.MilestonePersisting,
		time.Now().UTC(),
		marshalDebugJSON(map[string]any{
			"processed_frame_count": len(frames),
			"accepted_count":        acceptedCount,
			"has_pending":           hasPending,
		}),
	); err != nil {
		return err
	}

	return finalizeProcessingOutcome(hasPending, acceptedCount)
}

func (p imageProcessor) resolveMediaForProcessing(ctx context.Context, mediaURL string) (string, func() error, error) {
	switch p.storage.Mode {
	case config.UploadStorageModeLocal:
		localPath, err := resolveLocalMediaPath(mediaURL, p.storage.LocalDir)
		if err != nil {
			return "", nil, err
		}
		return localPath, func() error { return nil }, nil
	case config.UploadStorageModeUploadThing:
		parsed, err := url.Parse(mediaURL)
		if err != nil {
			return "", nil, fmt.Errorf("parse uploadthing media URL: %w", err)
		}

		switch parsed.Scheme {
		case "http", "https":
			return p.downloadUploadThingMedia(ctx, mediaURL)
		case "local":
			if strings.TrimSpace(p.storage.LocalDir) == "" {
				return "", nil, fmt.Errorf("local storage directory is required for local media URLs")
			}
			localPath, err := resolveLocalMediaPath(mediaURL, p.storage.LocalDir)
			if err != nil {
				return "", nil, err
			}
			return localPath, func() error { return nil }, nil
		default:
			return "", nil, fmt.Errorf("unsupported media URL scheme %q", parsed.Scheme)
		}
	default:
		return "", nil, fmt.Errorf("storage mode %q is not supported by image processor", p.storage.Mode)
	}
}

func (p imageProcessor) downloadUploadThingMedia(ctx context.Context, mediaURL string) (string, func() error, error) {
	if _, err := mustParseHTTPMediaURL(mediaURL); err != nil {
		return "", nil, err
	}

	tempDir := strings.TrimSpace(p.storage.UploadThingDownloadTempDir)
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	httpClient := &http.Client{
		Timeout: time.Duration(p.storage.UploadThingDownloadTimeoutSecs) * time.Second,
	}
	attempts := p.storage.UploadThingDownloadRetryCount + 1
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		localPath, err := downloadUploadThingMediaOnce(ctx, httpClient, mediaURL, tempDir)
		if err == nil {
			return localPath, cleanupDownloadedMedia(localPath), nil
		}
		lastErr = err
		if attempt == attempts {
			break
		}
		if err := sleepWithContext(ctx, time.Duration(attempt)*200*time.Millisecond); err != nil {
			return "", nil, err
		}
	}

	return "", nil, lastErr
}

func downloadUploadThingMediaOnce(
	ctx context.Context,
	httpClient *http.Client,
	mediaURL string,
	tempDir string,
) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return "", fmt.Errorf("build media download request: %w", err)
	}

	response, err := httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("download media: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		if readErr != nil {
			return "", fmt.Errorf("download media returned status %d", response.StatusCode)
		}
		return "", fmt.Errorf(
			"download media returned status %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare download temp directory: %w", err)
	}

	tempFile, err := os.CreateTemp(tempDir, "uploadthing-media-*")
	if err != nil {
		return "", fmt.Errorf("create downloaded media temp file: %w", err)
	}
	tempFilePath := tempFile.Name()

	limitedReader := &io.LimitedReader{R: response.Body, N: maxRemoteMediaBytes + 1}
	written, err := io.Copy(tempFile, limitedReader)
	if err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFilePath)
		return "", fmt.Errorf("stream downloaded media: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFilePath)
		return "", fmt.Errorf("close downloaded media temp file: %w", err)
	}
	if written > maxRemoteMediaBytes {
		_ = os.Remove(tempFilePath)
		return "", fmt.Errorf("downloaded media exceeds max size of %d bytes", maxRemoteMediaBytes)
	}
	if written == 0 {
		_ = os.Remove(tempFilePath)
		return "", fmt.Errorf("downloaded media is empty")
	}

	return tempFilePath, nil
}

func cleanupDownloadedMedia(filePath string) func() error {
	return func() error {
		if strings.TrimSpace(filePath) == "" {
			return nil
		}
		if err := os.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove downloaded media temp file: %w", err)
		}
		return nil
	}
}

func mustParseHTTPMediaURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse uploadthing media URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("uploadthing media URL must use http or https scheme")
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("uploadthing media URL must include host")
	}
	return parsed, nil
}

func (p imageProcessor) processFrame(
	ctx context.Context,
	decodedImage image.Image,
	artifactWriter *imageArtifactWriter,
	labelPrefix string,
) (result processedFrame, err error) {
	result = processedFrame{
		status:    debugtrace.FrameStatusProcessed,
		startedAt: time.Now().UTC(),
	}
	textEntryStart := artifactWriter.textEntryCount()
	defer func() {
		enrichFrameWithArtifactText(&result, artifactWriter.textEntriesSince(textEntryStart))
	}()

	if err = artifactWriter.write(artifactLabel(labelPrefix, "decoded"), decodedImage); err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		err = ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
		return result, err
	}

	var preprocessedImage image.Image
	preprocessedImage, err = imageproc.PreprocessForOCR(decodedImage)
	if err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		err = ProcessingError{
			Code:    "PREPROCESS_FAILED",
			Message: err.Error(),
		}
		return result, err
	}
	if err = artifactWriter.write(artifactLabel(labelPrefix, "preprocessed"), preprocessedImage); err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		err = ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
		return result, err
	}

	detectedLayout := appraisal.DetectNameLayout(decodedImage)
	layout := scaleNameLayoutToBounds(detectedLayout, preprocessedImage.Bounds())
	nameROI := appraisal.DetectSpeciesNameROI(preprocessedImage, layout)
	result.layoutMetaJSON = marshalDebugJSON(map[string]any{
		"detected_layout_image_bounds": detectedLayout.ImageBounds,
		"scaled_layout_image_bounds":   layout.ImageBounds,
		"layout_confidence":            layout.Confidence,
		"card_bounds":                  layout.CardBounds,
		"has_card":                     layout.HasCard,
		"card_confidence":              layout.CardConfidence,
		"hp_y":                         layout.HPBarY,
		"has_hp":                       layout.HasHPBar,
		"hp_confidence":                layout.HPConfidence,
		"name_band":                    layout.NameBand,
		"layout_failure_reason":        layout.FailureReason,
		"roi_region":                   nameROI.Region,
		"roi_band":                     nameROI.Band,
		"roi_method":                   nameROI.Method,
		"roi_confidence":               nameROI.Confidence,
		"roi_failure_reason":           nameROI.FailureReason,
	})

	var speciesRawOCR string
	speciesRawOCR, err = p.extractSpeciesName(ctx, decodedImage, preprocessedImage, artifactWriter)
	if err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		return result, err
	}
	speciesFinishedAt := time.Now().UTC()
	result.speciesFinishedAt = &speciesFinishedAt

	parsedCandidate := appraisal.ParseCandidateFromOCR(speciesRawOCR)
	result.speciesMetaJSON = marshalDebugJSON(map[string]any{
		"selected_text":             strings.TrimSpace(speciesRawOCR),
		"parsed_species_raw":        nullableStringValue(parsedCandidate.SpeciesNameRaw),
		"parsed_species_normalized": nullableStringValue(parsedCandidate.SpeciesNameNormalized),
	})

	var cpRawValue *string
	var cpCandidateValues []string
	cpRawValue, cpCandidateValues, err = p.extractCPRaw(ctx, decodedImage, preprocessedImage, artifactWriter)
	if err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		return result, err
	}
	cpFinishedAt := time.Now().UTC()
	result.cpFinishedAt = &cpFinishedAt
	parsedCandidate.CPRaw = cpRawValue
	result.cpMetaJSON = marshalDebugJSON(map[string]any{
		"selected_cp_raw":  nullableStringValue(cpRawValue),
		"candidate_values": cpCandidateValues,
	})

	var hpRawValue *string
	hpRawValue, err = p.extractHPRaw(ctx, decodedImage, preprocessedImage, artifactWriter)
	if err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		return result, err
	}
	hpFinishedAt := time.Now().UTC()
	result.hpFinishedAt = &hpFinishedAt
	parsedCandidate.HPRaw = hpRawValue
	result.hpMetaJSON = marshalDebugJSON(map[string]any{
		"selected_hp_raw": nullableStringValue(hpRawValue),
	})

	var parsedIVRaw appraisal.ParsedIVRaw
	parsedIVRaw, err = p.extractIVRaw(ctx, decodedImage, preprocessedImage, artifactWriter)
	if err != nil {
		result.status = debugtrace.FrameStatusError
		result.finishedAt = time.Now().UTC()
		return result, err
	}
	ivFinishedAt := time.Now().UTC()
	result.ivFinishedAt = &ivFinishedAt
	parsedCandidate.IVAttackRaw = parsedIVRaw.AttackRaw
	parsedCandidate.IVDefenseRaw = parsedIVRaw.DefenseRaw
	parsedCandidate.IVStaminaRaw = parsedIVRaw.StaminaRaw
	result.ivMetaJSON = marshalDebugJSON(map[string]any{
		"selected_iv_attack_raw":  nullableStringValue(parsedIVRaw.AttackRaw),
		"selected_iv_defense_raw": nullableStringValue(parsedIVRaw.DefenseRaw),
		"selected_iv_stamina_raw": nullableStringValue(parsedIVRaw.StaminaRaw),
		"selected_method":         "bar_ratio",
	})
	result.ivBarMeasurementMetaJSON = marshalDebugJSON(map[string]any{
		"parsed_attack":  nullableStringValue(parsedIVRaw.AttackRaw),
		"parsed_defense": nullableStringValue(parsedIVRaw.DefenseRaw),
		"parsed_stamina": nullableStringValue(parsedIVRaw.StaminaRaw),
	})

	validation := appraisal.ValidateCandidate(parsedCandidate, p.speciesCatalog, nil)
	if len(validation.AcceptedResults) == 0 && len(cpCandidateValues) > 1 {
		for _, candidateCPRaw := range buildCPRetryCandidates(parsedCandidate.CPRaw, cpCandidateValues) {
			candidateCPRawCopy := candidateCPRaw
			parsedCandidate.CPRaw = &candidateCPRawCopy

			alternateValidation := appraisal.ValidateCandidate(parsedCandidate, p.speciesCatalog, nil)
			if len(alternateValidation.AcceptedResults) == 0 {
				continue
			}

			validation = alternateValidation
			break
		}
	}

	acceptedSpecies := make([]string, 0, len(validation.AcceptedResults))
	for _, accepted := range validation.AcceptedResults {
		acceptedSpecies = append(acceptedSpecies, accepted.SpeciesName)
	}
	result.selectionMetaJSON = marshalDebugJSON(map[string]any{
		"species_is_canonical":    validation.SpeciesIsCanonical,
		"accepted_results_count":  len(validation.AcceptedResults),
		"accepted_species":        acceptedSpecies,
		"selected_cp_raw":         nullableStringValue(parsedCandidate.CPRaw),
		"selected_hp_raw":         nullableStringValue(parsedCandidate.HPRaw),
		"selected_iv_attack_raw":  nullableStringValue(parsedCandidate.IVAttackRaw),
		"selected_iv_defense_raw": nullableStringValue(parsedCandidate.IVDefenseRaw),
		"selected_iv_stamina_raw": nullableStringValue(parsedCandidate.IVStaminaRaw),
		"cp_retry_candidates":     cpCandidateValues,
	})

	result.finishedAt = time.Now().UTC()
	result.parsed = parsedCandidate
	result.rawOCRText = speciesRawOCR
	result.validation = validation

	return result, nil
}

func artifactLabel(prefix string, base string) string {
	cleanPrefix := strings.TrimSpace(prefix)
	if cleanPrefix == "" {
		return base
	}
	return cleanPrefix + "_" + base
}

func finalizeProcessingOutcome(hasPending bool, acceptedCount int) error {
	if hasPending {
		return pendingUserDedupSignal{}
	}
	if acceptedCount > 0 {
		return nil
	}
	return ProcessingError{
		Code:    errorCodeNoAppraisals,
		Message: errorMessageNoAppraisals,
	}
}

func updateJobDebugMilestone(
	ctx context.Context,
	debugStore debugtrace.Store,
	jobID string,
	milestone string,
	finishedAt time.Time,
	metaJSON *string,
) error {
	updated, err := debugStore.UpdateJobDebugMilestone(ctx, debugtrace.UpdateJobDebugMilestoneParams{
		JobID:      jobID,
		Milestone:  milestone,
		FinishedAt: finishedAt,
		MetaJSON:   metaJSON,
		UpdatedAt:  finishedAt,
	})
	if err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("update debug milestone %q: %v", milestone, err),
		}
	}
	if !updated {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("debug milestone %q not updated for job %s", milestone, jobID),
		}
	}

	return nil
}

func updateFrameStepMilestones(
	ctx context.Context,
	debugStore debugtrace.Store,
	jobID string,
	frame processedFrame,
) error {
	if frame.speciesFinishedAt != nil {
		if err := updateJobDebugMilestone(
			ctx,
			debugStore,
			jobID,
			debugtrace.MilestoneSpecies,
			*frame.speciesFinishedAt,
			nil,
		); err != nil {
			return err
		}
	}
	if frame.cpFinishedAt != nil {
		if err := updateJobDebugMilestone(
			ctx,
			debugStore,
			jobID,
			debugtrace.MilestoneCP,
			*frame.cpFinishedAt,
			nil,
		); err != nil {
			return err
		}
	}
	if frame.hpFinishedAt != nil {
		if err := updateJobDebugMilestone(
			ctx,
			debugStore,
			jobID,
			debugtrace.MilestoneHP,
			*frame.hpFinishedAt,
			nil,
		); err != nil {
			return err
		}
	}
	if frame.ivFinishedAt != nil {
		if err := updateJobDebugMilestone(
			ctx,
			debugStore,
			jobID,
			debugtrace.MilestoneIV,
			*frame.ivFinishedAt,
			nil,
		); err != nil {
			return err
		}
	}

	return nil
}

func enrichFrameWithArtifactText(frame *processedFrame, entries []artifactTextEntry) {
	if frame == nil || len(entries) == 0 {
		return
	}

	allMetaTexts := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		allMetaTexts = append(allMetaTexts, structuredArtifactEntry(entry))
	}
	frame.selectionMetaJSON = mergeDebugJSON(frame.selectionMetaJSON, map[string]any{
		"all_meta_texts": allMetaTexts,
	})

	if layoutText, ok := findArtifactTextByLabel(entries, "layout_detection_meta"); ok {
		frame.layoutMetaJSON = mergeDebugJSON(frame.layoutMetaJSON, map[string]any{
			"layout_detection_meta_text": parseArtifactMetaContent(layoutText),
		})
	}

	if speciesAttempts := collectArtifactTextByPrefix(entries, "ocr_attempt_"); len(speciesAttempts) > 0 {
		frame.speciesMetaJSON = mergeDebugJSON(frame.speciesMetaJSON, map[string]any{
			"species_attempt_meta_texts": speciesAttempts,
		})
	}
	if speciesSelection, ok := findArtifactTextByLabel(entries, "species_selection_meta"); ok {
		frame.speciesMetaJSON = mergeDebugJSON(frame.speciesMetaJSON, map[string]any{
			"species_selection_meta_text": parseArtifactMetaContent(speciesSelection),
		})
	}

	if cpAttempts := collectArtifactTextByPrefix(entries, "cp_ocr_region_"); len(cpAttempts) > 0 {
		frame.cpMetaJSON = mergeDebugJSON(frame.cpMetaJSON, map[string]any{
			"cp_attempt_meta_texts": cpAttempts,
		})
	}
	if cpSelection, ok := findArtifactTextByLabel(entries, "cp_selection_meta"); ok {
		frame.cpMetaJSON = mergeDebugJSON(frame.cpMetaJSON, map[string]any{
			"cp_selection_meta_text": parseArtifactMetaContent(cpSelection),
		})
	}

	if hpAttempts := collectArtifactTextByPrefix(entries, "hp_ocr_region_"); len(hpAttempts) > 0 {
		frame.hpMetaJSON = mergeDebugJSON(frame.hpMetaJSON, map[string]any{
			"hp_attempt_meta_texts": hpAttempts,
		})
	}
	if hpSelection, ok := findArtifactTextByLabel(entries, "hp_selection_meta"); ok {
		frame.hpMetaJSON = mergeDebugJSON(frame.hpMetaJSON, map[string]any{
			"hp_selection_meta_text": parseArtifactMetaContent(hpSelection),
		})
	}

	if ivSelection, ok := findArtifactTextByLabel(entries, "iv_selection_meta"); ok {
		frame.ivMetaJSON = mergeDebugJSON(frame.ivMetaJSON, map[string]any{
			"iv_selection_meta_text": parseArtifactMetaContent(ivSelection),
		})
	}

	if ivBarMeasurement, ok := findArtifactTextByLabel(entries, "iv_bar_measurement_meta"); ok {
		frame.ivBarMeasurementMetaJSON = mergeDebugJSON(frame.ivBarMeasurementMetaJSON, map[string]any{
			"iv_bar_measurement_meta_text": parseArtifactMetaContent(ivBarMeasurement),
		})
	}
}

func structuredArtifactEntry(entry artifactTextEntry) map[string]any {
	return map[string]any{
		"label":   strings.TrimSpace(entry.Label),
		"content": parseArtifactMetaContent(entry.Content),
	}
}

func mergeDebugJSON(existing *string, additional map[string]any) *string {
	if len(additional) == 0 {
		return existing
	}

	merged := map[string]any{}
	if existing != nil && strings.TrimSpace(*existing) != "" {
		_ = json.Unmarshal([]byte(*existing), &merged)
	}
	for key, value := range additional {
		merged[key] = value
	}

	return marshalDebugJSON(merged)
}

func findArtifactTextByLabel(entries []artifactTextEntry, label string) (string, bool) {
	for idx := len(entries) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(entries[idx].Label) != label {
			continue
		}

		return entries[idx].Content, true
	}

	return "", false
}

func collectArtifactTextByPrefix(entries []artifactTextEntry, prefix string) []map[string]any {
	collected := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		label := strings.TrimSpace(entry.Label)
		if !strings.HasPrefix(label, prefix) {
			continue
		}

		collected = append(collected, structuredArtifactEntry(entry))
	}

	return collected
}

func parseArtifactMetaContent(content string) map[string]any {
	trimmed := strings.TrimSpace(content)
	lines := strings.Split(trimmed, "\n")

	parsedLines := make([]map[string]any, 0, len(lines))
	flat := map[string]any{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parsedLine := parseArtifactMetaLine(line)
		parsedLines = append(parsedLines, parsedLine)

		for key, value := range parsedLine {
			if strings.HasPrefix(key, "_") {
				continue
			}
			if existing, ok := flat[key]; ok {
				if slice, ok := existing.([]any); ok {
					flat[key] = append(slice, value)
				} else {
					flat[key] = []any{existing, value}
				}
			} else {
				flat[key] = value
			}
		}
	}

	return map[string]any{
		"raw":   trimmed,
		"flat":  flat,
		"lines": parsedLines,
	}
}

func parseArtifactMetaLine(line string) map[string]any {
	result := map[string]any{}
	idx := 0

	for idx < len(line) {
		for idx < len(line) && line[idx] == ' ' {
			idx++
		}
		if idx >= len(line) {
			break
		}

		keyStart := idx
		for idx < len(line) && isMetaKeyChar(line[idx]) {
			idx++
		}

		if keyStart == idx || idx >= len(line) || line[idx] != '=' {
			result["_raw"] = line
			if len(result) == 1 {
				return result
			}
			return result
		}

		key := line[keyStart:idx]
		idx++

		value, nextIdx, quoted := parseArtifactMetaValueToken(line, idx)
		idx = nextIdx
		result[key] = coerceArtifactMetaValue(value, quoted)
	}

	if len(result) == 0 {
		return map[string]any{"_raw": line}
	}

	return result
}

func parseArtifactMetaValueToken(line string, start int) (string, int, bool) {
	if start >= len(line) {
		return "", start, false
	}

	if line[start] == '"' {
		idx := start + 1
		var builder strings.Builder
		for idx < len(line) {
			ch := line[idx]
			if ch == '\\' && idx+1 < len(line) {
				idx++
				builder.WriteByte(line[idx])
				idx++
				continue
			}
			if ch == '"' {
				idx++
				break
			}
			builder.WriteByte(ch)
			idx++
		}
		return builder.String(), idx, true
	}

	end := start
	for end < len(line) && line[end] != ' ' {
		end++
	}

	return line[start:end], end, false
}

func coerceArtifactMetaValue(value string, quoted bool) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if trimmed == "<nil>" || trimmed == "nil" {
		return nil
	}

	if quoted {
		return trimmed
	}

	if trimmed == "true" {
		return true
	}
	if trimmed == "false" {
		return false
	}

	if i, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}

	return trimmed
}

func isMetaKeyChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

func insertFrameDebugRow(
	ctx context.Context,
	debugStore debugtrace.Store,
	job jobqueue.ClaimedJob,
	sourceType string,
	frameIndex int,
	frameTimestampMS *int64,
	frame processedFrame,
) error {
	frameStatus := strings.TrimSpace(frame.status)
	if frameStatus == "" {
		frameStatus = debugtrace.FrameStatusProcessed
	}

	frameFinishedAt := frame.finishedAt.UTC()
	if frameFinishedAt.IsZero() {
		frameFinishedAt = time.Now().UTC()
	}

	var frameStartedAt *time.Time
	if !frame.startedAt.IsZero() {
		started := frame.startedAt.UTC()
		frameStartedAt = &started
	}

	frameDurationMS := int64(0)
	if frameStartedAt != nil {
		frameDurationMS = frameFinishedAt.Sub(*frameStartedAt).Milliseconds()
		if frameDurationMS < 0 {
			frameDurationMS = 0
		}
	}

	_, err := debugStore.InsertFrameDebug(ctx, debugtrace.InsertFrameDebugParams{
		JobID:                    job.ID,
		UploadID:                 job.UploadID,
		SessionID:                job.SessionID,
		SourceType:               normalizePersistenceSourceType(sourceType),
		FrameIndex:               frameIndex,
		FrameTimestampMS:         frameTimestampMS,
		FrameStatus:              frameStatus,
		FrameStartedAt:           frameStartedAt,
		FrameFinishedAt:          frameFinishedAt,
		FrameDurationMS:          frameDurationMS,
		SpeciesFinishedAt:        frame.speciesFinishedAt,
		CPFinishedAt:             frame.cpFinishedAt,
		HPFinishedAt:             frame.hpFinishedAt,
		IVFinishedAt:             frame.ivFinishedAt,
		LayoutMetaJSON:           frame.layoutMetaJSON,
		SpeciesMetaJSON:          frame.speciesMetaJSON,
		CPMetaJSON:               frame.cpMetaJSON,
		HPMetaJSON:               frame.hpMetaJSON,
		IVMetaJSON:               frame.ivMetaJSON,
		IVBarMeasurementMetaJSON: frame.ivBarMeasurementMetaJSON,
		FrameStabilityMetaJSON:   frame.frameStabilityMetaJSON,
		SelectionMetaJSON:        frame.selectionMetaJSON,
		CreatedAt:                frameFinishedAt,
	})
	if err != nil {
		return ProcessingError{
			Code:    "PERSIST_FAILED",
			Message: fmt.Sprintf("insert frame debug row: %v", err),
		}
	}

	return nil
}

func marshalDebugJSON(value any) *string {
	if value == nil {
		return nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}

	text := string(raw)
	return &text
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}

	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}

	return normalized
}

func lookupUploadMedia(ctx context.Context, databasePath string, uploadID string) (string, string, error) {
	db, err := sql.Open("libsql", normalizeDatabaseURL(databasePath))
	if err != nil {
		return "", "", fmt.Errorf("open libsql db: %w", err)
	}
	defer db.Close()

	var mediaURL string
	var kind string
	err = db.QueryRowContext(
		ctx,
		`SELECT uploadthing_url, kind FROM uploads WHERE id = ?;`,
		uploadID,
	).Scan(&mediaURL, &kind)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("upload %q not found", uploadID)
		}
		return "", "", fmt.Errorf("query upload media: %w", err)
	}

	return mediaURL, strings.ToLower(strings.TrimSpace(kind)), nil
}

func resolveLocalMediaPath(mediaURL string, localDir string) (string, error) {
	if strings.TrimSpace(localDir) == "" {
		return "", fmt.Errorf("local storage directory is required")
	}

	parsed, err := url.Parse(mediaURL)
	if err != nil {
		return "", fmt.Errorf("parse local media URL: %w", err)
	}
	if parsed.Scheme != "local" {
		return "", fmt.Errorf("unsupported media URL scheme %q", parsed.Scheme)
	}
	if parsed.Host != "uploads" {
		return "", fmt.Errorf("unsupported local media host %q", parsed.Host)
	}

	cleanPath := path.Clean(strings.TrimPrefix(parsed.Path, "/"))
	if cleanPath == "." || cleanPath == "" {
		return "", fmt.Errorf("local media URL path is empty")
	}
	if cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return "", fmt.Errorf("local media URL path must stay within uploads root")
	}

	return filepath.Join(localDir, filepath.FromSlash(cleanPath)), nil
}

func (p imageProcessor) extractSpeciesName(
	ctx context.Context,
	decodedImage image.Image,
	preprocessedImage image.Image,
	artifactWriter *imageArtifactWriter,
) (string, error) {
	detectedLayout := appraisal.DetectNameLayout(decodedImage)
	layout := scaleNameLayoutToBounds(detectedLayout, preprocessedImage.Bounds())
	nameROI := appraisal.DetectSpeciesNameROI(preprocessedImage, layout)
	if err := writeDeterministicLayoutDebug(artifactWriter, decodedImage, preprocessedImage, detectedLayout, layout, nameROI); err != nil {
		return "", ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	attempts := buildDeterministicSpeciesNameAttempts(preprocessedImage, nameROI)
	if len(attempts) == 0 {
		return "", nil
	}

	catalogSpecies := p.speciesCatalog.SpeciesNamesNormalized()
	if len(catalogSpecies) == 0 {
		return "", ProcessingError{
			Code:    "SPECIES_CATALOG_EMPTY",
			Message: "species catalog contains no canonical species",
		}
	}

	var lastErr error
	results := make([]speciesOCRAttemptResult, 0, len(attempts))
	for idx, request := range attempts {
		attemptNumber := idx + 1
		inputImage, err := ocrInputImage(request)
		if err != nil {
			return "", ProcessingError{
				Code:    "OCR_INPUT_PREP_FAILED",
				Message: err.Error(),
			}
		}

		if err := artifactWriter.write(
			"ocr_attempt_"+strconv.Itoa(attemptNumber)+"_psm_"+strconv.Itoa(request.PageSegMode),
			inputImage,
		); err != nil {
			return "", ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}

		result := speciesOCRAttemptResult{
			AttemptNumber: attemptNumber,
			AttemptLabel:  "deterministic",
			Request:       request,
		}

		text, err := p.ocrEngine.ExtractText(ctx, ocr.ExtractRequest{
			Image:         inputImage,
			PageSegMode:   request.PageSegMode,
			CharWhitelist: request.CharWhitelist,
		})
		if err == nil {
			parsed, score := appraisal.ParseCandidateFromOCRWithScore(text)
			result.RawText = text
			result.Parsed = parsed
			result.Score = score
			if match, ok := appraisal.ResolveCanonicalSpeciesMatch(parsed, catalogSpecies); ok {
				result.Canonical = &match
			}
			result.SelectionScore = speciesAttemptSelectionScore(result)
			results = append(results, result)
			continue
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		result.OCRError = err
		results = append(results, result)
		lastErr = err
	}

	selectedAttemptNumber := -1
	selectedRawText := ""
	selectionMode := "none"
	selectionSpecies := ""

	if winner, ok := selectCanonicalSpeciesAttempt(results, catalogSpecies); ok {
		selectedAttemptNumber = winner.AttemptNumber
		if winner.Canonical != nil {
			selectionMode = winner.Canonical.Mode
			selectionSpecies = winner.Canonical.SpeciesNormalized
			if winner.Canonical.Mode == "prefix" {
				if winner.Parsed.SpeciesNameNormalized != nil {
					selectedRawText = *winner.Parsed.SpeciesNameNormalized
				} else {
					selectedRawText = winner.RawText
				}
			} else {
				selectedRawText = winner.Canonical.SpeciesNormalized
			}
		} else {
			selectedRawText = winner.RawText
		}
	} else if fallback, ok := selectFirstParsedAttempt(results); ok {
		selectedAttemptNumber = fallback.AttemptNumber
		selectionMode = "parsed_fallback"
		if fallback.Parsed.SpeciesNameNormalized != nil {
			selectedRawText = *fallback.Parsed.SpeciesNameNormalized
		} else {
			selectedRawText = fallback.RawText
		}
	}

	for _, result := range results {
		selected := result.AttemptNumber == selectedAttemptNumber
		if err := writeOCRAttemptDebug(
			artifactWriter,
			result.AttemptNumber,
			result.Request,
			result.RawText,
			result.OCRError,
			result.Parsed,
			result.Score,
			result.SelectionScore,
			result.AttemptLabel,
			result.Canonical,
			selected,
		); err != nil {
			return "", ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
	}
	if err := writeSpeciesSelectionSummary(
		artifactWriter,
		selectedAttemptNumber,
		selectionMode,
		selectionSpecies,
		selectedRawText,
	); err != nil {
		return "", ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	if selectedRawText != "" {
		return selectedRawText, nil
	}

	if lastErr != nil {
		return "", nil
	}

	return "", nil
}

func (p imageProcessor) extractCPRaw(
	ctx context.Context,
	decodedImage image.Image,
	preprocessedImage image.Image,
	artifactWriter *imageArtifactWriter,
) (*string, []string, error) {
	if preprocessedImage == nil || decodedImage == nil {
		return nil, nil, nil
	}

	decodedLayout := appraisal.DetectNameLayout(decodedImage)
	cpRegionsDecoded := deriveCPRegions(decodedLayout, decodedImage.Bounds())
	if len(cpRegionsDecoded) == 0 {
		return nil, nil, nil
	}

	results := make([]cpOCRAttemptResult, 0, len(cpRegionsDecoded)*12)
	selectedRegionNumber := -1
	selectedAttemptNumber := -1
	var selectedCPRaw *string
	for regionIndex, cpRegionDecoded := range cpRegionsDecoded {
		regionNumber := regionIndex + 1

		rawCrop, err := ocrInputImage(ocr.ExtractRequest{Image: decodedImage, Region: cpRegionDecoded})
		if err != nil {
			return nil, nil, ProcessingError{
				Code:    "OCR_INPUT_PREP_FAILED",
				Message: err.Error(),
			}
		}

		cleanedCrop := buildCPTextFocusedImage(rawCrop)
		cleanedOCRImage := upscaleImageNearest(cleanedCrop, 2)
		rawOCRImage := upscaleImageNearest(rawCrop, 2)
		strictCleanedCrop, hasStrictStrategy := buildCPStrictTextStrategyImage(rawCrop)
		var strictOCRImage image.Image
		if hasStrictStrategy {
			strictOCRImage = upscaleImageNearest(strictCleanedCrop, 2)
		}

		if err := artifactWriter.write("cp_region_"+strconv.Itoa(regionNumber)+"_preprocessed", cleanedOCRImage); err != nil {
			return nil, nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
		if err := artifactWriter.write("cp_region_"+strconv.Itoa(regionNumber)+"_preprocessed_raw", rawOCRImage); err != nil {
			return nil, nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
		if hasStrictStrategy {
			if err := artifactWriter.write("cp_region_"+strconv.Itoa(regionNumber)+"_preprocessed_strict", strictOCRImage); err != nil {
				return nil, nil, ProcessingError{
					Code:    "ARTIFACT_DUMP_FAILED",
					Message: err.Error(),
				}
			}
		}

		attempts := buildCPExtractionAttempts(regionNumber, cleanedOCRImage, rawOCRImage, strictOCRImage)
		for idx, attempt := range attempts {
			attemptNumber := idx + 1
			attempt.AttemptNumber = attemptNumber
			request := attempt.Request

			inputImage, err := ocrInputImage(request)
			if err != nil {
				return nil, nil, ProcessingError{
					Code:    "OCR_INPUT_PREP_FAILED",
					Message: err.Error(),
				}
			}

			if err := artifactWriter.write(
				"cp_ocr_region_"+strconv.Itoa(attempt.RegionNumber)+"_attempt_"+strconv.Itoa(attemptNumber)+"_psm_"+strconv.Itoa(request.PageSegMode),
				inputImage,
			); err != nil {
				return nil, nil, ProcessingError{
					Code:    "ARTIFACT_DUMP_FAILED",
					Message: err.Error(),
				}
			}

			rawText, err := p.ocrEngine.ExtractText(ctx, ocr.ExtractRequest{
				Image:         inputImage,
				PageSegMode:   request.PageSegMode,
				CharWhitelist: request.CharWhitelist,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, nil, err
				}
				attempt.OCRError = err
				results = append(results, attempt)
				continue
			}

			attempt.RawText = rawText
			attempt.ParsedCPRaw = appraisal.ParseCPRawFromOCR(rawText)
			results = append(results, attempt)
		}
	}

	candidateValues := rankCPAttemptCandidates(results)
	if winner, ok := selectPreferredCPAttempt(results, candidateValues); ok {
		selectedRegionNumber = winner.RegionNumber
		selectedAttemptNumber = winner.AttemptNumber
		selectedCPRaw = winner.ParsedCPRaw
	}

	for _, result := range results {
		if err := writeCPAttemptDebug(
			artifactWriter,
			result.RegionNumber,
			result.AttemptNumber,
			result.Request,
			result.RawText,
			result.ParsedCPRaw,
			result.OCRError,
			result.AttemptLabel,
			result.RegionNumber == selectedRegionNumber && result.AttemptNumber == selectedAttemptNumber,
		); err != nil {
			return nil, nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
	}
	if err := writeCPSelectionSummary(artifactWriter, cpRegionsDecoded, selectedRegionNumber, selectedAttemptNumber, selectedCPRaw); err != nil {
		return nil, nil, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	return selectedCPRaw, candidateValues, nil
}

func selectPreferredCPAttempt(results []cpOCRAttemptResult, rankedCandidateValues []string) (cpOCRAttemptResult, bool) {
	if len(rankedCandidateValues) == 0 {
		return selectBestCPAttempt(results)
	}

	targetValue := rankedCandidateValues[0]
	bestIndex := -1
	bestScore := -1
	for idx, result := range results {
		if result.ParsedCPRaw == nil || strings.TrimSpace(*result.ParsedCPRaw) != targetValue {
			continue
		}
		score := scoreParsedCPAttempt(result)
		if score > bestScore {
			bestIndex = idx
			bestScore = score
		}
	}

	if bestIndex < 0 {
		return selectBestCPAttempt(results)
	}
	return results[bestIndex], true
}

func buildCPExtractionAttempts(
	regionNumber int,
	cleanedOCRImage image.Image,
	rawOCRImage image.Image,
	strictOCRImage image.Image,
) []cpOCRAttemptResult {
	attempts := []cpOCRAttemptResult{
		{
			RegionNumber: regionNumber,
			AttemptLabel: "cleaned",
			Request: ocr.ExtractRequest{
				Image:         cleanedOCRImage,
				PageSegMode:   cpPageSegModePrimary,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "cleaned",
			Request: ocr.ExtractRequest{
				Image:         cleanedOCRImage,
				PageSegMode:   cpPageSegModeSecondary,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "cleaned",
			Request: ocr.ExtractRequest{
				Image:         cleanedOCRImage,
				PageSegMode:   10,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "cleaned_digits_only",
			Request: ocr.ExtractRequest{
				Image:         cleanedOCRImage,
				PageSegMode:   cpPageSegModePrimary,
				CharWhitelist: "0123456789",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "raw_decoded",
			Request: ocr.ExtractRequest{
				Image:         rawOCRImage,
				PageSegMode:   cpPageSegModePrimary,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "raw_decoded",
			Request: ocr.ExtractRequest{
				Image:         rawOCRImage,
				PageSegMode:   cpPageSegModeSecondary,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "raw_decoded",
			Request: ocr.ExtractRequest{
				Image:         rawOCRImage,
				PageSegMode:   10,
				CharWhitelist: "CPcp0123456789OILQ|!:.",
			},
		},
		{
			RegionNumber: regionNumber,
			AttemptLabel: "raw_decoded_digits_only",
			Request: ocr.ExtractRequest{
				Image:         rawOCRImage,
				PageSegMode:   cpPageSegModePrimary,
				CharWhitelist: "0123456789",
			},
		},
	}

	if strictOCRImage != nil {
		attempts = append(attempts,
			cpOCRAttemptResult{
				RegionNumber: regionNumber,
				AttemptLabel: "strict_cleaned",
				Request: ocr.ExtractRequest{
					Image:         strictOCRImage,
					PageSegMode:   cpPageSegModePrimary,
					CharWhitelist: "CPcp0123456789OILQ|!:.",
				},
			},
			cpOCRAttemptResult{
				RegionNumber: regionNumber,
				AttemptLabel: "strict_cleaned",
				Request: ocr.ExtractRequest{
					Image:         strictOCRImage,
					PageSegMode:   cpPageSegModeSecondary,
					CharWhitelist: "CPcp0123456789OILQ|!:.",
				},
			},
			cpOCRAttemptResult{
				RegionNumber: regionNumber,
				AttemptLabel: "strict_cleaned",
				Request: ocr.ExtractRequest{
					Image:         strictOCRImage,
					PageSegMode:   10,
					CharWhitelist: "CPcp0123456789OILQ|!:.",
				},
			},
			cpOCRAttemptResult{
				RegionNumber: regionNumber,
				AttemptLabel: "strict_cleaned_digits_only",
				Request: ocr.ExtractRequest{
					Image:         strictOCRImage,
					PageSegMode:   cpPageSegModePrimary,
					CharWhitelist: "0123456789",
				},
			},
		)
	}

	return attempts
}

func rankCPAttemptCandidates(results []cpOCRAttemptResult) []string {
	type candidateAggregate struct {
		value            string
		scoreSum         int
		occurrences      int
		distinctRegions  map[int]struct{}
		firstResultIndex int
	}

	aggregatesByValue := make(map[string]*candidateAggregate)
	for idx, result := range results {
		if result.ParsedCPRaw == nil {
			continue
		}
		value := strings.TrimSpace(*result.ParsedCPRaw)
		if value == "" {
			continue
		}

		aggregate, ok := aggregatesByValue[value]
		if !ok {
			aggregate = &candidateAggregate{
				value:            value,
				scoreSum:         0,
				occurrences:      0,
				distinctRegions:  make(map[int]struct{}),
				firstResultIndex: idx,
			}
			aggregatesByValue[value] = aggregate
		}

		aggregate.scoreSum += scoreParsedCPAttempt(result)
		aggregate.occurrences++
		aggregate.distinctRegions[result.RegionNumber] = struct{}{}
	}

	aggregates := make([]candidateAggregate, 0, len(aggregatesByValue))
	for _, aggregate := range aggregatesByValue {
		aggregates = append(aggregates, *aggregate)
	}

	sort.SliceStable(aggregates, func(i int, j int) bool {
		scoreI := aggregates[i].scoreSum + aggregates[i].occurrences*3 + len(aggregates[i].distinctRegions)*4
		scoreJ := aggregates[j].scoreSum + aggregates[j].occurrences*3 + len(aggregates[j].distinctRegions)*4
		if scoreI == scoreJ {
			return aggregates[i].firstResultIndex < aggregates[j].firstResultIndex
		}
		return scoreI > scoreJ
	})

	ranked := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		ranked = append(ranked, aggregate.value)
	}
	return ranked
}

func buildCPRetryCandidates(currentCPRaw *string, rankedCandidateValues []string) []string {
	current := ""
	if currentCPRaw != nil {
		current = strings.TrimSpace(*currentCPRaw)
	}

	retries := make([]string, 0, len(rankedCandidateValues))
	for _, candidate := range rankedCandidateValues {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == current {
			continue
		}
		retries = append(retries, candidate)
	}
	return retries
}

func (p imageProcessor) extractHPRaw(
	ctx context.Context,
	decodedImage image.Image,
	preprocessedImage image.Image,
	artifactWriter *imageArtifactWriter,
) (*string, error) {
	if preprocessedImage == nil || decodedImage == nil {
		return nil, nil
	}

	decodedLayout := appraisal.DetectNameLayout(decodedImage)
	hpRegionDecoded := deriveHPRegion(decodedLayout, decodedImage.Bounds())
	if hpRegionDecoded.Empty() {
		return nil, nil
	}

	rawCrop, err := ocrInputImage(ocr.ExtractRequest{Image: decodedImage, Region: hpRegionDecoded})
	if err != nil {
		return nil, ProcessingError{
			Code:    "OCR_INPUT_PREP_FAILED",
			Message: err.Error(),
		}
	}
	hpOCRImage := upscaleImageNearest(rawCrop, 2)

	if err := artifactWriter.write("hp_region_1_preprocessed", hpOCRImage); err != nil {
		return nil, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	attempts := []hpOCRAttemptResult{
		{
			RegionNumber: 1,
			AttemptLabel: "raw_fraction_and_label",
			Request: ocr.ExtractRequest{
				Image:         hpOCRImage,
				PageSegMode:   hpPageSegModePrimary,
				CharWhitelist: "0123456789HP/ ",
			},
		},
	}

	results := make([]hpOCRAttemptResult, 0, len(attempts))
	selectedRegionNumber := 1
	selectedAttemptNumber := -1
	var selectedHPRaw *string
	for idx, attempt := range attempts {
		attemptNumber := idx + 1
		attempt.AttemptNumber = attemptNumber
		request := attempt.Request

		inputImage, err := ocrInputImage(request)
		if err != nil {
			return nil, ProcessingError{
				Code:    "OCR_INPUT_PREP_FAILED",
				Message: err.Error(),
			}
		}

		if err := artifactWriter.write(
			"hp_ocr_region_"+strconv.Itoa(attempt.RegionNumber)+"_attempt_"+strconv.Itoa(attemptNumber)+"_psm_"+strconv.Itoa(request.PageSegMode),
			inputImage,
		); err != nil {
			return nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}

		rawText, err := p.ocrEngine.ExtractText(ctx, ocr.ExtractRequest{
			Image:         inputImage,
			PageSegMode:   request.PageSegMode,
			CharWhitelist: request.CharWhitelist,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			attempt.OCRError = err
			results = append(results, attempt)
			continue
		}
		attempt.RawText = rawText

		parsed := appraisal.ParseHPRawFromOCR(rawText)
		attempt.ParsedHPRaw = parsed
		results = append(results, attempt)
	}

	if len(results) > 0 && results[0].ParsedHPRaw != nil {
		selectedAttemptNumber = results[0].AttemptNumber
		selectedHPRaw = results[0].ParsedHPRaw
	}

	for _, result := range results {
		if err := writeHPAttemptDebug(
			artifactWriter,
			result.RegionNumber,
			result.AttemptNumber,
			result.Request,
			result.RawText,
			result.ParsedHPRaw,
			result.OCRError,
			result.AttemptLabel,
			result.AttemptNumber == selectedAttemptNumber,
		); err != nil {
			return nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
	}
	if err := writeHPSelectionSummary(artifactWriter, []image.Rectangle{hpRegionDecoded}, selectedRegionNumber, selectedAttemptNumber, selectedHPRaw); err != nil {
		return nil, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	return selectedHPRaw, nil
}

func (p imageProcessor) extractIVRaw(
	_ context.Context,
	decodedImage image.Image,
	preprocessedImage image.Image,
	artifactWriter *imageArtifactWriter,
) (appraisal.ParsedIVRaw, error) {
	if preprocessedImage == nil || decodedImage == nil {
		return appraisal.ParsedIVRaw{}, nil
	}

	decodedLayout := appraisal.DetectNameLayout(decodedImage)
	ivRegionDecoded := deriveIVRegion(decodedLayout, decodedImage.Bounds())
	if ivRegionDecoded.Empty() {
		return appraisal.ParsedIVRaw{}, nil
	}

	rawCrop, err := ocrInputImage(ocr.ExtractRequest{Image: decodedImage, Region: ivRegionDecoded})
	if err != nil {
		return appraisal.ParsedIVRaw{}, ProcessingError{
			Code:    "OCR_INPUT_PREP_FAILED",
			Message: err.Error(),
		}
	}
	ivOCRImage := upscaleImageNearest(rawCrop, 2)
	if err := artifactWriter.write("iv_region_1_raw", rawCrop); err != nil {
		return appraisal.ParsedIVRaw{}, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	if err := artifactWriter.write("iv_region_1_preprocessed", ivOCRImage); err != nil {
		return appraisal.ParsedIVRaw{}, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	selectedRegionNumber := 1
	selectedAttemptNumber := 0
	selectedMethod := "bar_ratio"
	selectedIVRaw, measurements, searchRegion := estimateIVRawFromBars(rawCrop)
	if err := writeIVBarMeasurementSummary(artifactWriter, searchRegion, measurements, selectedIVRaw); err != nil {
		return appraisal.ParsedIVRaw{}, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}
	if err := writeIVSelectionSummary(
		artifactWriter,
		[]image.Rectangle{ivRegionDecoded},
		selectedRegionNumber,
		selectedAttemptNumber,
		selectedIVRaw,
		selectedMethod,
	); err != nil {
		return appraisal.ParsedIVRaw{}, ProcessingError{
			Code:    "ARTIFACT_DUMP_FAILED",
			Message: err.Error(),
		}
	}

	return selectedIVRaw, nil
}

func deriveCPRegions(layout appraisal.NameLayout, bounds image.Rectangle) []image.Rectangle {
	if bounds.Empty() {
		return nil
	}

	regions := make([]image.Rectangle, 0, 6)

	nameBand := layout.NameBand.Intersect(bounds)
	if !nameBand.Empty() {
		centerX := nameBand.Min.X + nameBand.Dx()/2
		width := bounds.Dx()
		primaryShift := int(float64(width) * 0.06)
		secondaryShift := int(float64(width) * 0.04)

		regions = appendUniqueCPRegion(regions, cpRectFromCenter(bounds, centerX, 0.21, 0.010, 0.095))
		regions = appendUniqueCPRegion(regions, cpRectFromCenter(bounds, centerX-primaryShift, 0.24, 0.008, 0.110))
		regions = appendUniqueCPRegion(regions, cpRectFromCenter(bounds, centerX+secondaryShift, 0.24, 0.008, 0.110))
		regions = appendUniqueCPRegion(regions, cpRectFromCenter(bounds, centerX, 0.29, 0.008, 0.110))
	}

	// Fallbacks: top-center while excluding most of the favorite-button corner.
	regions = appendUniqueCPRegion(regions, cpRectFromRatios(bounds, 0.24, 0.010, 0.66, 0.095))
	regions = appendUniqueCPRegion(regions, cpRectFromRatios(bounds, 0.18, 0.008, 0.72, 0.110))

	if len(regions) == 0 {
		return nil
	}
	return regions
}

func deriveCPRegion(layout appraisal.NameLayout, bounds image.Rectangle) image.Rectangle {
	regions := deriveCPRegions(layout, bounds)
	if len(regions) == 0 {
		return image.Rectangle{}
	}
	return regions[0]
}

func cpRectFromCenter(bounds image.Rectangle, centerX int, halfWidthRatio float64, y0 float64, y1 float64) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return image.Rectangle{}
	}

	halfWidth := maxInt(1, int(float64(width)*halfWidthRatio))
	top := bounds.Min.Y + int(float64(height)*y0)
	bottom := bounds.Min.Y + int(float64(height)*y1)
	left := centerX - halfWidth
	right := centerX + halfWidth

	if right <= left {
		right = left + 1
	}
	if bottom <= top {
		bottom = top + 1
	}

	return image.Rect(left, top, right, bottom).Intersect(bounds)
}

func appendUniqueCPRegion(regions []image.Rectangle, candidate image.Rectangle) []image.Rectangle {
	if candidate.Empty() {
		return regions
	}
	for _, existing := range regions {
		if existing == candidate {
			return regions
		}
	}
	return append(regions, candidate)
}

func deriveHPRegions(layout appraisal.NameLayout, bounds image.Rectangle) []image.Rectangle {
	region := deriveHPRegion(layout, bounds)
	if region.Empty() {
		return nil
	}
	return []image.Rectangle{region}
}

func deriveHPRegion(layout appraisal.NameLayout, bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	card := layout.CardBounds.Intersect(bounds)
	if card.Empty() {
		return hpRectFromRatios(bounds, 0.24, 0.36, 0.76, 0.48)
	}

	hpY := layout.HPBarY
	if hpY <= card.Min.Y || hpY >= card.Max.Y {
		hpY = card.Min.Y + int(float64(card.Dy())*0.27)
	}

	left := card.Min.X + int(float64(card.Dx())*0.20)
	right := card.Max.X - int(float64(card.Dx())*0.20)
	top := hpY + maxInt(2, int(float64(card.Dy())*0.012))
	bottom := hpY + maxInt(4, int(float64(card.Dy())*0.080))

	region := image.Rect(left, top, right, bottom).Intersect(bounds)
	if region.Empty() {
		return hpRectFromRatios(bounds, 0.24, 0.36, 0.76, 0.48)
	}

	return region
}

func deriveIVRegions(layout appraisal.NameLayout, bounds image.Rectangle) []image.Rectangle {
	region := deriveIVRegion(layout, bounds)
	if region.Empty() {
		return nil
	}
	return []image.Rectangle{region}
}

func deriveIVRegion(layout appraisal.NameLayout, bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	card := layout.CardBounds.Intersect(bounds)
	if card.Empty() {
		return hpRectFromRatios(bounds, 0.03, 0.50, 0.70, 0.92)
	}

	left := card.Min.X + int(float64(card.Dx())*0.03)
	right := card.Min.X + int(float64(card.Dx())*0.70)
	top := card.Min.Y + int(float64(card.Dy())*0.50)
	bottom := card.Min.Y + int(float64(card.Dy())*0.92)

	region := image.Rect(left, top, right, bottom).Intersect(bounds)
	if region.Empty() {
		return hpRectFromRatios(bounds, 0.03, 0.50, 0.70, 0.92)
	}

	return region
}

func hpRectFromRatios(bounds image.Rectangle, x0 float64, y0 float64, x1 float64, y1 float64) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return image.Rectangle{}
	}

	left := bounds.Min.X + int(float64(width)*x0)
	top := bounds.Min.Y + int(float64(height)*y0)
	right := bounds.Min.X + int(float64(width)*x1)
	bottom := bounds.Min.Y + int(float64(height)*y1)

	if right <= left {
		right = left + 1
		if right > bounds.Max.X {
			right = bounds.Max.X
		}
	}
	if bottom <= top {
		bottom = top + 1
		if bottom > bounds.Max.Y {
			bottom = bounds.Max.Y
		}
	}

	rect := image.Rect(left, top, right, bottom).Intersect(bounds)
	if rect.Empty() {
		return bounds
	}

	return rect
}

func cpRectFromRatios(bounds image.Rectangle, x0 float64, y0 float64, x1 float64, y1 float64) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return image.Rectangle{}
	}

	left := bounds.Min.X + int(float64(width)*x0)
	top := bounds.Min.Y + int(float64(height)*y0)
	right := bounds.Min.X + int(float64(width)*x1)
	bottom := bounds.Min.Y + int(float64(height)*y1)

	if right <= left {
		right = left + 1
		if right > bounds.Max.X {
			right = bounds.Max.X
		}
	}
	if bottom <= top {
		bottom = top + 1
		if bottom > bounds.Max.Y {
			bottom = bounds.Max.Y
		}
	}

	rect := image.Rect(left, top, right, bottom).Intersect(bounds)
	if rect.Empty() {
		return bounds
	}

	return rect
}

func appraisalPixelLuma(src image.Image, x int, y int) uint8 {
	switch typed := src.(type) {
	case *image.Gray:
		return typed.GrayAt(x, y).Y
	default:
		r, g, b, _ := src.At(x, y).RGBA()
		r8 := int(r >> 8)
		g8 := int(g >> 8)
		b8 := int(b >> 8)
		return uint8((299*r8 + 587*g8 + 114*b8) / 1000)
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func selectBestCPAttempt(results []cpOCRAttemptResult) (cpOCRAttemptResult, bool) {
	bestIndex := -1
	bestScore := -1

	for idx, result := range results {
		if result.ParsedCPRaw == nil {
			continue
		}
		score := scoreParsedCPAttempt(result)
		if score > bestScore {
			bestIndex = idx
			bestScore = score
		}
	}

	if bestIndex < 0 {
		return cpOCRAttemptResult{}, false
	}
	return results[bestIndex], true
}

func scoreParsedCPAttempt(result cpOCRAttemptResult) int {
	if result.ParsedCPRaw == nil {
		return 0
	}

	cpRaw := strings.TrimSpace(*result.ParsedCPRaw)
	score := 0

	switch cpLen := len(cpRaw); cpLen {
	case 4:
		score += 10
	case 3:
		score += 8
	case 2, 5:
		score += 2
	default:
		score += 1
	}

	rawUpper := strings.ToUpper(strings.TrimSpace(result.RawText))
	if strings.Contains(rawUpper, "CP") {
		score += 2
	}

	if cpValue, err := strconv.Atoi(cpRaw); err == nil {
		switch {
		case cpValue >= 10 && cpValue <= 6000:
			score += 4
		case cpValue >= 1 && cpValue <= 9999:
			score += 1
		}
	}

	if hasRepeatedDigitRun(cpRaw, 4) {
		score -= 2
	}

	return score
}

func estimateIVRawFromBars(src image.Image) (appraisal.ParsedIVRaw, []ivBarMeasurement, image.Rectangle) {
	if src == nil {
		return appraisal.ParsedIVRaw{}, nil, image.Rectangle{}
	}

	bounds := src.Bounds()
	if bounds.Empty() {
		return appraisal.ParsedIVRaw{}, nil, image.Rectangle{}
	}

	searchRegion := bounds
	if searchRegion.Empty() {
		return appraisal.ParsedIVRaw{}, nil, image.Rectangle{}
	}

	xStart := bounds.Min.X + int(float64(bounds.Dx())*0.08)
	xEnd := bounds.Min.X + int(float64(bounds.Dx())*0.76)
	if xEnd <= xStart {
		return appraisal.ParsedIVRaw{}, nil, searchRegion
	}

	bandHeight := maxInt(10, int(float64(bounds.Dy())*0.06))
	defaultCenters := []int{
		bounds.Min.Y + int(float64(bounds.Dy())*0.46),
		bounds.Min.Y + int(float64(bounds.Dy())*0.63),
		bounds.Min.Y + int(float64(bounds.Dy())*0.80),
	}

	measureForCenters := func(centers []int) []ivBarMeasurement {
		result := make([]ivBarMeasurement, 0, len(centers))
		for idx, centerY := range centers {
			measurement, ok := measureBestIVBarAroundCenter(src, bounds, xStart, xEnd, centerY, bandHeight)
			if !ok {
				continue
			}
			measurement.BarIndex = idx + 1
			result = append(result, measurement)
		}
		return result
	}

	defaultMeasurements := measureForCenters(defaultCenters)
	measurements := defaultMeasurements

	rowCenters := detectIVBarCenters(src, bounds, xStart, xEnd, bandHeight)
	if len(rowCenters) == 2 {
		spacing := rowCenters[1] - rowCenters[0]
		if spacing > 0 {
			topCandidate := rowCenters[0] - spacing
			if topCandidate >= bounds.Min.Y+bandHeight/2 {
				rowCenters = append(rowCenters, topCandidate)
			} else {
				bottomCandidate := rowCenters[1] + spacing
				if bottomCandidate <= bounds.Max.Y-bandHeight/2-1 {
					rowCenters = append(rowCenters, bottomCandidate)
				}
			}
		}
	}
	if len(rowCenters) >= 3 {
		sort.Ints(rowCenters)
		rowCenters = rowCenters[:3]
		detectedMeasurements := measureForCenters(rowCenters)
		if scoreIVMeasurementSet(detectedMeasurements) > scoreIVMeasurementSet(defaultMeasurements)+120 {
			measurements = detectedMeasurements
		}
	}
	if len(measurements) < 3 && len(defaultMeasurements) >= len(measurements) {
		measurements = defaultMeasurements
	}

	parsed := appraisal.ParsedIVRaw{}
	for _, measurement := range measurements {
		switch measurement.BarIndex {
		case 1:
			parsed.AttackRaw = intStringPtr(measurement.Value)
		case 2:
			parsed.DefenseRaw = intStringPtr(measurement.Value)
		case 3:
			parsed.StaminaRaw = intStringPtr(measurement.Value)
		}
	}

	return parsed, measurements, searchRegion
}

func scoreIVMeasurementSet(measurements []ivBarMeasurement) int {
	if len(measurements) != 3 {
		return -10000
	}

	sumTrack := 0
	sumFill := 0
	minTrack := measurements[0].TrackWidth
	maxTrack := measurements[0].TrackWidth
	for _, measurement := range measurements {
		sumTrack += measurement.TrackWidth
		sumFill += measurement.FillWidth
		if measurement.TrackWidth < minTrack {
			minTrack = measurement.TrackWidth
		}
		if measurement.TrackWidth > maxTrack {
			maxTrack = measurement.TrackWidth
		}
	}

	consistencyPenalty := (maxTrack - minTrack) * 6
	return sumTrack + sumFill/3 - consistencyPenalty
}

func detectIVBarCenters(
	src image.Image,
	panelBounds image.Rectangle,
	xStart int,
	xEnd int,
	bandHeight int,
) []int {
	if bandHeight <= 0 {
		return nil
	}

	step := maxInt(2, bandHeight/4)
	minTrackWidth := maxInt(48, (xEnd-xStart)/3)
	minSpacing := maxInt(10, (bandHeight*11)/10)

	type rowCandidate struct {
		centerY int
		score   int
	}

	candidates := make([]rowCandidate, 0, 32)
	minCenter := panelBounds.Min.Y + bandHeight/2
	maxCenter := panelBounds.Max.Y - bandHeight/2 - 1
	for centerY := minCenter; centerY <= maxCenter; centerY += step {
		rowTop := centerY - bandHeight/2
		rowBottom := centerY + bandHeight/2
		trackStart, trackEnd, ok := detectIVBarRunByContrast(src, xStart, xEnd, rowTop, rowBottom)
		if !ok {
			continue
		}
		trackWidth := trackEnd - trackStart + 1
		if trackWidth < minTrackWidth {
			continue
		}

		measurement, ok := measureIVBarBand(src, xStart, xEnd, rowTop, rowBottom)
		if !ok {
			continue
		}
		if measurement.TrackWidth < minTrackWidth {
			continue
		}
		if measurement.TrackWidth*100 < trackWidth*55 {
			continue
		}

		candidates = append(candidates, rowCandidate{
			centerY: centerY,
			score:   measurement.TrackWidth + measurement.FillWidth/4,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i int, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].centerY < candidates[j].centerY
		}
		return candidates[i].score > candidates[j].score
	})

	selected := make([]int, 0, 3)
	for _, candidate := range candidates {
		tooClose := false
		for _, existing := range selected {
			if absInt(existing-candidate.centerY) < minSpacing {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}

		selected = append(selected, candidate.centerY)
		if len(selected) == 3 {
			break
		}
	}

	sort.Ints(selected)
	return selected
}

func detectIVBarRunByContrast(
	src image.Image,
	xStart int,
	xEnd int,
	rowTop int,
	rowBottom int,
) (int, int, bool) {
	if xEnd <= xStart || rowBottom < rowTop {
		return 0, 0, false
	}

	bandHeight := rowBottom - rowTop + 1
	width := xEnd - xStart
	occupied := make([]bool, width)
	lumas := make([]int, 0, width*bandHeight)
	for x := xStart; x < xEnd; x++ {
		for y := rowTop; y <= rowBottom; y++ {
			lumas = append(lumas, int(appraisalPixelLuma(src, x, y)))
		}
	}
	if len(lumas) == 0 {
		return 0, 0, false
	}

	bgLuma := percentileInt(lumas, 85)
	darkLuma := percentileInt(lumas, 25)
	contrastThreshold := maxInt(12, (bgLuma-darkLuma)/3)
	darkPixelThreshold := bgLuma - contrastThreshold
	minTrackPixels := maxInt(2, (bandHeight*28)/100)

	for x := xStart; x < xEnd; x++ {
		darkCount := 0
		for y := rowTop; y <= rowBottom; y++ {
			if int(appraisalPixelLuma(src, x, y)) <= darkPixelThreshold {
				darkCount++
			}
		}
		if darkCount >= minTrackPixels {
			occupied[x-xStart] = true
		}
	}

	bridgeBooleanGaps(occupied, maxInt(8, width/80))
	trackStart, trackEnd, ok := longestTrueRun(occupied, maxInt(30, width/4))
	if !ok {
		return 0, 0, false
	}
	return trackStart, trackEnd, true
}

func measureBestIVBarAroundCenter(
	src image.Image,
	panelBounds image.Rectangle,
	xStart int,
	xEnd int,
	centerY int,
	bandHeight int,
) (ivBarMeasurement, bool) {
	if bandHeight <= 0 {
		return ivBarMeasurement{}, false
	}

	offsets := []int{
		0,
		-bandHeight / 2,
		bandHeight / 2,
		-bandHeight,
		bandHeight,
		-(bandHeight * 3) / 2,
		(bandHeight * 3) / 2,
		-2 * bandHeight,
		2 * bandHeight,
	}

	best := ivBarMeasurement{}
	bestScore := -1
	for _, offset := range offsets {
		rowCenter := centerY + offset
		rowTop := rowCenter - bandHeight/2
		rowBottom := rowCenter + bandHeight/2
		if rowTop < panelBounds.Min.Y {
			rowTop = panelBounds.Min.Y
		}
		if rowBottom >= panelBounds.Max.Y {
			rowBottom = panelBounds.Max.Y - 1
		}
		if rowBottom-rowTop+1 < 8 {
			continue
		}

		measurement, ok := measureIVBarBand(src, xStart, xEnd, rowTop, rowBottom)
		if !ok {
			continue
		}

		score := measurement.TrackWidth + measurement.FillWidth/3 - absInt(offset)*3
		if score > bestScore {
			best = measurement
			bestScore = score
		}
	}

	if bestScore < 0 {
		return ivBarMeasurement{}, false
	}
	return best, true
}

func measureIVBarBand(
	src image.Image,
	xStart int,
	xEnd int,
	rowTop int,
	rowBottom int,
) (ivBarMeasurement, bool) {
	if xEnd <= xStart || rowBottom < rowTop {
		return ivBarMeasurement{}, false
	}

	bandHeight := rowBottom - rowTop + 1
	width := xEnd - xStart
	occupied := make([]bool, width)
	filled := make([]bool, width)
	minTrackPixels := maxInt(2, (bandHeight*28)/100)
	minFillPixels := maxInt(2, (bandHeight*12)/100)

	for x := xStart; x < xEnd; x++ {
		trackCount := 0
		fillCount := 0
		for y := rowTop; y <= rowBottom; y++ {
			if isIVFilledBarPixel(src, x, y) {
				fillCount++
				trackCount++
				continue
			}
			if isIVBarTrackOrFillPixel(src, x, y) {
				trackCount++
			}
		}

		idx := x - xStart
		if trackCount >= minTrackPixels {
			occupied[idx] = true
		}
		if fillCount >= minFillPixels {
			filled[idx] = true
		}
	}

	bridgeBooleanGaps(occupied, maxInt(8, width/80))
	trackStart, trackEnd, ok := longestTrueRun(occupied, maxInt(30, width/4))
	if !ok {
		return ivBarMeasurement{}, false
	}

	trackWidth := trackEnd - trackStart + 1
	if trackWidth <= 0 {
		return ivBarMeasurement{}, false
	}

	bridgeBooleanGaps(filled, maxInt(12, width/40))
	gapLimit := maxInt(12, width/35)
	fillEnd := findConnectedFillEnd(filled, trackStart, trackEnd, gapLimit)

	fillWidth := 0
	if fillEnd >= trackStart {
		fillWidth = fillEnd - trackStart + 1
	}
	if fillWidth > trackWidth {
		fillWidth = trackWidth
	}

	adjustedFillWidth := fillWidth
	if fillWidth > 0 && fillWidth < trackWidth {
		// Compensate for anti-aliased bar endcaps that undercount by a few pixels.
		adjustedFillWidth += maxInt(2, trackWidth/90)
		if adjustedFillWidth > trackWidth {
			adjustedFillWidth = trackWidth
		}
	}

	value := clampInt((adjustedFillWidth*15+trackWidth/2)/trackWidth, 0, 15)
	ratio := 0.0
	if trackWidth > 0 {
		ratio = float64(fillWidth) / float64(trackWidth)
	}

	return ivBarMeasurement{
		RowTop:     rowTop,
		RowBottom:  rowBottom,
		TrackStart: xStart + trackStart,
		TrackEnd:   xStart + trackEnd,
		FillEnd:    xStart + fillEnd,
		TrackWidth: trackWidth,
		FillWidth:  fillWidth,
		Value:      value,
		Ratio:      ratio,
	}, true
}

func percentileInt(values []int, p int) int {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int(nil), values...)
	sort.Ints(copied)
	if p <= 0 {
		return copied[0]
	}
	if p >= 100 {
		return copied[len(copied)-1]
	}
	index := (len(copied) - 1) * p / 100
	return copied[index]
}

func longestTrueRun(values []bool, minRun int) (int, int, bool) {
	bestStart := -1
	bestEnd := -1
	start := -1
	for idx, value := range values {
		if value {
			if start < 0 {
				start = idx
			}
			continue
		}
		if start >= 0 {
			end := idx - 1
			if end-start > bestEnd-bestStart {
				bestStart = start
				bestEnd = end
			}
			start = -1
		}
	}
	if start >= 0 {
		end := len(values) - 1
		if end-start > bestEnd-bestStart {
			bestStart = start
			bestEnd = end
		}
	}

	if bestStart < 0 || bestEnd < bestStart {
		return 0, 0, false
	}
	if bestEnd-bestStart+1 < minRun {
		return 0, 0, false
	}
	return bestStart, bestEnd, true
}

func bridgeBooleanGaps(values []bool, maxGap int) {
	if maxGap <= 0 || len(values) == 0 {
		return
	}

	lastTrue := -1
	for idx := 0; idx < len(values); idx++ {
		if !values[idx] {
			continue
		}
		if lastTrue >= 0 {
			gapStart := lastTrue + 1
			gapSize := idx - gapStart
			if gapSize > 0 && gapSize <= maxGap {
				for fill := gapStart; fill < idx; fill++ {
					values[fill] = true
				}
			}
		}
		lastTrue = idx
	}
}

func findConnectedFillEnd(values []bool, start int, end int, gapLimit int) int {
	lastFilled := -1
	seenFill := false
	gap := 0
	for idx := start; idx <= end && idx < len(values); idx++ {
		if values[idx] {
			seenFill = true
			lastFilled = idx
			gap = 0
			continue
		}
		if !seenFill {
			continue
		}
		gap++
		if gap > gapLimit {
			break
		}
	}
	return lastFilled
}

func intStringPtr(value int) *string {
	asString := strconv.Itoa(value)
	return &asString
}

func isIVOrangePixel(src image.Image, x int, y int) bool {
	r, g, b := pixelRGB(src, x, y)
	if r < 120 || g < 50 {
		return false
	}
	if b > 185 {
		return false
	}
	if r < g-18 {
		return false
	}
	if r-b < 25 {
		return false
	}
	maxRGB := maxInt(r, maxInt(g, b))
	minRGB := minInt(r, minInt(g, b))
	if maxRGB-minRGB < 36 {
		return false
	}
	return true
}

func isIVBarTrackOrFillPixel(src image.Image, x int, y int) bool {
	r, g, b := pixelRGB(src, x, y)
	if isIVFilledBarPixel(src, x, y) {
		return true
	}
	maxRGB := maxInt(r, maxInt(g, b))
	minRGB := minInt(r, minInt(g, b))
	if maxRGB-minRGB > 24 {
		return false
	}
	luma := (299*r + 587*g + 114*b) / 1000
	return luma >= 130 && luma <= 246
}

func isIVFilledBarPixel(src image.Image, x int, y int) bool {
	r, g, b := pixelRGB(src, x, y)
	maxRGB := maxInt(r, maxInt(g, b))
	minRGB := minInt(r, minInt(g, b))
	if maxRGB-minRGB < 36 {
		return false
	}
	if r < 120 || g < 50 || b > 185 {
		return false
	}
	if r < g-18 {
		return false
	}
	if r-b < 25 {
		return false
	}
	return true
}

func pixelRGB(src image.Image, x int, y int) (int, int, int) {
	r, g, b, _ := src.At(x, y).RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func buildCPTextFocusedImage(src image.Image) *image.Gray {
	base := toGrayImage(src)
	bounds := base.Bounds()
	if bounds.Empty() {
		return base
	}

	totalPixels := bounds.Dx() * bounds.Dy()
	minArea := maxInt(12, totalPixels/9000)
	maxArea := maxInt(minArea+1, totalPixels/45)
	minHeight := maxInt(6, bounds.Dy()/18)
	maxHeight := maxInt(minHeight+1, bounds.Dy()*4/5)
	minWidth := 2
	maxWidth := maxInt(minWidth+1, bounds.Dx()/4)

	standardMask, standardKeptCount, _ := extractCPForegroundMask(
		base,
		func(x int, y int) bool { return isNearWhitePixel(src, x, y) },
		minArea,
		maxArea,
		minWidth,
		maxWidth,
		minHeight,
		maxHeight,
	)
	if standardKeptCount > 0 {
		return standardMask
	}

	// Preserve original behavior: fall back to original crop when filtering removed
	// all foreground pixels.
	return base
}

func buildCPStrictTextStrategyImage(src image.Image) (*image.Gray, bool) {
	base := toGrayImage(src)
	bounds := base.Bounds()
	if bounds.Empty() {
		return nil, false
	}

	totalPixels := bounds.Dx() * bounds.Dy()
	minArea := maxInt(12, totalPixels/9000)
	maxArea := maxInt(minArea+1, totalPixels/45)
	minHeight := maxInt(6, bounds.Dy()/18)
	maxHeight := maxInt(minHeight+1, bounds.Dy()*4/5)
	minWidth := 2
	maxWidth := maxInt(minWidth+1, bounds.Dx()/4)

	_, standardKeptCount, largestRejectedArea := extractCPForegroundMask(
		base,
		func(x int, y int) bool { return isNearWhitePixel(src, x, y) },
		minArea,
		maxArea,
		minWidth,
		maxWidth,
		minHeight,
		maxHeight,
	)
	if standardKeptCount > 0 {
		return nil, false
	}
	if largestRejectedArea <= maxArea {
		return nil, false
	}

	strictThreshold := estimateStrictCPTextLumaThreshold(base)
	strictMinArea := maxInt(4, totalPixels/30000)
	strictMaxArea := maxInt(strictMinArea+1, totalPixels/90)
	strictMinHeight := maxInt(4, bounds.Dy()/28)
	strictMaxHeight := maxInt(strictMinHeight+1, bounds.Dy()*3/4)
	strictMinWidth := 1
	strictMaxWidth := maxInt(strictMinWidth+1, bounds.Dx()/3)

	strictMask, strictKeptCount, _ := extractCPForegroundMask(
		base,
		func(x int, y int) bool { return base.GrayAt(x, y).Y >= strictThreshold },
		strictMinArea,
		strictMaxArea,
		strictMinWidth,
		strictMaxWidth,
		strictMinHeight,
		strictMaxHeight,
	)
	if strictKeptCount == 0 {
		return nil, false
	}
	return strictMask, true
}

func extractCPForegroundMask(
	base *image.Gray,
	predicate func(x int, y int) bool,
	minArea int,
	maxArea int,
	minWidth int,
	maxWidth int,
	minHeight int,
	maxHeight int,
) (*image.Gray, int, int) {
	bounds := base.Bounds()
	if bounds.Empty() {
		return base, 0, 0
	}

	type point struct{ x, y int }
	visited := make([]bool, len(base.Pix))
	dst := image.NewGray(bounds)
	for i := range dst.Pix {
		dst.Pix[i] = 0
	}

	rowStride := base.Stride
	w := bounds.Dx()
	h := bounds.Dy()
	dx := []int{1, -1, 0, 0}
	dy := []int{0, 0, 1, -1}

	keptCount := 0
	largestRejectedArea := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			index := y*rowStride + x
			if visited[index] {
				continue
			}
			visited[index] = true
			if !predicate(x, y) {
				continue
			}

			queue := make([]point, 0, 64)
			component := make([]point, 0, 64)
			queue = append(queue, point{x: x, y: y})
			component = append(component, point{x: x, y: y})
			minX, maxX := x, x
			minY, maxY := y, y

			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]

				for dir := 0; dir < 4; dir++ {
					nx := cur.x + dx[dir]
					ny := cur.y + dy[dir]
					if nx < 0 || nx >= w || ny < 0 || ny >= h {
						continue
					}
					nidx := ny*rowStride + nx
					if visited[nidx] {
						continue
					}
					visited[nidx] = true
					if !predicate(nx, ny) {
						continue
					}
					queue = append(queue, point{x: nx, y: ny})
					component = append(component, point{x: nx, y: ny})
					if nx < minX {
						minX = nx
					}
					if nx > maxX {
						maxX = nx
					}
					if ny < minY {
						minY = ny
					}
					if ny > maxY {
						maxY = ny
					}
				}
			}

			area := len(component)
			compW := maxX - minX + 1
			compH := maxY - minY + 1
			keep := area >= minArea &&
				area <= maxArea &&
				compW >= minWidth &&
				compW <= maxWidth &&
				compH >= minHeight &&
				compH <= maxHeight
			if !keep {
				if area > largestRejectedArea {
					largestRejectedArea = area
				}
				continue
			}

			keptCount++
			for _, pt := range component {
				dst.SetGray(pt.x, pt.y, color.Gray{Y: 255})
			}
		}
	}

	return dst, keptCount, largestRejectedArea
}

func estimateStrictCPTextLumaThreshold(base *image.Gray) uint8 {
	var histogram [256]int
	total := 0
	for _, px := range base.Pix {
		histogram[int(px)]++
		total++
	}
	if total <= 0 {
		return 205
	}

	targetRank := int(float64(total) * 0.96)
	if targetRank < 0 {
		targetRank = 0
	}
	if targetRank >= total {
		targetRank = total - 1
	}

	cumulative := 0
	threshold := 255
	for i := 0; i < len(histogram); i++ {
		cumulative += histogram[i]
		if cumulative > targetRank {
			threshold = i
			break
		}
	}

	threshold = maxInt(threshold, 205)
	threshold = minInt(threshold, 245)
	return uint8(threshold)
}

func toGrayImage(src image.Image) *image.Gray {
	if typed, ok := src.(*image.Gray); ok {
		copyImage := image.NewGray(typed.Bounds())
		copy(copyImage.Pix, typed.Pix)
		return copyImage
	}

	bounds := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			luma := appraisalPixelLuma(src, x, y)
			dst.SetGray(x-bounds.Min.X, y-bounds.Min.Y, color.Gray{Y: luma})
		}
	}
	return dst
}

func isNearWhitePixel(src image.Image, x int, y int) bool {
	switch typed := src.(type) {
	case *image.Gray:
		return typed.GrayAt(x, y).Y >= 200
	default:
		r, g, b, _ := src.At(x, y).RGBA()
		r8 := int(r >> 8)
		g8 := int(g >> 8)
		b8 := int(b >> 8)
		luma := (299*r8 + 587*g8 + 114*b8) / 1000
		if luma < 175 {
			return false
		}

		maxRGB := maxInt(r8, maxInt(g8, b8))
		minRGB := minInt(r8, minInt(g8, b8))
		return maxRGB-minRGB <= 35
	}
}

func upscaleImageNearest(src image.Image, factor int) image.Image {
	if src == nil || factor <= 1 {
		return src
	}

	bounds := src.Bounds()
	if bounds.Empty() {
		return src
	}

	width := bounds.Dx()
	height := bounds.Dy()
	dst := image.NewGray(image.Rect(0, 0, width*factor, height*factor))
	for y := 0; y < height*factor; y++ {
		sourceY := y / factor
		for x := 0; x < width*factor; x++ {
			sourceX := x / factor
			luma := appraisalPixelLuma(src, bounds.Min.X+sourceX, bounds.Min.Y+sourceY)
			dst.SetGray(x, y, color.Gray{Y: luma})
		}
	}

	return dst
}

func hasRepeatedDigitRun(value string, minRun int) bool {
	if minRun <= 1 || len(value) < minRun {
		return false
	}

	runDigit := rune(0)
	runLength := 0
	for _, r := range value {
		if r == runDigit {
			runLength++
		} else {
			runDigit = r
			runLength = 1
		}
		if runLength >= minRun {
			return true
		}
	}

	return false
}

// selectCanonicalSpeciesAttempt selects the best canonical match among OCR attempts.
// Runtime attempts are ordered by priority (attempt 1 then 2), so ties are broken
// by earlier attempt index.
func selectCanonicalSpeciesAttempt(
	results []speciesOCRAttemptResult,
	catalogSpecies []string,
) (speciesOCRAttemptResult, bool) {
	var zero speciesOCRAttemptResult
	if len(catalogSpecies) == 0 {
		return zero, false
	}

	bestIndex := -1
	bestRank := 0
	bestDistance := 0
	var bestCanonical appraisal.CanonicalSpeciesMatch

	for idx, rawResult := range results {
		result := rawResult
		match := result.Canonical
		if match == nil {
			resolved, ok := appraisal.ResolveCanonicalSpeciesMatch(result.Parsed, catalogSpecies)
			if !ok {
				continue
			}
			match = &resolved
		}

		rank := canonicalModeRank(match.Mode)
		if rank == 0 {
			continue
		}

		replace := false
		if rank > bestRank {
			replace = true
		} else if rank == bestRank {
			if match.Distance < bestDistance {
				replace = true
			} else if match.Distance == bestDistance && (bestIndex < 0 || idx < bestIndex) {
				replace = true
			}
		}
		if !replace {
			continue
		}

		bestIndex = idx
		bestRank = rank
		bestDistance = match.Distance
		bestCanonical = *match
	}

	if bestIndex < 0 || bestIndex >= len(results) {
		return zero, false
	}

	winner := results[bestIndex]
	winner.Canonical = &bestCanonical
	if winner.SelectionScore <= 0 {
		winner.SelectionScore = speciesAttemptSelectionScore(winner)
	}
	return winner, true
}

func canonicalModeRank(mode string) int {
	switch mode {
	case "exact":
		return 3
	case "prefix":
		return 2
	case "fuzzy":
		return 1
	default:
		return 0
	}
}

func scaleNameLayoutToBounds(layout appraisal.NameLayout, targetBounds image.Rectangle) appraisal.NameLayout {
	if targetBounds.Empty() {
		return layout
	}

	sourceBounds := layout.ImageBounds
	if sourceBounds.Empty() {
		layout.ImageBounds = targetBounds
		return layout
	}

	if sourceBounds == targetBounds {
		layout.ImageBounds = targetBounds
		return layout
	}

	sourceWidth := sourceBounds.Dx()
	if sourceWidth <= 0 {
		sourceWidth = 1
	}
	sourceHeight := sourceBounds.Dy()
	if sourceHeight <= 0 {
		sourceHeight = 1
	}
	scaleX := float64(targetBounds.Dx()) / float64(sourceWidth)
	scaleY := float64(targetBounds.Dy()) / float64(sourceHeight)

	layout.ImageBounds = targetBounds
	layout.CardBounds = scaleRectangleBetweenBounds(layout.CardBounds, sourceBounds, targetBounds, scaleX, scaleY)
	layout.NameBand = scaleRectangleBetweenBounds(layout.NameBand, sourceBounds, targetBounds, scaleX, scaleY)
	if layout.HPBarY > 0 {
		scaledY := targetBounds.Min.Y + int(float64(layout.HPBarY-sourceBounds.Min.Y)*scaleY)
		if scaledY < targetBounds.Min.Y {
			scaledY = targetBounds.Min.Y
		}
		if scaledY >= targetBounds.Max.Y {
			scaledY = targetBounds.Max.Y - 1
		}
		layout.HPBarY = scaledY
	}

	return layout
}

func scaleRectangleBetweenBounds(
	rect image.Rectangle,
	sourceBounds image.Rectangle,
	targetBounds image.Rectangle,
	scaleX float64,
	scaleY float64,
) image.Rectangle {
	if rect.Empty() || sourceBounds.Empty() || targetBounds.Empty() {
		return image.Rectangle{}
	}

	x0 := targetBounds.Min.X + int(float64(rect.Min.X-sourceBounds.Min.X)*scaleX)
	y0 := targetBounds.Min.Y + int(float64(rect.Min.Y-sourceBounds.Min.Y)*scaleY)
	x1 := targetBounds.Min.X + int(float64(rect.Max.X-sourceBounds.Min.X)*scaleX)
	y1 := targetBounds.Min.Y + int(float64(rect.Max.Y-sourceBounds.Min.Y)*scaleY)
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}

	return image.Rect(x0, y0, x1, y1).Intersect(targetBounds)
}

func buildSpeciesNameAttempts(
	preprocessedImage image.Image,
	nameAnchors []appraisal.NameAnchor,
) []ocr.ExtractRequest {
	regions := appraisal.BuildSpeciesNameRegions(preprocessedImage.Bounds(), nameAnchors)
	if len(regions) == 0 {
		return nil
	}

	attempts := make([]ocr.ExtractRequest, 0, len(regions)*2)
	for _, region := range regions {
		attempts = append(attempts, ocr.ExtractRequest{
			Image:         preprocessedImage,
			Region:        region,
			PageSegMode:   speciesNamePageSegMode,
			CharWhitelist: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz .'-",
		})
		attempts = append(attempts, ocr.ExtractRequest{
			Image:         preprocessedImage,
			Region:        region,
			PageSegMode:   6,
			CharWhitelist: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz .'-",
		})
	}

	return attempts
}

func buildDeterministicSpeciesNameAttempts(preprocessedImage image.Image, nameROI appraisal.NameROI) []ocr.ExtractRequest {
	bounds := preprocessedImage.Bounds()
	if bounds.Empty() {
		return nil
	}

	region := nameROI.Region.Intersect(bounds)
	if region.Empty() {
		region = nameROI.Band.Intersect(bounds)
	}
	if region.Empty() {
		region = bounds
	}

	return []ocr.ExtractRequest{
		{
			Image:         preprocessedImage,
			Region:        region,
			PageSegMode:   speciesNamePageSegMode,
			CharWhitelist: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz .'-",
		},
		{
			Image:         preprocessedImage,
			Region:        region,
			PageSegMode:   6,
			CharWhitelist: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz .'-",
		},
	}
}

func selectFirstParsedAttempt(results []speciesOCRAttemptResult) (speciesOCRAttemptResult, bool) {
	var zero speciesOCRAttemptResult
	for _, result := range results {
		if result.Parsed.SpeciesNameNormalized == nil {
			continue
		}
		return result, true
	}
	return zero, false
}

func speciesAttemptSelectionScore(result speciesOCRAttemptResult) float64 {
	score := float64(result.Score)
	normalizedLength := 0
	if result.Parsed.SpeciesNameNormalized != nil {
		normalizedLength = len(strings.TrimSpace(*result.Parsed.SpeciesNameNormalized))
	}
	if normalizedLength >= 4 && normalizedLength <= 14 {
		score += 8
	}
	if normalizedLength > 0 {
		score += 3
	}

	if result.Canonical != nil {
		if result.Canonical.Mode == "exact" {
			score += 120
		} else if result.Canonical.Mode == "prefix" {
			score += 92
		} else if result.Canonical.Mode == "fuzzy" {
			score += 60 - float64(result.Canonical.Distance*8)
		}
	}

	return score
}

func writeDeterministicLayoutDebug(
	artifactWriter *imageArtifactWriter,
	decodedImage image.Image,
	preprocessedImage image.Image,
	detectedLayout appraisal.NameLayout,
	layout appraisal.NameLayout,
	nameROI appraisal.NameROI,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	summary := fmt.Sprintf(
		"detected_layout_image_bounds=%v scaled_layout_image_bounds=%v\nlayout_confidence=%.3f card=%v has_card=%t card_confidence=%.3f hp_y=%d has_hp=%t hp_confidence=%.3f name_band=%v failure_reason=%q\noriginal_card=%v original_hp_y=%d original_name_band=%v\nroi_region=%v roi_band=%v roi_method=%q roi_confidence=%.3f roi_failure_reason=%q\n",
		detectedLayout.ImageBounds,
		layout.ImageBounds,
		layout.Confidence,
		layout.CardBounds,
		layout.HasCard,
		layout.CardConfidence,
		layout.HPBarY,
		layout.HasHPBar,
		layout.HPConfidence,
		layout.NameBand,
		layout.FailureReason,
		detectedLayout.CardBounds,
		detectedLayout.HPBarY,
		detectedLayout.NameBand,
		nameROI.Region,
		nameROI.Band,
		nameROI.Method,
		nameROI.Confidence,
		nameROI.FailureReason,
	)
	if err := artifactWriter.writeText("layout_detection_meta", summary); err != nil {
		return err
	}

	if decodedImage != nil && !detectedLayout.CardBounds.Empty() {
		cardCrop, err := ocrInputImage(ocr.ExtractRequest{
			Image:  decodedImage,
			Region: detectedLayout.CardBounds,
		})
		if err == nil {
			if err := artifactWriter.write("layout_card_crop", cardCrop); err != nil {
				return err
			}
		}
	}
	if preprocessedImage != nil && !layout.NameBand.Empty() {
		bandCrop, err := ocrInputImage(ocr.ExtractRequest{
			Image:  preprocessedImage,
			Region: layout.NameBand,
		})
		if err == nil {
			if err := artifactWriter.write("layout_name_band", bandCrop); err != nil {
				return err
			}
		}
	}
	if preprocessedImage != nil && !nameROI.Region.Empty() {
		roiCrop, err := ocrInputImage(ocr.ExtractRequest{
			Image:  preprocessedImage,
			Region: nameROI.Region,
		})
		if err == nil {
			if err := artifactWriter.write("layout_name_roi", roiCrop); err != nil {
				return err
			}
		}
	}

	return nil
}

func writeSpeciesSelectionSummary(
	artifactWriter *imageArtifactWriter,
	selectedAttemptNumber int,
	selectionMode string,
	selectionSpecies string,
	selectedText string,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	if selectionMode == "" {
		selectionMode = "none"
	}
	summary := fmt.Sprintf(
		"selected_attempt=%d selection_mode=%q selection_species=%q selected_text=%q\n",
		selectedAttemptNumber,
		selectionMode,
		selectionSpecies,
		strings.TrimSpace(selectedText),
	)
	return artifactWriter.writeText("species_selection_meta", summary)
}

func writeOCRAttemptDebug(
	artifactWriter *imageArtifactWriter,
	attemptNumber int,
	request ocr.ExtractRequest,
	rawText string,
	ocrErr error,
	parsed appraisal.ParsedCandidate,
	score int,
	selectionScore float64,
	attemptLabel string,
	canonical *appraisal.CanonicalSpeciesMatch,
	selected bool,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	status := "ok"
	errText := ""
	if ocrErr != nil {
		status = "error"
		errText = ocrErr.Error()
	}

	parsedRaw := "<nil>"
	if parsed.SpeciesNameRaw != nil {
		parsedRaw = *parsed.SpeciesNameRaw
	}
	parsedNormalized := "<nil>"
	if parsed.SpeciesNameNormalized != nil {
		parsedNormalized = *parsed.SpeciesNameNormalized
	}
	canonicalSpecies := "<nil>"
	canonicalMode := "none"
	canonicalDistance := -1
	if canonical != nil {
		canonicalSpecies = canonical.SpeciesNormalized
		canonicalMode = canonical.Mode
		canonicalDistance = canonical.Distance
	}

	region := request.Region
	if region.Empty() && request.Image != nil {
		region = request.Image.Bounds()
	}
	imageBounds := image.Rectangle{}
	if request.Image != nil {
		imageBounds = request.Image.Bounds()
	}

	summary := fmt.Sprintf(
		"attempt=%d label=%s status=%s psm=%d selected=%t\nimage_bounds=%v region=%v char_whitelist=%q\nparse_score=%d selection_score=%.2f parsed_species_raw=%q parsed_species_normalized=%q canonical_species=%q canonical_mode=%q canonical_distance=%d\nocr_error=%q\nocr_text=%q\n",
		attemptNumber,
		attemptLabel,
		status,
		request.PageSegMode,
		selected,
		imageBounds,
		region,
		request.CharWhitelist,
		score,
		selectionScore,
		parsedRaw,
		parsedNormalized,
		canonicalSpecies,
		canonicalMode,
		canonicalDistance,
		errText,
		strings.TrimSpace(rawText),
	)

	label := "ocr_attempt_" + strconv.Itoa(attemptNumber) + "_psm_" + strconv.Itoa(request.PageSegMode) + "_meta"
	return artifactWriter.writeText(label, summary)
}

func writeCPAttemptDebug(
	artifactWriter *imageArtifactWriter,
	regionNumber int,
	attemptNumber int,
	request ocr.ExtractRequest,
	rawText string,
	parsedCPRaw *string,
	ocrErr error,
	attemptLabel string,
	selected bool,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	status := "ok"
	errText := ""
	if ocrErr != nil {
		status = "error"
		errText = ocrErr.Error()
	}

	parsedCP := "<nil>"
	if parsedCPRaw != nil {
		parsedCP = *parsedCPRaw
	}

	region := request.Region
	if region.Empty() && request.Image != nil {
		region = request.Image.Bounds()
	}
	imageBounds := image.Rectangle{}
	if request.Image != nil {
		imageBounds = request.Image.Bounds()
	}

	summary := fmt.Sprintf(
		"region=%d attempt=%d label=%s status=%s psm=%d selected=%t\nimage_bounds=%v region_bounds=%v char_whitelist=%q\nparsed_cp_raw=%q\nocr_error=%q\nocr_text=%q\n",
		regionNumber,
		attemptNumber,
		attemptLabel,
		status,
		request.PageSegMode,
		selected,
		imageBounds,
		region,
		request.CharWhitelist,
		parsedCP,
		errText,
		strings.TrimSpace(rawText),
	)

	label := "cp_ocr_region_" + strconv.Itoa(regionNumber) + "_attempt_" + strconv.Itoa(attemptNumber) + "_psm_" + strconv.Itoa(request.PageSegMode) + "_meta"
	return artifactWriter.writeText(label, summary)
}

func writeCPSelectionSummary(
	artifactWriter *imageArtifactWriter,
	cpRegions []image.Rectangle,
	selectedRegionNumber int,
	selectedAttemptNumber int,
	selectedCPRaw *string,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	selectedCP := ""
	if selectedCPRaw != nil {
		selectedCP = strings.TrimSpace(*selectedCPRaw)
	}

	regionLines := make([]string, 0, len(cpRegions))
	for idx, region := range cpRegions {
		regionLines = append(regionLines, fmt.Sprintf("region_%d=%v", idx+1, region))
	}

	summary := fmt.Sprintf(
		"%s\nselected_region=%d selected_attempt=%d selected_cp_raw=%q\n",
		strings.Join(regionLines, "\n"),
		selectedRegionNumber,
		selectedAttemptNumber,
		selectedCP,
	)
	return artifactWriter.writeText("cp_selection_meta", summary)
}

func writeHPAttemptDebug(
	artifactWriter *imageArtifactWriter,
	regionNumber int,
	attemptNumber int,
	request ocr.ExtractRequest,
	rawText string,
	parsedHPRaw *string,
	ocrErr error,
	attemptLabel string,
	selected bool,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	status := "ok"
	errText := ""
	if ocrErr != nil {
		status = "error"
		errText = ocrErr.Error()
	}

	parsedHP := "<nil>"
	if parsedHPRaw != nil {
		parsedHP = *parsedHPRaw
	}

	region := request.Region
	if region.Empty() && request.Image != nil {
		region = request.Image.Bounds()
	}
	imageBounds := image.Rectangle{}
	if request.Image != nil {
		imageBounds = request.Image.Bounds()
	}

	summary := fmt.Sprintf(
		"region=%d attempt=%d label=%s status=%s psm=%d selected=%t\nimage_bounds=%v region_bounds=%v char_whitelist=%q\nparsed_hp_raw=%q\nocr_error=%q\nocr_text=%q\n",
		regionNumber,
		attemptNumber,
		attemptLabel,
		status,
		request.PageSegMode,
		selected,
		imageBounds,
		region,
		request.CharWhitelist,
		parsedHP,
		errText,
		strings.TrimSpace(rawText),
	)

	label := "hp_ocr_region_" + strconv.Itoa(regionNumber) + "_attempt_" + strconv.Itoa(attemptNumber) + "_psm_" + strconv.Itoa(request.PageSegMode) + "_meta"
	return artifactWriter.writeText(label, summary)
}

func writeHPSelectionSummary(
	artifactWriter *imageArtifactWriter,
	hpRegions []image.Rectangle,
	selectedRegionNumber int,
	selectedAttemptNumber int,
	selectedHPRaw *string,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	selectedHP := ""
	if selectedHPRaw != nil {
		selectedHP = strings.TrimSpace(*selectedHPRaw)
	}

	regionLines := make([]string, 0, len(hpRegions))
	for idx, region := range hpRegions {
		regionLines = append(regionLines, fmt.Sprintf("region_%d=%v", idx+1, region))
	}

	summary := fmt.Sprintf(
		"%s\nselected_region=%d selected_attempt=%d selected_hp_raw=%q\n",
		strings.Join(regionLines, "\n"),
		selectedRegionNumber,
		selectedAttemptNumber,
		selectedHP,
	)
	return artifactWriter.writeText("hp_selection_meta", summary)
}

func writeIVSelectionSummary(
	artifactWriter *imageArtifactWriter,
	ivRegions []image.Rectangle,
	selectedRegionNumber int,
	selectedAttemptNumber int,
	selectedIVRaw appraisal.ParsedIVRaw,
	selectedMethod string,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	selectedAttack := ""
	if selectedIVRaw.AttackRaw != nil {
		selectedAttack = strings.TrimSpace(*selectedIVRaw.AttackRaw)
	}
	selectedDefense := ""
	if selectedIVRaw.DefenseRaw != nil {
		selectedDefense = strings.TrimSpace(*selectedIVRaw.DefenseRaw)
	}
	selectedStamina := ""
	if selectedIVRaw.StaminaRaw != nil {
		selectedStamina = strings.TrimSpace(*selectedIVRaw.StaminaRaw)
	}

	regionLines := make([]string, 0, len(ivRegions))
	for idx, region := range ivRegions {
		regionLines = append(regionLines, fmt.Sprintf("region_%d=%v", idx+1, region))
	}
	if strings.TrimSpace(selectedMethod) == "" {
		selectedMethod = "unknown"
	}

	summary := fmt.Sprintf(
		"%s\nselected_region=%d selected_attempt=%d selected_method=%q selected_iv_attack_raw=%q selected_iv_defense_raw=%q selected_iv_stamina_raw=%q\n",
		strings.Join(regionLines, "\n"),
		selectedRegionNumber,
		selectedAttemptNumber,
		selectedMethod,
		selectedAttack,
		selectedDefense,
		selectedStamina,
	)
	return artifactWriter.writeText("iv_selection_meta", summary)
}

func writeIVBarMeasurementSummary(
	artifactWriter *imageArtifactWriter,
	searchRegion image.Rectangle,
	measurements []ivBarMeasurement,
	parsed appraisal.ParsedIVRaw,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}

	attack := "<nil>"
	if parsed.AttackRaw != nil {
		attack = *parsed.AttackRaw
	}
	defense := "<nil>"
	if parsed.DefenseRaw != nil {
		defense = *parsed.DefenseRaw
	}
	stamina := "<nil>"
	if parsed.StaminaRaw != nil {
		stamina = *parsed.StaminaRaw
	}

	lines := []string{
		fmt.Sprintf("search_region=%v", searchRegion),
		fmt.Sprintf("parsed_attack=%q parsed_defense=%q parsed_stamina=%q", attack, defense, stamina),
	}

	if len(measurements) == 0 {
		lines = append(lines, "bars=none")
	} else {
		for _, measurement := range measurements {
			lines = append(lines, fmt.Sprintf(
				"bar_%d rows=%d-%d track=%d-%d fill_end=%d track_width=%d fill_width=%d ratio=%.3f value=%d",
				measurement.BarIndex,
				measurement.RowTop,
				measurement.RowBottom,
				measurement.TrackStart,
				measurement.TrackEnd,
				measurement.FillEnd,
				measurement.TrackWidth,
				measurement.FillWidth,
				measurement.Ratio,
				measurement.Value,
			))
		}
	}

	return artifactWriter.writeText("iv_bar_measurement_meta", strings.Join(lines, "\n")+"\n")
}

func (p imageProcessor) detectNameAnchorsWithEvidence(
	ctx context.Context,
	preprocessedImage image.Image,
	artifactWriter *imageArtifactWriter,
) ([]appraisal.NameAnchor, error) {
	type anchorSelection struct {
		anchor      appraisal.NameAnchor
		probeRegion image.Rectangle
		probeText   string
		probeScore  float64
		evidence    appraisal.NameAnchorOCREvidence
	}

	anchors := appraisal.DetectNameAnchors(preprocessedImage)
	if len(anchors) == 0 {
		return nil, nil
	}

	selections := make([]anchorSelection, 0, len(anchors))
	for _, anchor := range anchors {
		probeRegions := appraisal.NameAnchorProbeRegions(preprocessedImage.Bounds(), anchor)
		if len(probeRegions) == 0 {
			probeRegions = []image.Rectangle{appraisal.NameAnchorProbeRegion(preprocessedImage.Bounds(), anchor)}
		}

		bestSelection := anchorSelection{
			probeRegion: probeRegions[0],
			probeScore:  -1,
		}

		for _, probeRegion := range probeRegions {
			request := ocr.ExtractRequest{
				Image:         preprocessedImage,
				Region:        probeRegion,
				PageSegMode:   6,
				CharWhitelist: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 /",
			}

			inputImage, err := ocrInputImage(request)
			if err != nil {
				return nil, ProcessingError{
					Code:    "OCR_INPUT_PREP_FAILED",
					Message: err.Error(),
				}
			}

			text, err := p.ocrEngine.ExtractText(ctx, ocr.ExtractRequest{
				Image:         inputImage,
				PageSegMode:   request.PageSegMode,
				CharWhitelist: request.CharWhitelist,
			})
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				continue
			}

			evidence := appraisal.EvaluateNameAnchorOCREvidence(text)
			probeScore := anchorProbeEvidenceScore(evidence)
			if probeScore <= bestSelection.probeScore {
				continue
			}

			bestSelection.probeRegion = probeRegion
			bestSelection.probeText = text
			bestSelection.probeScore = probeScore
			bestSelection.evidence = evidence
		}

		bestSelection.anchor = appraisal.BlendNameAnchorConfidence(anchor, bestSelection.evidence)
		selections = append(selections, bestSelection)
	}

	sort.SliceStable(selections, func(i, j int) bool {
		if selections[i].anchor.Confidence == selections[j].anchor.Confidence {
			if selections[i].probeScore == selections[j].probeScore {
				return selections[i].anchor.CenterY < selections[j].anchor.CenterY
			}
			return selections[i].probeScore > selections[j].probeScore
		}
		return selections[i].anchor.Confidence > selections[j].anchor.Confidence
	})

	limit := 3
	if len(selections) < limit {
		limit = len(selections)
	}

	for idx := 0; idx < limit; idx++ {
		request := ocr.ExtractRequest{
			Image:  preprocessedImage,
			Region: selections[idx].probeRegion,
		}

		inputImage, err := ocrInputImage(request)
		if err != nil {
			return nil, ProcessingError{
				Code:    "OCR_INPUT_PREP_FAILED",
				Message: err.Error(),
			}
		}

		if err := artifactWriter.write("anchor_probe_"+strconv.Itoa(idx+1), inputImage); err != nil {
			return nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}

		summary := fmt.Sprintf(
			"confidence=%.3f geometry=%.3f ocr_score=%.3f probe_score=%.3f has_hp_token=%t has_hp_value_pattern=%t probe_region=%v\nocr_text=%q\n",
			selections[idx].anchor.Confidence,
			selections[idx].anchor.GeometryScore,
			selections[idx].anchor.OCRScore,
			selections[idx].probeScore,
			selections[idx].anchor.HasHPToken,
			selections[idx].anchor.HasHPValuePattern,
			selections[idx].probeRegion,
			strings.TrimSpace(selections[idx].probeText),
		)
		if err := artifactWriter.writeText("anchor_probe_"+strconv.Itoa(idx+1)+"_meta", summary); err != nil {
			return nil, ProcessingError{
				Code:    "ARTIFACT_DUMP_FAILED",
				Message: err.Error(),
			}
		}
	}

	result := make([]appraisal.NameAnchor, 0, len(selections))
	for _, selection := range selections {
		result = append(result, selection.anchor)
	}

	return result, nil
}

func anchorProbeEvidenceScore(evidence appraisal.NameAnchorOCREvidence) float64 {
	score := evidence.OCRScore
	if evidence.HasHPToken {
		score += 0.45
	}
	if evidence.HasHPValuePattern {
		score += 0.80
	}
	return score
}

func persistenceSessionID(job jobqueue.ClaimedJob) (string, error) {
	if strings.TrimSpace(job.ID) == "" {
		return "", fmt.Errorf("job id is required")
	}
	if strings.TrimSpace(job.UploadID) == "" {
		return "", fmt.Errorf("upload id is required")
	}

	sessionID := strings.TrimSpace(job.SessionID)
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}

	return sessionID, nil
}

func normalizePersistenceSourceType(sourceType string) string {
	normalized := strings.TrimSpace(sourceType)
	if normalized == "" {
		return appraisal.SourceTypeImage
	}
	return normalized
}

func (p imageProcessor) persistVideoFrames(
	ctx context.Context,
	job jobqueue.ClaimedJob,
	frames []processedFrame,
) (bool, int, error) {
	sessionID, err := persistenceSessionID(job)
	if err != nil {
		return false, 0, err
	}

	store, err := appraisal.NewSQLiteStore(p.databasePath)
	if err != nil {
		return false, 0, fmt.Errorf("open appraisal store: %w", err)
	}
	defer store.Close()

	acceptedReadings := make([]canonicalAcceptedReading, 0, len(frames))
	pendingReadings := make([]pendingVideoReading, 0, len(frames))

	for _, frame := range frames {
		if err := p.insertCandidate(
			ctx,
			store,
			job,
			sessionID,
			frame.parsed,
			frame.rawOCRText,
			frame.validation,
			appraisal.SourceTypeVideo,
			frame.timestampMS,
		); err != nil {
			return false, 0, err
		}

		accepted := rankAcceptedResultsForPendingOptions(frame.validation.AcceptedResults)
		if len(accepted) == 0 {
			continue
		}

		if len(accepted) > 1 {
			pendingReadings = append(pendingReadings, pendingVideoReading{
				AcceptedOptions:  append([]appraisal.AcceptedResultCandidate(nil), accepted...),
				FrameTimestampMS: frame.timestampMS,
			})
			continue
		}

		acceptedReadings = append(acceptedReadings, canonicalAcceptedReading{
			Accepted:         accepted[0],
			FrameTimestampMS: frame.timestampMS,
		})
	}

	dedupedPending := deduplicatePendingVideoReadings(pendingReadings)
	for _, reading := range dedupedPending {
		if err := p.insertPendingReadingWithOptions(
			ctx,
			store,
			job,
			sessionID,
			reading.AcceptedOptions,
			appraisal.SourceTypeVideo,
			reading.FrameTimestampMS,
		); err != nil {
			return false, 0, err
		}
	}

	dedupedAccepted := reduceAcceptedVideoReadings(acceptedReadings)
	for _, reading := range dedupedAccepted {
		if err := p.insertAcceptedResult(
			ctx,
			store,
			job,
			sessionID,
			reading.Accepted,
			appraisal.SourceTypeVideo,
			reading.FrameTimestampMS,
		); err != nil {
			return false, 0, err
		}
	}

	return len(dedupedPending) > 0, len(dedupedAccepted), nil
}

func (p imageProcessor) persistCandidateAndAccepted(
	ctx context.Context,
	job jobqueue.ClaimedJob,
	parsed appraisal.ParsedCandidate,
	rawOCRText string,
	validation appraisal.ValidationDecision,
	sourceType string,
	frameTimestampMS *int64,
) (int, error) {
	sessionID, err := persistenceSessionID(job)
	if err != nil {
		return 0, err
	}

	store, err := appraisal.NewSQLiteStore(p.databasePath)
	if err != nil {
		return 0, fmt.Errorf("open appraisal store: %w", err)
	}
	defer store.Close()

	sourceType = normalizePersistenceSourceType(sourceType)

	if err := p.insertCandidate(
		ctx,
		store,
		job,
		sessionID,
		parsed,
		rawOCRText,
		validation,
		sourceType,
		frameTimestampMS,
	); err != nil {
		return 0, err
	}

	accepted := rankAcceptedResultsForPendingOptions(validation.AcceptedResults)
	if len(accepted) == 0 {
		return 0, nil
	}

	if len(accepted) == 1 {
		if err := p.insertAcceptedResult(
			ctx,
			store,
			job,
			sessionID,
			accepted[0],
			sourceType,
			frameTimestampMS,
		); err != nil {
			return 0, err
		}
		return 1, nil
	}

	if err := p.insertPendingReadingWithOptions(
		ctx,
		store,
		job,
		sessionID,
		accepted,
		sourceType,
		frameTimestampMS,
	); err != nil {
		return 0, err
	}

	return len(accepted), nil
}

func (p imageProcessor) insertCandidate(
	ctx context.Context,
	store appraisal.Store,
	job jobqueue.ClaimedJob,
	sessionID string,
	parsed appraisal.ParsedCandidate,
	rawOCRText string,
	validation appraisal.ValidationDecision,
	sourceType string,
	frameTimestampMS *int64,
) error {
	_, err := store.InsertCandidate(ctx, appraisal.InsertCandidateParams{
		JobID:                 job.ID,
		UploadID:              job.UploadID,
		SessionID:             sessionID,
		SourceType:            sourceType,
		FrameTimestampMS:      frameTimestampMS,
		SpeciesNameRaw:        parsed.SpeciesNameRaw,
		SpeciesNameNormalized: parsed.SpeciesNameNormalized,
		SpeciesIsCanonical:    validation.SpeciesIsCanonical,
		CPRaw:                 parsed.CPRaw,
		HPRaw:                 parsed.HPRaw,
		IVAttackRaw:           parsed.IVAttackRaw,
		IVDefenseRaw:          parsed.IVDefenseRaw,
		IVStaminaRaw:          parsed.IVStaminaRaw,
		RawText:               normalizeRawOCRText(rawOCRText),
	})
	if err != nil {
		return fmt.Errorf("insert appraisal candidate: %w", err)
	}
	return nil
}

func (p imageProcessor) insertAcceptedResult(
	ctx context.Context,
	store appraisal.Store,
	job jobqueue.ClaimedJob,
	sessionID string,
	accepted appraisal.AcceptedResultCandidate,
	sourceType string,
	frameTimestampMS *int64,
) error {
	levelMethod := strings.TrimSpace(accepted.LevelMethod)
	if levelMethod == "" {
		levelMethod = appraisal.LevelMethodUnknown
	}

	_, err := store.InsertResult(ctx, appraisal.InsertResultParams{
		JobID:                job.ID,
		UploadID:             job.UploadID,
		SessionID:            sessionID,
		SpeciesName:          accepted.SpeciesName,
		CP:                   accepted.CP,
		HP:                   accepted.HP,
		PowerUpStardustCost:  0,
		IVAttack:             accepted.IVAttack,
		IVDefense:            accepted.IVDefense,
		IVStamina:            accepted.IVStamina,
		LevelEstimate:        accepted.LevelEstimate,
		LevelConfidence:      accepted.LevelConfidence,
		LevelMethod:          levelMethod,
		SourceType:           sourceType,
		FrameTimestampMS:     frameTimestampMS,
		ExtractionConfidence: nil,
	})
	if err != nil {
		return fmt.Errorf("insert appraisal result: %w", err)
	}
	return nil
}

func (p imageProcessor) insertPendingReadingWithOptions(
	ctx context.Context,
	store appraisal.Store,
	job jobqueue.ClaimedJob,
	sessionID string,
	accepted []appraisal.AcceptedResultCandidate,
	sourceType string,
	frameTimestampMS *int64,
) error {
	chosen := accepted[0]
	levelMethod := strings.TrimSpace(chosen.LevelMethod)
	if levelMethod == "" {
		levelMethod = appraisal.LevelMethodUnknown
	}

	options := make([]appraisal.InsertPendingSpeciesOptionParams, 0, len(accepted))
	for idx, acceptedResult := range accepted {
		matchMode := strings.TrimSpace(acceptedResult.MatchMode)
		if matchMode == "" {
			matchMode = "unknown"
		}

		normalizedSpeciesName := strings.TrimSpace(acceptedResult.SpeciesNameNormalized)
		if normalizedSpeciesName == "" {
			normalizedSpeciesName = strings.ToLower(strings.TrimSpace(acceptedResult.SpeciesName))
		}

		options = append(options, appraisal.InsertPendingSpeciesOptionParams{
			SpeciesName:           acceptedResult.SpeciesName,
			SpeciesNameNormalized: normalizedSpeciesName,
			MatchMode:             matchMode,
			MatchDistance:         acceptedResult.MatchDistance,
			OptionRank:            idx + 1,
		})
	}

	_, err := store.InsertPendingReadingWithOptions(ctx, appraisal.InsertPendingReadingWithOptionsParams{
		JobID:                job.ID,
		UploadID:             job.UploadID,
		SessionID:            sessionID,
		CP:                   chosen.CP,
		HP:                   chosen.HP,
		IVAttack:             chosen.IVAttack,
		IVDefense:            chosen.IVDefense,
		IVStamina:            chosen.IVStamina,
		LevelEstimate:        chosen.LevelEstimate,
		LevelConfidence:      chosen.LevelConfidence,
		LevelMethod:          levelMethod,
		SourceType:           sourceType,
		FrameTimestampMS:     frameTimestampMS,
		ExtractionConfidence: nil,
		Status:               jobqueue.JobStatusPendingUserDedup,
		Locked:               false,
		Options:              options,
	})
	if err != nil {
		return fmt.Errorf("insert appraisal pending reading: %w", err)
	}

	return nil
}

func rankAcceptedResultsForPendingOptions(
	accepted []appraisal.AcceptedResultCandidate,
) []appraisal.AcceptedResultCandidate {
	if len(accepted) == 0 {
		return nil
	}

	ranked := append([]appraisal.AcceptedResultCandidate(nil), accepted...)
	sort.SliceStable(ranked, func(i int, j int) bool {
		left := ranked[i]
		right := ranked[j]

		leftRank := pendingOptionModeRank(left.MatchMode)
		rightRank := pendingOptionModeRank(right.MatchMode)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.MatchDistance != right.MatchDistance {
			return left.MatchDistance < right.MatchDistance
		}

		leftName := strings.TrimSpace(left.SpeciesNameNormalized)
		if leftName == "" {
			leftName = strings.ToLower(strings.TrimSpace(left.SpeciesName))
		}
		rightName := strings.TrimSpace(right.SpeciesNameNormalized)
		if rightName == "" {
			rightName = strings.ToLower(strings.TrimSpace(right.SpeciesName))
		}
		if leftName != rightName {
			return leftName < rightName
		}

		return strings.TrimSpace(left.SpeciesName) < strings.TrimSpace(right.SpeciesName)
	})
	return ranked
}

func writeVideoFrameStabilitySummary(
	artifactWriter *imageArtifactWriter,
	labelPrefix string,
	timestampMS int64,
	assessment videoproc.StabilityAssessment,
	stableStreak int,
	acceptedForOCR bool,
) error {
	summary := buildVideoFrameStabilitySummary(timestampMS, assessment, stableStreak, acceptedForOCR)

	return artifactWriter.writeText(artifactLabel(labelPrefix, "frame_stability_meta"), summary)
}

func buildVideoFrameStabilitySummary(
	timestampMS int64,
	assessment videoproc.StabilityAssessment,
	stableStreak int,
	acceptedForOCR bool,
) string {
	return fmt.Sprintf(
		"timestamp_ms=%d stable=%t accepted_for_ocr=%t stable_streak=%d reason=%q\ncard_centered=%t card_confidence=%.3f white_ratio=%.3f motion_delta=%.3f\n",
		timestampMS,
		assessment.Stable,
		acceptedForOCR,
		stableStreak,
		assessment.Reason,
		assessment.CardCentered,
		assessment.CardConfidence,
		assessment.WhiteRatio,
		assessment.MotionDeltaScore,
	)
}

func writeVideoFrameDispositionImage(
	artifactWriter *imageArtifactWriter,
	frameNumber int,
	disposition string,
	img image.Image,
) error {
	if artifactWriter == nil {
		return fmt.Errorf("artifact writer is required")
	}
	if frameNumber <= 0 {
		return fmt.Errorf("frame number must be greater than zero")
	}
	if img == nil {
		return fmt.Errorf("artifact image is required")
	}

	normalizedDisposition := strings.ToLower(strings.TrimSpace(disposition))
	switch normalizedDisposition {
	case "processed", "duplicate", "jittered":
	default:
		normalizedDisposition = "jittered"
	}

	fileName := fmt.Sprintf("frame_%d_%s.png", frameNumber, normalizedDisposition)
	outputPath := filepath.Join(artifactWriter.jobDir, fileName)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create artifact file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("encode artifact image: %w", err)
	}

	return nil
}

func pendingOptionModeRank(mode string) int {
	switch mode {
	case "exact":
		return 0
	case "prefix":
		return 1
	case "fuzzy":
		return 2
	default:
		return 3
	}
}

func normalizeRawOCRText(rawText string) *string {
	normalized := strings.Join(strings.Fields(rawText), " ")
	if normalized == "" {
		return nil
	}
	return &normalized
}

func newImageArtifactWriter(databasePath string, jobID string) (*imageArtifactWriter, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("job id is required for artifact output")
	}

	artifactBaseDir, err := artifactBaseDirForDatabase(databasePath)
	if err != nil {
		return nil, err
	}

	jobDir := filepath.Join(artifactBaseDir, "worker-image-debug", jobID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return nil, fmt.Errorf("create job artifact directory: %w", err)
	}

	return &imageArtifactWriter{
		jobDir: jobDir,
		seq:    1,
	}, nil
}

func artifactBaseDirForDatabase(databasePath string) (string, error) {
	normalized := strings.TrimSpace(databasePath)
	if normalized == "" || !isLocalDatabaseURL(normalized) {
		return os.TempDir(), nil
	}

	if strings.HasPrefix(strings.ToLower(normalized), "file:") {
		normalized = normalized[len("file:"):]
	}

	absDatabasePath, err := filepath.Abs(normalized)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}

	return filepath.Dir(absDatabasePath), nil
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

func (w *imageArtifactWriter) write(label string, img image.Image) error {
	if w == nil {
		return fmt.Errorf("artifact writer is required")
	}
	if img == nil {
		return fmt.Errorf("artifact image is required")
	}

	fileName := fmt.Sprintf("%03d_%s.png", w.seq, sanitizeFileLabel(label))
	w.seq++
	outputPath := filepath.Join(w.jobDir, fileName)

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create artifact file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		return fmt.Errorf("encode artifact image: %w", err)
	}

	return nil
}

func (w *imageArtifactWriter) writeText(label string, content string) error {
	if w == nil {
		return fmt.Errorf("artifact writer is required")
	}

	fileName := fmt.Sprintf("%03d_%s.txt", w.seq, sanitizeFileLabel(label))
	w.seq++
	outputPath := filepath.Join(w.jobDir, fileName)

	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write artifact text file: %w", err)
	}
	w.textEntries = append(w.textEntries, artifactTextEntry{
		Label:   strings.TrimSpace(label),
		Content: content,
	})

	return nil
}

func (w *imageArtifactWriter) textEntryCount() int {
	if w == nil {
		return 0
	}
	return len(w.textEntries)
}

func (w *imageArtifactWriter) textEntriesSince(start int) []artifactTextEntry {
	if w == nil {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if start >= len(w.textEntries) {
		return nil
	}

	result := make([]artifactTextEntry, len(w.textEntries)-start)
	copy(result, w.textEntries[start:])
	return result
}

func sanitizeFileLabel(label string) string {
	normalized := strings.ToLower(strings.TrimSpace(label))
	if normalized == "" {
		return "image"
	}

	var builder strings.Builder
	builder.Grow(len(normalized))

	lastUnderscore := false
	for _, r := range normalized {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastUnderscore = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "image"
	}

	return result
}

func ocrInputImage(request ocr.ExtractRequest) (image.Image, error) {
	if request.Image == nil {
		return nil, fmt.Errorf("ocr request image is required")
	}
	if request.Region.Empty() {
		return request.Image, nil
	}

	bounds := request.Image.Bounds()
	clipped := request.Region.Intersect(bounds)
	if clipped.Empty() {
		return nil, fmt.Errorf("ocr region %v is outside image bounds %v", request.Region, bounds)
	}

	dst := image.NewRGBA(image.Rect(0, 0, clipped.Dx(), clipped.Dy()))
	draw.Draw(dst, dst.Bounds(), request.Image, clipped.Min, draw.Src)
	return dst, nil
}
