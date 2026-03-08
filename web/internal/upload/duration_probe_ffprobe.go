package upload

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const defaultFFprobeBinary = "ffprobe"

// FFprobeDurationProber probes video duration using ffprobe.
type FFprobeDurationProber struct {
	Binary string
}

// NewFFprobeDurationProber creates a duration prober backed by ffprobe.
func NewFFprobeDurationProber() *FFprobeDurationProber {
	return &FFprobeDurationProber{
		Binary: defaultFFprobeBinary,
	}
}

// DurationSeconds returns the duration of the media at filePath in seconds.
func (p *FFprobeDurationProber) DurationSeconds(ctx context.Context, filePath string) (float64, error) {
	binary := strings.TrimSpace(p.Binary)
	if binary == "" {
		binary = defaultFFprobeBinary
	}

	cmd := exec.CommandContext(
		ctx,
		binary,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("run ffprobe: %w", err)
	}

	durationRaw := strings.TrimSpace(string(out))
	duration, err := strconv.ParseFloat(durationRaw, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ffprobe duration %q: %w", durationRaw, err)
	}

	return duration, nil
}
