package videoproc

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFFmpegSamplerSampleFramesFixture(t *testing.T) {
	requireBinary(t, "ffmpeg")
	requireBinary(t, "ffprobe")

	sampler := NewFFmpegSampler(300 * time.Millisecond)
	samples, err := sampler.SampleFrames(context.Background(), videoFixturePath("valid_short_1s.mp4"))
	if err != nil {
		t.Fatalf("expected fixture sampling to succeed, got: %v", err)
	}

	if len(samples) != 4 {
		t.Fatalf("expected 4 sampled frames for 1s fixture at 300ms, got %d", len(samples))
	}

	previousTimestamp := int64(-1)
	for idx, sample := range samples {
		if sample.Image == nil {
			t.Fatalf("expected sampled frame image at index %d", idx)
		}
		if sample.TimestampMS <= previousTimestamp {
			t.Fatalf("expected strictly increasing timestamps, got %d after %d", sample.TimestampMS, previousTimestamp)
		}
		previousTimestamp = sample.TimestampMS
	}

	for idx := 1; idx < len(samples); idx++ {
		delta := samples[idx].TimestampMS - samples[idx-1].TimestampMS
		if delta < 275 || delta > 325 {
			t.Fatalf("expected ~300ms cadence, got %dms between sample %d and %d", delta, idx-1, idx)
		}
	}
}

func TestFFmpegSamplerSampleFramesCorruptVideoReturnsStructuredError(t *testing.T) {
	requireBinary(t, "ffprobe")

	sampler := NewFFmpegSampler(300 * time.Millisecond)
	_, err := sampler.SampleFrames(context.Background(), videoFixturePath("corrupt.mp4"))
	if err == nil {
		t.Fatal("expected corrupt fixture to fail")
	}

	var sampleErr *Error
	if !errors.As(err, &sampleErr) {
		t.Fatalf("expected structured Error, got: %T", err)
	}

	if sampleErr.Code != ErrorCodeProbeFailed {
		t.Fatalf("expected error code %q, got %q", ErrorCodeProbeFailed, sampleErr.Code)
	}
}

func TestBuildTimestampsRejectsZeroInterval(t *testing.T) {
	_, err := buildTimestamps(1, 0)
	if err == nil {
		t.Fatal("expected zero interval to fail")
	}

	var sampleErr *Error
	if !errors.As(err, &sampleErr) {
		t.Fatalf("expected structured Error, got: %T", err)
	}

	if sampleErr.Code != ErrorCodeInvalidInterval {
		t.Fatalf("expected code %q, got %q", ErrorCodeInvalidInterval, sampleErr.Code)
	}
}

func TestBuildTimestampsExcludesExactDurationBoundary(t *testing.T) {
	timestamps, err := buildTimestamps(3.0, 300*time.Millisecond)
	if err != nil {
		t.Fatalf("expected timestamp generation to succeed, got: %v", err)
	}

	expected := []int64{0, 300, 600, 900, 1200, 1500, 1800, 2100, 2400, 2700}
	if len(timestamps) != len(expected) {
		t.Fatalf("expected %d timestamps, got %d (%v)", len(expected), len(timestamps), timestamps)
	}
	for idx := range expected {
		if timestamps[idx] != expected[idx] {
			t.Fatalf("expected timestamp[%d]=%d, got %d", idx, expected[idx], timestamps[idx])
		}
	}
}

func videoFixturePath(name string) string {
	return filepath.Join("..", "..", "testdata", "videos", name)
}

func requireBinary(t *testing.T, binary string) {
	t.Helper()
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("skipping because %s is not available: %v", binary, err)
	}
}
