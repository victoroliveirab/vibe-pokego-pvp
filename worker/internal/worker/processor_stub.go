package worker

import (
	"context"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

const (
	errorCodeNoAppraisals    = "NO_APPRAISALS_FOUND"
	errorMessageNoAppraisals = "No readable appraisals detected"
)

type stubProcessor struct {
	stepDelay time.Duration
}

// newStubProcessor returns deterministic placeholder processing until OCR stories land.
func newStubProcessor(heartbeatInterval time.Duration) Processor {
	return stubProcessor{
		stepDelay: heartbeatInterval + 200*time.Millisecond,
	}
}

func (p stubProcessor) Process(
	ctx context.Context,
	_ jobqueue.ClaimedJob,
	reportProgress ProgressReporter,
) error {
	steps := []struct {
		stage    string
		progress float64
	}{
		{stage: jobqueue.StageDownloadingMedia, progress: 3},
		{stage: jobqueue.StagePostprocessing, progress: 95},
		{stage: jobqueue.StagePersistingResults, progress: 95},
	}

	for _, step := range steps {
		if err := reportProgress(step.stage, step.progress, nil); err != nil {
			return err
		}

		if err := sleepWithContext(ctx, p.stepDelay); err != nil {
			return err
		}
	}

	return ProcessingError{
		Code:    errorCodeNoAppraisals,
		Message: errorMessageNoAppraisals,
	}
}
