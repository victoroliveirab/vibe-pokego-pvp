package appraisal

import (
	"image"
	"image/color"
	"testing"
)

func TestDetectSpeciesNameROIReturnsTightRegionInsideBand(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 1200, 2400))
	fillGray(img, img.Bounds(), 255)

	band := image.Rect(200, 740, 1000, 980)
	layout := NameLayout{
		ImageBounds: img.Bounds(),
		CardBounds:  image.Rect(120, 620, 1080, 2100),
		HPBarY:      1020,
		NameBand:    band,
	}

	// Draw a synthetic "name" line with multiple glyph-like blocks.
	drawBlock(img, image.Rect(360, 820, 400, 910), 0)
	drawBlock(img, image.Rect(430, 840, 470, 910), 0)
	drawBlock(img, image.Rect(500, 810, 540, 910), 0)
	drawBlock(img, image.Rect(570, 845, 612, 910), 0)
	drawBlock(img, image.Rect(640, 825, 680, 910), 0)
	drawBlock(img, image.Rect(710, 835, 750, 910), 0)

	roi := DetectSpeciesNameROI(img, layout)
	if roi.Region.Empty() {
		t.Fatal("expected non-empty roi")
	}
	if !roi.Region.In(band) && !band.In(roi.Region) {
		// ROI can include small padding outside band; it should still intersect meaningfully.
		if roi.Region.Intersect(band).Empty() {
			t.Fatalf("expected roi to intersect band, band=%v roi=%v", band, roi.Region)
		}
	}
	if roi.Region.Min.Y > 830 || roi.Region.Max.Y < 900 {
		t.Fatalf("expected roi vertical coverage around text line, got %v", roi.Region)
	}
	if roi.Confidence <= 0 {
		t.Fatalf("expected positive confidence, got %f", roi.Confidence)
	}
}

func TestDetectSpeciesNameROIFallsBackToBandWhenSignalIsWeak(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 1200, 2400))
	fillGray(img, img.Bounds(), 255)

	layout := NameLayout{
		ImageBounds: img.Bounds(),
		CardBounds:  image.Rect(120, 620, 1080, 2100),
		HPBarY:      1020,
		NameBand:    image.Rect(200, 740, 1000, 980),
	}

	roi := DetectSpeciesNameROI(img, layout)
	if roi.Region != layout.NameBand {
		t.Fatalf("expected fallback to band, got %v (band %v)", roi.Region, layout.NameBand)
	}
	if roi.FailureReason == "" {
		t.Fatal("expected failure reason for fallback")
	}
}

func fillGray(img *image.Gray, rect image.Rectangle, value uint8) {
	r := rect.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetGray(x, y, color.Gray{Y: value})
		}
	}
}

func drawBlock(img *image.Gray, rect image.Rectangle, value uint8) {
	fillGray(img, rect, value)
}
