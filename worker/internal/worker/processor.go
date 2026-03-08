package worker

import (
	"context"
	"fmt"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

// ProgressReporter persists stage/progress updates for an owned job.
type ProgressReporter func(stage string, progress int) error

// Processor executes job-specific work while reporting lifecycle progress.
type Processor interface {
	Process(ctx context.Context, job jobqueue.ClaimedJob, reportProgress ProgressReporter) error
}

// ProcessingError encodes structured terminal failure details.
type ProcessingError struct {
	Code    string
	Message string
}

func (e ProcessingError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
