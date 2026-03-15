package upload

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

const (
	KindImage = "image"
	KindVideo = "video"

	JobStatusQueued           = "QUEUED"
	JobStatusProcessing       = "PROCESSING"
	JobStatusSucceeded        = "SUCCEEDED"
	JobStatusFailed           = "FAILED"
	JobStatusPendingUserDedup = "PENDING_USER_DEDUP"
)

var ErrJobNotFound = errors.New("job not found")
var ErrJobRetryNotAllowed = errors.New("job retry not allowed")
var ErrPendingReadingNotFound = errors.New("pending reading not found")
var ErrPendingReadingLocked = errors.New("pending reading locked")
var ErrPendingOptionNotFound = errors.New("pending option not found")

// Upload stores metadata about a submitted media file.
type Upload struct {
	ID          string
	SessionID   string
	Kind        string
	MediaURL    string
	ContentType string
	ByteSize    int64
	CreatedAt   time.Time
}

// Job represents an immutable processing job tied to an upload.
type Job struct {
	ID        string
	UploadID  string
	SessionID string
	Status    string
	Progress  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RetryJob represents a child job created from an existing parent job.
type RetryJob struct {
	ID          string
	ParentJobID string
	UploadID    string
	SessionID   string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// JobStatusRecord stores a polling-ready projection for a job.
type JobStatusRecord struct {
	ID           string
	UploadID     string
	SessionID    string
	Status       string
	Progress     int
	Stage        *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	FinishedAt   *time.Time
	ErrorCode    *string
	ErrorMessage *string
}

// PokemonResultRecord stores one accepted appraisal result row.
type PokemonResultRecord struct {
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
	MaxCPEvaluations     []PokemonResultMaxCPEvaluationRecord
	CreatedAt            time.Time
}

// PokemonResultMaxCPEvaluationRecord stores one persisted max-CP evaluation for an appraisal result.
type PokemonResultMaxCPEvaluationRecord struct {
	MaxCP              int
	EvaluatedSpeciesID string
	BestLevel          float64
	BestCP             int
	StatProduct        float64
	Rank               int
	Percentage         float64
}

// PendingSpeciesOptionRecord stores one selectable species option for a pending reading.
type PendingSpeciesOptionRecord struct {
	ID                    string
	PendingReadingID      string
	SpeciesName           string
	SpeciesNameNormalized string
	MatchMode             string
	MatchDistance         int
	OptionRank            int
	CreatedAt             time.Time
}

// PendingSpeciesReadingRecord stores one unresolved ambiguous reading and its options.
type PendingSpeciesReadingRecord struct {
	ID                   string
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
	CreatedAt            time.Time
	Options              []PendingSpeciesOptionRecord
}

// ResolvePendingReadingParams carries the payload for finalizing a pending reading.
type ResolvePendingReadingParams struct {
	ReadingID string
	OptionID  string
	SessionID string
	Now       time.Time
}

// NewUploadID creates a UUID v4 identifier for uploads.
func NewUploadID() (string, error) {
	return newID()
}

// NewJobID creates a UUID v4 identifier for jobs.
func NewJobID() (string, error) {
	return newID()
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
