package videoproc

import "image"

// FrameSample contains a sampled video frame and its timestamp.
type FrameSample struct {
	TimestampMS int64
	Image       image.Image
}

type ErrorCode string

const (
	ErrorCodeProbeFailed     ErrorCode = "VIDEO_PROBE_FAILED"
	ErrorCodeSampleFailed    ErrorCode = "VIDEO_SAMPLE_FAILED"
	ErrorCodeDecodeFailed    ErrorCode = "VIDEO_FRAME_DECODE_FAILED"
	ErrorCodeInvalidInterval ErrorCode = "VIDEO_INVALID_SAMPLE_INTERVAL"
)

// Error represents a structured video sampling failure.
type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Message
	}
	return e.Message + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
