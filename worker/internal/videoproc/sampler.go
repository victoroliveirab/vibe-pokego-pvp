package videoproc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"time"
)

const (
	defaultFFmpegBinary  = "ffmpeg"
	defaultFFprobeBinary = "ffprobe"
)

const DefaultInterval = 300 * time.Millisecond

// Sampler samples decoded video frames.
type Sampler interface {
	SampleFrames(ctx context.Context, filePath string) ([]FrameSample, error)
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

// FFmpegSampler samples frames with ffprobe + ffmpeg.
type FFmpegSampler struct {
	FFprobeBinary string
	FFmpegBinary  string
	Interval      time.Duration

	runner commandRunner
}

// NewFFmpegSampler creates a sampler with defaults when fields are not configured.
func NewFFmpegSampler(interval time.Duration) *FFmpegSampler {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &FFmpegSampler{
		FFprobeBinary: defaultFFprobeBinary,
		FFmpegBinary:  defaultFFmpegBinary,
		Interval:      interval,
		runner:        osExecRunner{},
	}
}

// SampleFrames returns sampled frames at the configured cadence.
func (s *FFmpegSampler) SampleFrames(ctx context.Context, filePath string) ([]FrameSample, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, &Error{
			Code:    ErrorCodeProbeFailed,
			Message: "video file path is required",
		}
	}

	interval := s.sampleInterval()
	if interval <= 0 {
		return nil, &Error{
			Code:    ErrorCodeInvalidInterval,
			Message: "video sampling interval must be greater than zero",
		}
	}

	durationSeconds, err := s.probeDurationSeconds(ctx, filePath)
	if err != nil {
		return nil, err
	}

	timestamps, err := buildTimestamps(durationSeconds, interval)
	if err != nil {
		return nil, err
	}

	samples := make([]FrameSample, 0, len(timestamps))
	for _, timestampMS := range timestamps {
		frameImage, err := s.extractFrame(ctx, filePath, timestampMS)
		if err != nil {
			return nil, err
		}
		samples = append(samples, FrameSample{
			TimestampMS: timestampMS,
			Image:       frameImage,
		})
	}

	return samples, nil
}

func (s *FFmpegSampler) sampleInterval() time.Duration {
	if s.Interval <= 0 {
		return DefaultInterval
	}
	return s.Interval
}

func buildTimestamps(durationSeconds float64, interval time.Duration) ([]int64, error) {
	intervalMS := int64(interval / time.Millisecond)
	if intervalMS <= 0 {
		return nil, &Error{
			Code:    ErrorCodeInvalidInterval,
			Message: "video sampling interval must be greater than zero",
		}
	}

	durationMS := int64(math.Round(durationSeconds * 1000))
	if durationMS < 0 {
		durationMS = 0
	}
	if durationMS == 0 {
		return []int64{0}, nil
	}

	capEstimate := int(durationMS/intervalMS) + 1
	timestamps := make([]int64, 0, capEstimate)
	// Avoid sampling exactly at duration boundary; that timestamp can legitimately
	// fall after the last decodable frame for some containers/encoders.
	for timestampMS := int64(0); timestampMS < durationMS; timestampMS += intervalMS {
		timestamps = append(timestamps, timestampMS)
	}

	if len(timestamps) == 0 {
		timestamps = append(timestamps, 0)
	}

	return timestamps, nil
}

func decodePNGFrame(frameBytes []byte, timestampMS int64) (image.Image, error) {
	decodedImage, err := png.Decode(bytes.NewReader(frameBytes))
	if err != nil {
		return nil, &Error{
			Code:    ErrorCodeDecodeFailed,
			Message: fmt.Sprintf("decode sampled frame at %dms", timestampMS),
			Err:     err,
		}
	}
	return decodedImage, nil
}
