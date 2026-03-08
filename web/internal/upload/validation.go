package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
)

const (
	maxVideoBytes int64 = 75 * 1024 * 1024
	maxVideoSecs        = 90
)

const (
	ErrorCodeMissingFile          = "MISSING_FILE"
	ErrorCodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
	ErrorCodeVideoTooLarge        = "VIDEO_TOO_LARGE"
	ErrorCodeVideoTooLong         = "VIDEO_TOO_LONG"
)

// DurationProber retrieves media duration in seconds for a file path.
type DurationProber interface {
	DurationSeconds(ctx context.Context, filePath string) (float64, error)
}

// ValidatedMedia is the normalized upload metadata produced by validation.
type ValidatedMedia struct {
	Kind             string
	ContentType      string
	ByteSize         int64
	OriginalFilename string
	TempFilePath     string
}

// Cleanup removes the temporary file created during validation.
func (m ValidatedMedia) Cleanup() error {
	if m.TempFilePath == "" {
		return nil
	}
	if err := os.Remove(m.TempFilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove temp media file: %w", err)
	}
	return nil
}

// ValidationError describes a request validation failure mapped to error codes.
type ValidationError struct {
	HTTPStatus int
	Code       string
	Message    string
	Details    map[string]interface{}
}

func (e *ValidationError) Error() string {
	return e.Message
}

// AsValidationError unwraps a ValidationError from err when present.
func AsValidationError(err error) (*ValidationError, bool) {
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		return nil, false
	}
	return validationErr, true
}

// ValidateMultipartFile validates and normalizes a multipart upload file.
func ValidateMultipartFile(
	ctx context.Context,
	file multipart.File,
	header *multipart.FileHeader,
	prober DurationProber,
) (ValidatedMedia, error) {
	if file == nil || header == nil {
		return ValidatedMedia{}, &ValidationError{
			HTTPStatus: http.StatusBadRequest,
			Code:       ErrorCodeMissingFile,
			Message:    "Missing required file field",
		}
	}

	tempPath, written, err := writeMultipartToTempFile(file)
	if err != nil {
		return ValidatedMedia{}, err
	}

	cleanupTempOnError := func() {
		_ = os.Remove(tempPath)
	}

	contentType, err := sniffContentType(tempPath)
	if err != nil {
		cleanupTempOnError()
		return ValidatedMedia{}, err
	}

	kind := classifyKind(contentType)
	if kind == "" {
		cleanupTempOnError()
		return ValidatedMedia{}, &ValidationError{
			HTTPStatus: http.StatusUnsupportedMediaType,
			Code:       ErrorCodeUnsupportedMediaType,
			Message:    "Only image and video uploads are supported",
		}
	}

	byteSize := written
	if header.Size > 0 {
		byteSize = header.Size
	}

	if kind == KindVideo && byteSize > maxVideoBytes {
		cleanupTempOnError()
		return ValidatedMedia{}, &ValidationError{
			HTTPStatus: http.StatusBadRequest,
			Code:       ErrorCodeVideoTooLarge,
			Message:    "Video file exceeds 75 MB",
		}
	}

	if kind == KindVideo {
		if prober == nil {
			cleanupTempOnError()
			return ValidatedMedia{}, fmt.Errorf("duration prober is required for video validation")
		}

		durationSeconds, probeErr := prober.DurationSeconds(ctx, tempPath)
		if probeErr != nil {
			cleanupTempOnError()
			return ValidatedMedia{}, fmt.Errorf("probe video duration: %w", probeErr)
		}

		if durationSeconds > maxVideoSecs {
			cleanupTempOnError()
			return ValidatedMedia{}, &ValidationError{
				HTTPStatus: http.StatusBadRequest,
				Code:       ErrorCodeVideoTooLong,
				Message:    "Video duration exceeds 90 seconds",
				Details: map[string]interface{}{
					"maxSeconds": maxVideoSecs,
				},
			}
		}
	}

	return ValidatedMedia{
		Kind:             kind,
		ContentType:      contentType,
		ByteSize:         byteSize,
		OriginalFilename: header.Filename,
		TempFilePath:     tempPath,
	}, nil
}

func writeMultipartToTempFile(file multipart.File) (string, int64, error) {
	tempFile, err := os.CreateTemp("", "upload-validation-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}

	tempPath := tempFile.Name()
	written, copyErr := io.Copy(tempFile, file)
	closeErr := tempFile.Close()

	if copyErr != nil {
		_ = os.Remove(tempPath)
		return "", 0, fmt.Errorf("copy upload into temp file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return "", 0, fmt.Errorf("close temp file: %w", closeErr)
	}

	return tempPath, written, nil
}

func sniffContentType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open temp file for media sniffing: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read temp file for media sniffing: %w", err)
	}

	detected := http.DetectContentType(buffer[:n])
	if detected == "application/octet-stream" && isMP4Header(buffer[:n]) {
		return "video/mp4", nil
	}

	return detected, nil
}

func classifyKind(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return KindImage
	case strings.HasPrefix(contentType, "video/"):
		return KindVideo
	default:
		return ""
	}
}

func isMP4Header(header []byte) bool {
	return len(header) >= 8 && bytes.Equal(header[4:8], []byte("ftyp"))
}
