package appraisal

import (
	"image"
	"image/color"
	"testing"
)

func TestDetectNameLayoutFindsCardAndHPBar(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1920))

	fillRect(img, img.Bounds(), color.RGBA{R: 36, G: 18, B: 72, A: 255})
	card := image.Rect(120, 620, 960, 1820)
	fillRect(img, card, color.RGBA{R: 246, G: 246, B: 246, A: 255})

	hpY := 880
	for y := hpY - 2; y <= hpY+2; y++ {
		for x := 300; x <= 780; x++ {
			img.Set(x, y, color.RGBA{R: 88, G: 208, B: 128, A: 255})
		}
	}

	layout := DetectNameLayout(img)

	if !layout.HasCard {
		t.Fatalf("expected card detection, got fallback with reason %q", layout.FailureReason)
	}
	if !layout.HasHPBar {
		t.Fatalf("expected hp bar detection, got fallback with reason %q", layout.FailureReason)
	}
	if layout.HPBarY < hpY-8 || layout.HPBarY > hpY+8 {
		t.Fatalf("expected hp y around %d, got %d", hpY, layout.HPBarY)
	}
	if layout.NameBand.Empty() {
		t.Fatal("expected non-empty name band")
	}
	if layout.NameBand.Max.Y >= layout.HPBarY {
		t.Fatalf("expected name band above hp bar, got band %v hpY %d", layout.NameBand, layout.HPBarY)
	}
}

func TestDetectNameLayoutFallsBackOnBlankImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	fillRect(img, img.Bounds(), color.RGBA{R: 18, G: 18, B: 18, A: 255})

	layout := DetectNameLayout(img)
	if layout.CardBounds.Empty() {
		t.Fatal("expected fallback card bounds")
	}
	if layout.NameBand.Empty() {
		t.Fatal("expected fallback name band")
	}
	if layout.FailureReason == "" {
		t.Fatal("expected failure reason for fallback path")
	}
}

func fillRect(img *image.RGBA, rect image.Rectangle, color color.RGBA) {
	r := rect.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, color)
		}
	}
}
