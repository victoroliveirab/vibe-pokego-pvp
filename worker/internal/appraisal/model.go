package appraisal

import (
	"crypto/rand"
	"fmt"
	"time"
)

const (
	SourceTypeImage = "IMAGE"
	SourceTypeVideo = "VIDEO"

	LevelMethodArcPosition = "ARC_POSITION"
	LevelMethodUnknown     = "UNKNOWN"

	PvPEvalQueueStatusPending    = "PENDING"
	PvPEvalQueueStatusProcessing = "PROCESSING"
	PvPEvalQueueStatusSucceeded  = "SUCCEEDED"
	PvPEvalQueueStatusFailed     = "FAILED"
)

// Candidate represents a raw OCR extraction attempt.
type Candidate struct {
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

// Result represents an accepted canonical appraisal result.
type Result struct {
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

// PvPEvaluationQueueItem represents one queue row for deferred PvP evaluation processing.
type PvPEvaluationQueueItem struct {
	ID                string
	AppraisalResultID string
	Status            string
	RetryCount        int
	LastError         *string
	Locked            bool
	NextRetryAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// UpsertPvPEvaluationParams describes one raw max-CP evaluation row to persist.
type UpsertPvPEvaluationParams struct {
	ID                 string
	MaxCP              int
	EvaluatedSpeciesID string
	BestLevel          float64
	BestCP             int
	StatProduct        float64
	RankPosition       int
	Percentage         float64
	CreatedAt          time.Time
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
