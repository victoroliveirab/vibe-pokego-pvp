package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

func TestStubProcessorReportsProgressAndReturnsStructuredFailure(t *testing.T) {
	processor := newStubProcessor(0)

	type progressUpdate struct {
		stage    string
		progress int
	}

	var updates []progressUpdate
	err := processor.Process(
		context.Background(),
		jobqueue.ClaimedJob{ID: "job-1"},
		func(stage string, progress int) error {
			updates = append(updates, progressUpdate{stage: stage, progress: progress})
			return nil
		},
	)
	if err == nil {
		t.Fatal("expected structured processor failure")
	}

	var processingErr ProcessingError
	if !errors.As(err, &processingErr) {
		t.Fatalf("expected ProcessingError, got %T", err)
	}
	if processingErr.Code != errorCodeNoAppraisals {
		t.Fatalf("expected code %q, got %q", errorCodeNoAppraisals, processingErr.Code)
	}
	if processingErr.Message != errorMessageNoAppraisals {
		t.Fatalf("expected message %q, got %q", errorMessageNoAppraisals, processingErr.Message)
	}

	expected := []progressUpdate{
		{stage: jobqueue.StageDownloadingMedia, progress: 6},
		{stage: jobqueue.StagePostprocessing, progress: 88},
		{stage: jobqueue.StagePersistingResults, progress: 96},
	}

	if len(updates) != len(expected) {
		t.Fatalf("expected %d progress updates, got %d", len(expected), len(updates))
	}
	for i := range expected {
		if updates[i] != expected[i] {
			t.Fatalf("expected update %d to be %#v, got %#v", i, expected[i], updates[i])
		}
	}
}

func TestStubProcessorRespectsContextCancellation(t *testing.T) {
	processor := newStubProcessor(100 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := processor.Process(
		ctx,
		jobqueue.ClaimedJob{ID: "job-2"},
		func(stage string, progress int) error {
			_ = stage
			_ = progress
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
