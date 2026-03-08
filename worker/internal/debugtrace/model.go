package debugtrace

import (
	"crypto/rand"
	"fmt"
	"time"
)

const (
	FrameStatusProcessed        = "processed"
	FrameStatusSkippedStability = "skipped_stability"
	FrameStatusError            = "error"
)

const (
	MilestoneDownloading    = "downloading"
	MilestoneDecoding       = "decoding"
	MilestoneSampling       = "sampling"
	MilestoneExtracting     = "extracting"
	MilestonePostprocessing = "postprocessing"
	MilestonePersisting     = "persisting"
	MilestoneProcessing     = "processing"
	MilestoneSpecies        = "species"
	MilestoneCP             = "cp"
	MilestoneHP             = "hp"
	MilestoneIV             = "iv"
)

// JobDebug represents one row in job_debug_jobs.
type JobDebug struct {
	JobID                    string
	UploadID                 string
	SessionID                string
	Kind                     string
	ProcessingStartedAt      time.Time
	DownloadingFinishedAt    *time.Time
	DecodingFinishedAt       *time.Time
	SamplingFinishedAt       *time.Time
	ExtractingFinishedAt     *time.Time
	PostprocessingFinishedAt *time.Time
	PersistingFinishedAt     *time.Time
	ProcessingFinishedAt     *time.Time
	SpeciesFinishedAt        *time.Time
	CPFinishedAt             *time.Time
	HPFinishedAt             *time.Time
	IVFinishedAt             *time.Time
	DownloadMetaJSON         *string
	DecodeMetaJSON           *string
	SamplingMetaJSON         *string
	PostprocessingMetaJSON   *string
	PersistingMetaJSON       *string
	TerminalMetaJSON         *string
	ErrorCode                *string
	ErrorMessage             *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// FrameDebug represents one row in job_debug_frames.
type FrameDebug struct {
	ID                       string
	JobID                    string
	UploadID                 string
	SessionID                string
	SourceType               string
	FrameIndex               int
	FrameTimestampMS         *int64
	FrameStatus              string
	FrameStartedAt           *time.Time
	FrameFinishedAt          time.Time
	FrameDurationMS          int64
	SpeciesFinishedAt        *time.Time
	CPFinishedAt             *time.Time
	HPFinishedAt             *time.Time
	IVFinishedAt             *time.Time
	LayoutMetaJSON           *string
	SpeciesMetaJSON          *string
	CPMetaJSON               *string
	HPMetaJSON               *string
	IVMetaJSON               *string
	IVBarMeasurementMetaJSON *string
	FrameStabilityMetaJSON   *string
	SelectionMetaJSON        *string
	CreatedAt                time.Time
}

// UpsertJobDebugParams carries write payload for job_debug_jobs upserts.
type UpsertJobDebugParams struct {
	JobID                    string
	UploadID                 string
	SessionID                string
	Kind                     string
	ProcessingStartedAt      time.Time
	DownloadingFinishedAt    *time.Time
	DecodingFinishedAt       *time.Time
	SamplingFinishedAt       *time.Time
	ExtractingFinishedAt     *time.Time
	PostprocessingFinishedAt *time.Time
	PersistingFinishedAt     *time.Time
	ProcessingFinishedAt     *time.Time
	SpeciesFinishedAt        *time.Time
	CPFinishedAt             *time.Time
	HPFinishedAt             *time.Time
	IVFinishedAt             *time.Time
	DownloadMetaJSON         *string
	DecodeMetaJSON           *string
	SamplingMetaJSON         *string
	PostprocessingMetaJSON   *string
	PersistingMetaJSON       *string
	TerminalMetaJSON         *string
	ErrorCode                *string
	ErrorMessage             *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// UpdateJobDebugMilestoneParams updates one milestone timestamp and optional JSON metadata.
type UpdateJobDebugMilestoneParams struct {
	JobID      string
	Milestone  string
	FinishedAt time.Time
	MetaJSON   *string
	UpdatedAt  time.Time
}

// InsertFrameDebugParams carries insert payload for job_debug_frames.
type InsertFrameDebugParams struct {
	ID                       string
	JobID                    string
	UploadID                 string
	SessionID                string
	SourceType               string
	FrameIndex               int
	FrameTimestampMS         *int64
	FrameStatus              string
	FrameStartedAt           *time.Time
	FrameFinishedAt          time.Time
	FrameDurationMS          int64
	SpeciesFinishedAt        *time.Time
	CPFinishedAt             *time.Time
	HPFinishedAt             *time.Time
	IVFinishedAt             *time.Time
	LayoutMetaJSON           *string
	SpeciesMetaJSON          *string
	CPMetaJSON               *string
	HPMetaJSON               *string
	IVMetaJSON               *string
	IVBarMeasurementMetaJSON *string
	FrameStabilityMetaJSON   *string
	SelectionMetaJSON        *string
	CreatedAt                time.Time
}

// MarkJobDebugTerminalParams records terminal details for a job_debug_jobs row.
type MarkJobDebugTerminalParams struct {
	JobID                string
	ProcessingFinishedAt time.Time
	TerminalMetaJSON     *string
	ErrorCode            *string
	ErrorMessage         *string
	UpdatedAt            time.Time
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}

	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16],
	), nil
}
