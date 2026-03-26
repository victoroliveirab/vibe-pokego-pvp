package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

func runClaimedJobLifecycle(
	ctx context.Context,
	queue jobqueue.Store,
	logger *slog.Logger,
	job jobqueue.ClaimedJob,
	workerID string,
	heartbeatInterval time.Duration,
	processor Processor,
	nowFn func() time.Time,
) {
	if processor == nil {
		processor = newStubProcessor(heartbeatInterval)
	}

	logger = logger.With(
		"job_id", job.ID,
		"upload_id", job.UploadID,
		"worker_id", workerID,
	)
	logger.Info("job processing started")

	lifecycleCtx, cancelLifecycle := context.WithCancel(ctx)
	defer cancelLifecycle()

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		heartbeatTicker := time.NewTicker(heartbeatInterval)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-lifecycleCtx.Done():
				return
			case <-heartbeatTicker.C:
				ok, err := queue.RefreshHeartbeat(ctx, job.ID, workerID, nowFn())
				if err != nil {
					logger.Warn("refresh heartbeat failed", "job_id", job.ID, "error", err)
					continue
				}
				if !ok {
					logger.Warn("heartbeat lost job ownership", "job_id", job.ID, "worker_id", workerID)
					cancelLifecycle()
					return
				}
			}
		}
	}()

	reportProgress := func(stage string, progress float64, progressDescription *string) error {
		ok, err := queue.UpdateJobProgress(ctx, job.ID, workerID, stage, progress, progressDescription, nowFn())
		if err != nil {
			return fmt.Errorf("update job progress: %w", err)
		}
		if !ok {
			return errOwnershipLost
		}

		logger.Info("job progress updated", "stage", stage, "progress", progress, "progress_description", progressDescription)

		return nil
	}

	runErr := processor.Process(lifecycleCtx, job, reportProgress)
	cancelLifecycle()
	<-heartbeatDone

	if runErr != nil {
		var pendingSignal pendingUserDedupSignal
		if errors.As(runErr, &pendingSignal) {
			ok, err := queue.MarkJobPendingUserDedup(ctx, job.ID, workerID, nowFn())
			if err != nil {
				logger.Error("mark job pending-user-dedup errored", "error", err)
				return
			}
			if !ok {
				logger.Warn("mark job pending-user-dedup lost ownership")
				return
			}

			logger.Info("job terminalized as pending-user-dedup")
			return
		}

		code, message := failureDetailsForError(runErr)
		ok, err := queue.MarkJobFailed(ctx, job.ID, workerID, code, message, nowFn())
		if err != nil {
			logger.Error("mark job failed errored", "error", err)
			return
		}
		if !ok {
			logger.Warn("mark job failed lost ownership")
			return
		}

		logger.Info("job terminalized as failed", "error_code", code)
		return
	}

	ok, err := queue.MarkJobSucceeded(ctx, job.ID, workerID, nowFn())
	if err != nil {
		logger.Error("mark job succeeded errored", "error", err)
		return
	}
	if !ok {
		logger.Warn("mark job succeeded lost ownership")
		return
	}

	logger.Info("job terminalized as succeeded")
}

var errOwnershipLost = errors.New("job ownership lost")

func failureDetailsForError(err error) (string, string) {
	if errors.Is(err, errOwnershipLost) {
		return "WORKER_OWNERSHIP_LOST", "Worker lost ownership while processing job"
	}

	var processingErr ProcessingError
	if errors.As(err, &processingErr) {
		return processingErr.Code, processingErr.Message
	}

	if errors.Is(err, context.Canceled) {
		return "WORKER_CANCELLED", "Job processing was cancelled"
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "WORKER_DEADLINE_EXCEEDED", "Job processing exceeded deadline"
	}

	return "PROCESSING_FAILED", err.Error()
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
