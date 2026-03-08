package videoproc

import (
	"image"
	"image/color"
	"testing"
)

func TestFrameStabilityEvaluatorStableCenteredCardWithLowMotion(t *testing.T) {
	evaluator := NewFrameStabilityEvaluator()

	previous, _ := syntheticStabilityFrame(0)
	current, _ := syntheticStabilityFrame(0)
	assessment := evaluator.Assess(current, previous)

	if !assessment.Stable {
		t.Fatalf("expected stable frame assessment, got stable=%t reason=%q", assessment.Stable, assessment.Reason)
	}
	if !assessment.CardCentered {
		t.Fatal("expected centered card")
	}
	if assessment.WhiteRatio < stabilityMinWhiteRatio {
		t.Fatalf("expected white ratio >= %.2f, got %.3f", stabilityMinWhiteRatio, assessment.WhiteRatio)
	}
	if assessment.MotionDeltaScore > stabilityMaxMeanMotionDeltaLuma {
		t.Fatalf("expected motion delta <= %.2f, got %.3f", stabilityMaxMeanMotionDeltaLuma, assessment.MotionDeltaScore)
	}
}

func TestFrameStabilityEvaluatorRejectsOffCenterCard(t *testing.T) {
	evaluator := NewFrameStabilityEvaluator()

	current, _ := syntheticStabilityFrame(210)
	assessment := evaluator.Assess(current, nil)
	if assessment.Stable {
		t.Fatal("expected off-center card to be unstable")
	}
	if assessment.Reason != "card_not_centered" {
		t.Fatalf("expected reason card_not_centered, got %q", assessment.Reason)
	}
}

func TestFrameStabilityEvaluatorRejectsLowWhiteRatio(t *testing.T) {
	evaluator := NewFrameStabilityEvaluator()

	current, card := syntheticStabilityFrame(0)
	dimRegion := image.Rect(card.Min.X+40, card.Min.Y+420, card.Max.X-40, card.Max.Y-180)
	fillRect(current, dimRegion, color.RGBA{R: 128, G: 128, B: 128, A: 255})

	assessment := evaluator.Assess(current, nil)
	if assessment.Stable {
		t.Fatal("expected low-white-ratio frame to be unstable")
	}
	if assessment.Reason != "card_not_white_enough" {
		t.Fatalf("expected reason card_not_white_enough, got %q", assessment.Reason)
	}
}

func TestFrameStabilityEvaluatorRejectsHighMotionDelta(t *testing.T) {
	evaluator := NewFrameStabilityEvaluator()

	previous, card := syntheticStabilityFrame(0)
	current, _ := syntheticStabilityFrame(0)
	animatedRegion := image.Rect(card.Min.X+80, card.Min.Y+520, card.Max.X-140, card.Max.Y-160)
	fillRect(current, animatedRegion, color.RGBA{R: 240, G: 120, B: 70, A: 255})

	assessment := evaluator.Assess(current, previous)
	if assessment.Stable {
		t.Fatalf("expected high-motion frame to be unstable, got stable reason=%q delta=%.3f", assessment.Reason, assessment.MotionDeltaScore)
	}
	if assessment.Reason != "motion_too_high" {
		t.Fatalf("expected reason motion_too_high, got %q", assessment.Reason)
	}
}

func syntheticStabilityFrame(shiftX int) (*image.RGBA, image.Rectangle) {
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	fillRect(img, img.Bounds(), color.RGBA{R: 38, G: 42, B: 56, A: 255})

	card := image.Rect(140+shiftX, 620, 940+shiftX, 1820).Intersect(img.Bounds())
	fillRect(img, card, color.RGBA{R: 245, G: 245, B: 245, A: 255})

	hpY := card.Min.Y + int(float64(card.Dy())*0.24)
	hp := image.Rect(
		card.Min.X+int(float64(card.Dx())*0.20),
		hpY-2,
		card.Max.X-int(float64(card.Dx())*0.20),
		hpY+3,
	)
	fillRect(img, hp, color.RGBA{R: 88, G: 208, B: 128, A: 255})

	ivTop := card.Min.Y + int(float64(card.Dy())*0.56)
	barHeight := 12
	barGap := 58
	barLeft := card.Min.X + int(float64(card.Dx())*0.10)
	barRight := card.Min.X + int(float64(card.Dx())*0.66)
	for idx := 0; idx < 3; idx++ {
		top := ivTop + idx*barGap
		track := image.Rect(barLeft, top, barRight, top+barHeight)
		fillRect(img, track, color.RGBA{R: 214, G: 214, B: 214, A: 255})
		fill := image.Rect(barLeft, top, barLeft+int(float64(track.Dx())*0.62), top+barHeight)
		fillRect(img, fill, color.RGBA{R: 236, G: 153, B: 63, A: 255})
	}

	return img, card
}

func fillRect(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	r := rect.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}
