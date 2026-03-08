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
		progress int
	}{
		{stage: jobqueue.StageDownloadingMedia, progress: 6},
		{stage: jobqueue.StagePostprocessing, progress: 88},
		{stage: jobqueue.StagePersistingResults, progress: 96},
	}

	for _, step := range steps {
		if err := reportProgress(step.stage, step.progress); err != nil {
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
