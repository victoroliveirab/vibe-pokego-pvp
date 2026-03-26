package jobqueue

import "time"

const (
	JobStatusQueued           = "QUEUED"
	JobStatusProcessing       = "PROCESSING"
	JobStatusSucceeded        = "SUCCEEDED"
	JobStatusFailed           = "FAILED"
	JobStatusPendingUserDedup = "PENDING_USER_DEDUP"
)

const (
	StageDownloadingMedia       = "DOWNLOADING_MEDIA"
	StageDecodingImage          = "DECODING_IMAGE"
	StageDecodingVideo          = "DECODING_VIDEO"
	StageSamplingFrames         = "SAMPLING_FRAMES"
	StageDetectingAppraisal     = "DETECTING_APPRAISAL_SCREENS"
	StageExtractingAppraisal    = "EXTRACTING_APPRAISAL"
	StagePostprocessing         = "POSTPROCESSING"
	StagePersistingResults      = "PERSISTING_RESULTS"
	ErrorCodeWorkerTimeout      = "WORKER_TIMEOUT"
	ErrorMessageWorkerTimeout   = "Job exceeded processing lease timeout"
	DefaultClaimStage           = StageDownloadingMedia
	DefaultClaimInitialProgress = 0
)

// ClaimedJob is the queue payload returned to a worker after a successful claim.
type ClaimedJob struct {
	ID        string
	UploadID  string
	SessionID string
	Status    string
	Progress  float64
	Stage     string
	WorkerID  string
	ClaimedAt time.Time
}
