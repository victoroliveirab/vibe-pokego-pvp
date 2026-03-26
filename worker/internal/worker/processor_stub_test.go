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
		progress float64
	}

	var updates []progressUpdate
	err := processor.Process(
		context.Background(),
		jobqueue.ClaimedJob{ID: "job-1"},
		func(stage string, progress float64, progressDescription *string) error {
			updates = append(updates, progressUpdate{stage: stage, progress: progress})
			if progressDescription != nil {
				t.Fatalf("expected stub processor to omit progress descriptions, got %q", *progressDescription)
			}
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
		{stage: jobqueue.StageDownloadingMedia, progress: 3},
		{stage: jobqueue.StagePostprocessing, progress: 95},
		{stage: jobqueue.StagePersistingResults, progress: 95},
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
		func(stage string, progress float64, progressDescription *string) error {
			_ = stage
			_ = progress
			_ = progressDescription
			return nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
