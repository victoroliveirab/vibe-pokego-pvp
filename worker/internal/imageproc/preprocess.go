package imageproc

import (
	"fmt"
	"image"
	"image/color"
)

const defaultUpscaleFactor = 2

// PreprocessOptions configures baseline OCR preprocessing.
type PreprocessOptions struct {
	UpscaleFactor int
	Threshold     uint8
}

// PreprocessForOCR applies grayscale, contrast normalization, thresholding, and resize.
func PreprocessForOCR(src image.Image) (*image.Gray, error) {
	return Preprocess(src, PreprocessOptions{
		UpscaleFactor: defaultUpscaleFactor,
		Threshold:     0,
	})
}

// Preprocess applies baseline OCR preprocessing options.
func Preprocess(src image.Image, options PreprocessOptions) (*image.Gray, error) {
	if src == nil {
		return nil, fmt.Errorf("source image is required")
	}

	upscaleFactor := options.UpscaleFactor
	if upscaleFactor <= 0 {
		upscaleFactor = defaultUpscaleFactor
	}

	gray := toGray(src)
	normalized := normalizeContrast(gray)
	thresholded := applyThreshold(normalized, options.Threshold)
	resized := resizeNearest(thresholded, upscaleFactor)

	return resized, nil
}

func toGray(src image.Image) *image.Gray {
	bounds := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.SetGray(x-bounds.Min.X, y-bounds.Min.Y, color.GrayModel.Convert(src.At(x, y)).(color.Gray))
		}
	}

	return dst
}

func normalizeContrast(src *image.Gray) *image.Gray {
	dst := image.NewGray(src.Bounds())

	minValue := uint8(255)
	maxValue := uint8(0)
	for _, px := range src.Pix {
		if px < minValue {
			minValue = px
		}
		if px > maxValue {
			maxValue = px
		}
	}

	if maxValue == minValue {
		copy(dst.Pix, src.Pix)
		return dst
	}

	denominator := int(maxValue) - int(minValue)
	for i, px := range src.Pix {
		scaled := (int(px) - int(minValue)) * 255 / denominator
		dst.Pix[i] = uint8(scaled)
	}

	return dst
}

func applyThreshold(src *image.Gray, configuredThreshold uint8) *image.Gray {
	dst := image.NewGray(src.Bounds())
	threshold := configuredThreshold
	if threshold == 0 {
		threshold = meanIntensity(src.Pix)
	}

	for i, px := range src.Pix {
		if px >= threshold {
			dst.Pix[i] = 255
			continue
		}
		dst.Pix[i] = 0
	}

	return dst
}

func meanIntensity(pixels []uint8) uint8 {
	if len(pixels) == 0 {
		return 128
	}

	sum := 0
	for _, px := range pixels {
		sum += int(px)
	}

	return uint8(sum / len(pixels))
}

func resizeNearest(src *image.Gray, factor int) *image.Gray {
	if factor <= 1 {
		dst := image.NewGray(src.Bounds())
		copy(dst.Pix, src.Pix)
		return dst
	}

	srcBounds := src.Bounds()
	dstWidth := srcBounds.Dx() * factor
	dstHeight := srcBounds.Dy() * factor
	dst := image.NewGray(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		sourceY := y / factor
		for x := 0; x < dstWidth; x++ {
			sourceX := x / factor
			dst.SetGray(x, y, src.GrayAt(sourceX, sourceY))
		}
	}

	return dst
}
