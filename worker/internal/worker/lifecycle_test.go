package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"log/slog"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

func TestRunClaimedJobLifecycleWritesHeartbeatsAndTerminalFailure(t *testing.T) {
	now := time.Date(2026, time.March, 2, 13, 0, 0, 0, time.UTC)
	store := &fakeQueueStore{}
	logger := slog.New(slog.NewTextHandler(testWriter{t: t}, nil))

	runClaimedJobLifecycle(
		context.Background(),
		store,
		logger,
		jobqueue.ClaimedJob{ID: "job-1", UploadID: "upload-1"},
		"worker-1",
		10*time.Millisecond,
		newStubProcessor(10*time.Millisecond),
		func() time.Time { return now },
	)

	if store.progressUpdates == 0 {
		t.Fatal("expected progress updates during lifecycle")
	}
	if store.heartbeatUpdates == 0 {
		t.Fatal("expected heartbeat updates during lifecycle")
	}
	if store.markFailedCalls != 1 {
		t.Fatalf("expected 1 terminal failure write, got %d", store.markFailedCalls)
	}
	if store.markPendingUserDedupCalls != 0 {
		t.Fatalf("expected no pending-user-dedup terminal writes, got %d", store.markPendingUserDedupCalls)
	}
	if store.markSucceededCalls != 0 {
		t.Fatalf("expected no success terminal writes, got %d", store.markSucceededCalls)
	}
	if store.failedCode != errorCodeNoAppraisals {
		t.Fatalf("expected error code %q, got %q", errorCodeNoAppraisals, store.failedCode)
	}
	if store.failedMessage != errorMessageNoAppraisals {
		t.Fatalf("expected error message %q, got %q", errorMessageNoAppraisals, store.failedMessage)
	}
}

func TestRunClaimedJobLifecycleInvokesProcessorAndMarksSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 2, 13, 10, 0, 0, time.UTC)
	store := &fakeQueueStore{}
	logger := slog.New(slog.NewTextHandler(testWriter{t: t}, nil))
	processor := processorFunc(func(ctx context.Context, job jobqueue.ClaimedJob, report ProgressReporter) error {
		if err := report(jobqueue.StageDecodingImage, 22, nil); err != nil {
			return err
		}
		return nil
	})

	runClaimedJobLifecycle(
		context.Background(),
		store,
		logger,
		jobqueue.ClaimedJob{ID: "job-2", UploadID: "upload-2"},
		"worker-2",
		10*time.Millisecond,
		processor,
		func() time.Time { return now },
	)

	if store.progressUpdates == 0 {
		t.Fatal("expected progress updates from processor callback")
	}
	if store.markSucceededCalls != 1 {
		t.Fatalf("expected 1 terminal success write, got %d", store.markSucceededCalls)
	}
	if store.markPendingUserDedupCalls != 0 {
		t.Fatalf("expected no pending-user-dedup terminal writes, got %d", store.markPendingUserDedupCalls)
	}
	if store.markFailedCalls != 0 {
		t.Fatalf("expected no terminal failure writes, got %d", store.markFailedCalls)
	}
}

func TestRunClaimedJobLifecycleMarksPendingUserDedup(t *testing.T) {
	now := time.Date(2026, time.March, 2, 13, 20, 0, 0, time.UTC)
	store := &fakeQueueStore{}
	logger := slog.New(slog.NewTextHandler(testWriter{t: t}, nil))
	processor := processorFunc(func(ctx context.Context, job jobqueue.ClaimedJob, report ProgressReporter) error {
		if err := report(jobqueue.StagePersistingResults, 95, nil); err != nil {
			return err
		}
		return pendingUserDedupSignal{}
	})

	runClaimedJobLifecycle(
		context.Background(),
		store,
		logger,
		jobqueue.ClaimedJob{ID: "job-3", UploadID: "upload-3"},
		"worker-3",
		10*time.Millisecond,
		processor,
		func() time.Time { return now },
	)

	if store.markPendingUserDedupCalls != 1 {
		t.Fatalf("expected 1 pending-user-dedup terminal write, got %d", store.markPendingUserDedupCalls)
	}
	if store.markSucceededCalls != 0 {
		t.Fatalf("expected no terminal success writes, got %d", store.markSucceededCalls)
	}
	if store.markFailedCalls != 0 {
		t.Fatalf("expected no terminal failure writes, got %d", store.markFailedCalls)
	}
}

type fakeQueueStore struct {
	mu sync.Mutex

	progressUpdates           int
	heartbeatUpdates          int
	markFailedCalls           int
	markSucceededCalls        int
	markPendingUserDedupCalls int
	failedCode                string
	failedMessage             string
}

func (f *fakeQueueStore) Close() error { return nil }

func (f *fakeQueueStore) ClaimNextQueuedJob(context.Context, string, time.Time) (jobqueue.ClaimedJob, bool, error) {
	return jobqueue.ClaimedJob{}, false, nil
}

func (f *fakeQueueStore) UpdateJobProgress(context.Context, string, string, string, float64, *string, time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progressUpdates++
	return true, nil
}

func (f *fakeQueueStore) RefreshHeartbeat(context.Context, string, string, time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeatUpdates++
	return true, nil
}

func (f *fakeQueueStore) FailExpiredProcessingJobs(context.Context, time.Time, time.Time) (int64, error) {
	return 0, nil
}

func (f *fakeQueueStore) MarkJobSucceeded(context.Context, string, string, time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markSucceededCalls++
	return true, nil
}

func (f *fakeQueueStore) MarkJobPendingUserDedup(context.Context, string, string, time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markPendingUserDedupCalls++
	return true, nil
}

func (f *fakeQueueStore) MarkJobFailed(
	ctx context.Context,
	jobID string,
	workerID string,
	code string,
	message string,
	now time.Time,
) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markFailedCalls++
	f.failedCode = code
	f.failedMessage = message
	_ = ctx
	_ = jobID
	_ = workerID
	_ = now
	return true, nil
}

type testWriter struct {
	t *testing.T
}

type processorFunc func(context.Context, jobqueue.ClaimedJob, ProgressReporter) error

func (f processorFunc) Process(ctx context.Context, job jobqueue.ClaimedJob, report ProgressReporter) error {
	return f(ctx, job, report)
}

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
