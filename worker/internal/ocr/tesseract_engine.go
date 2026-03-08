package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultPageSegMode = 6

type tesseractEngine struct {
	binary string
}

// NewTesseractEngine returns an OCR engine backed by the tesseract CLI.
func NewTesseractEngine() Engine {
	return &tesseractEngine{
		binary: "tesseract",
	}
}

func (e *tesseractEngine) ExtractText(ctx context.Context, request ExtractRequest) (string, error) {
	if request.Image == nil {
		return "", fmt.Errorf("extract image is required")
	}

	source := request.Image
	if !request.Region.Empty() {
		cropped, err := cropToRegion(request.Image, request.Region)
		if err != nil {
			return "", err
		}
		source = cropped
	}

	tempDir, err := os.MkdirTemp("", "tesseract-ocr-*")
	if err != nil {
		return "", fmt.Errorf("create tesseract temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input.png")
	if err := writePNG(inputPath, source); err != nil {
		return "", err
	}

	pageSegMode := request.PageSegMode
	if pageSegMode <= 0 {
		pageSegMode = defaultPageSegMode
	}

	args := []string{inputPath, "stdout", "--psm", strconv.Itoa(pageSegMode)}
	if request.CharWhitelist != "" {
		args = append(args, "-c", "tessedit_char_whitelist="+request.CharWhitelist)
	}

	command := exec.CommandContext(ctx, e.binary, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("run tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(string(output)), nil
}

func writePNG(path string, src image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create tesseract input image: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, src); err != nil {
		return fmt.Errorf("encode tesseract input image: %w", err)
	}

	return nil
}

func cropToRegion(src image.Image, region image.Rectangle) (image.Image, error) {
	bounds := src.Bounds()
	clipped := region.Intersect(bounds)
	if clipped.Empty() {
		return nil, fmt.Errorf("ocr region %v is outside image bounds %v", region, bounds)
	}

	dst := image.NewRGBA(image.Rect(0, 0, clipped.Dx(), clipped.Dy()))
	draw.Draw(dst, dst.Bounds(), src, clipped.Min, draw.Src)
	return dst, nil
}

func isTesseractMissing(err error) bool {
	var execErr *exec.Error
	return errors.As(err, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound)
}
