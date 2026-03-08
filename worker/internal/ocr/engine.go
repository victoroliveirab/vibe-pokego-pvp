package ocr

import (
	"context"
	"image"
)

// ExtractRequest defines OCR extraction inputs.
type ExtractRequest struct {
	Image         image.Image
	Region        image.Rectangle
	PageSegMode   int
	CharWhitelist string
}

// Engine extracts text from image content.
type Engine interface {
	ExtractText(ctx context.Context, request ExtractRequest) (string, error)
}

type noopEngine struct{}

// NewNoopEngine returns an engine that always extracts empty text.
func NewNoopEngine() Engine {
	return noopEngine{}
}

func (noopEngine) ExtractText(context.Context, ExtractRequest) (string, error) {
	return "", nil
}
