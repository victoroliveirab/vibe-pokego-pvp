package videoproc

import (
	"context"
	"fmt"
	"image"
	"os/exec"
	"strconv"
	"strings"
)

type osExecRunner struct{}

func (osExecRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

func (s *FFmpegSampler) probeDurationSeconds(ctx context.Context, filePath string) (float64, error) {
	binary := strings.TrimSpace(s.FFprobeBinary)
	if binary == "" {
		binary = defaultFFprobeBinary
	}

	out, err := s.commandRunner().CombinedOutput(
		ctx,
		binary,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	if err != nil {
		return 0, &Error{
			Code:    ErrorCodeProbeFailed,
			Message: "probe video duration",
			Err:     commandError(err, out),
		}
	}

	raw := strings.TrimSpace(string(out))
	durationSeconds, parseErr := strconv.ParseFloat(raw, 64)
	if parseErr != nil {
		return 0, &Error{
			Code:    ErrorCodeProbeFailed,
			Message: fmt.Sprintf("parse ffprobe duration %q", raw),
			Err:     parseErr,
		}
	}

	return durationSeconds, nil
}

func (s *FFmpegSampler) extractFrame(ctx context.Context, filePath string, timestampMS int64) (image.Image, error) {
	binary := strings.TrimSpace(s.FFmpegBinary)
	if binary == "" {
		binary = defaultFFmpegBinary
	}

	seekSeconds := fmt.Sprintf("%.3f", float64(timestampMS)/1000.0)
	out, err := s.commandRunner().CombinedOutput(
		ctx,
		binary,
		"-v", "error",
		"-i", filePath,
		"-ss", seekSeconds,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "png",
		"-",
	)
	if err != nil {
		return nil, &Error{
			Code:    ErrorCodeSampleFailed,
			Message: fmt.Sprintf("sample frame at %dms", timestampMS),
			Err:     commandError(err, out),
		}
	}

	return decodePNGFrame(out, timestampMS)
}

func (s *FFmpegSampler) commandRunner() commandRunner {
	if s.runner == nil {
		return osExecRunner{}
	}
	return s.runner
}

func commandError(err error, out []byte) error {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, text)
}
