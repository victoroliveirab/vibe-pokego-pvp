package videoproc

import (
	"image"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
)

const (
	stabilityMinCardConfidence      = 0.35
	stabilityMaxCenterOffsetRatio   = 0.07
	stabilityMaxMarginImbalance     = 0.12
	stabilityMinCardWidthRatio      = 0.62
	stabilityMaxCardWidthRatio      = 0.96
	stabilityBrightLumaThreshold    = 180
	stabilityMinWhiteRatio          = 0.52
	stabilityMotionGridWidth        = 48
	stabilityMotionGridHeight       = 72
	stabilityMaxMeanMotionDeltaLuma = 12.0
)

// StabilityAssessment contains deterministic frame stability signals.
type StabilityAssessment struct {
	Stable           bool
	Reason           string
	CardCentered     bool
	CardConfidence   float64
	WhiteRatio       float64
	MotionDeltaScore float64
}

// FrameStabilityEvaluator determines whether a sampled frame is stable enough for OCR.
type FrameStabilityEvaluator struct{}

// NewFrameStabilityEvaluator returns a default evaluator.
func NewFrameStabilityEvaluator() FrameStabilityEvaluator {
	return FrameStabilityEvaluator{}
}

// Assess evaluates whether the current frame is stable.
func (FrameStabilityEvaluator) Assess(current image.Image, previous image.Image) StabilityAssessment {
	assessment := StabilityAssessment{
		Stable: false,
		Reason: "unknown",
	}

	if current == nil {
		assessment.Reason = "nil_frame"
		return assessment
	}

	currentBounds := current.Bounds()
	if currentBounds.Empty() {
		assessment.Reason = "empty_bounds"
		return assessment
	}

	currentLayout := appraisal.DetectNameLayout(current)
	assessment.CardConfidence = currentLayout.CardConfidence
	if !currentLayout.HasCard {
		assessment.Reason = "card_not_detected"
		return assessment
	}
	if currentLayout.CardConfidence < stabilityMinCardConfidence {
		assessment.Reason = "card_confidence_low"
		return assessment
	}

	cardBounds := currentLayout.CardBounds.Intersect(currentBounds)
	if cardBounds.Empty() {
		assessment.Reason = "card_bounds_empty"
		return assessment
	}

	cardWidthRatio := float64(cardBounds.Dx()) / float64(maxInt(1, currentBounds.Dx()))
	if cardWidthRatio < stabilityMinCardWidthRatio || cardWidthRatio > stabilityMaxCardWidthRatio {
		assessment.Reason = "card_width_out_of_range"
		return assessment
	}

	assessment.CardCentered = cardIsCentered(cardBounds, currentBounds)
	if !assessment.CardCentered {
		assessment.Reason = "card_not_centered"
		return assessment
	}

	whiteRegion := insetRect(cardBounds, int(float64(cardBounds.Dx())*0.05), int(float64(cardBounds.Dy())*0.08))
	if whiteRegion.Empty() {
		whiteRegion = cardBounds
	}
	assessment.WhiteRatio = brightPixelRatio(current, whiteRegion, stabilityBrightLumaThreshold)
	if assessment.WhiteRatio < stabilityMinWhiteRatio {
		assessment.Reason = "card_not_white_enough"
		return assessment
	}

	motionDelta := 0.0
	if previous != nil {
		previousBounds := previous.Bounds()
		if !previousBounds.Empty() {
			previousLayout := appraisal.DetectNameLayout(previous)
			if previousLayout.HasCard {
				prevCardBounds := previousLayout.CardBounds.Intersect(previousBounds)
				if !prevCardBounds.Empty() {
					motionDelta = meanAbsoluteMotionDelta(
						current,
						motionRegion(cardBounds, currentBounds),
						previous,
						motionRegion(prevCardBounds, previousBounds),
					)
				}
			}
		}
	}
	assessment.MotionDeltaScore = motionDelta
	if motionDelta > stabilityMaxMeanMotionDeltaLuma {
		assessment.Reason = "motion_too_high"
		return assessment
	}

	assessment.Stable = true
	assessment.Reason = "stable"
	return assessment
}

func cardIsCentered(card image.Rectangle, bounds image.Rectangle) bool {
	if bounds.Empty() || card.Empty() {
		return false
	}

	imageCenterX := bounds.Min.X + bounds.Dx()/2
	cardCenterX := card.Min.X + card.Dx()/2
	centerOffsetRatio := float64(absInt(imageCenterX-cardCenterX)) / float64(maxInt(1, bounds.Dx()))

	leftMargin := card.Min.X - bounds.Min.X
	rightMargin := bounds.Max.X - card.Max.X
	marginImbalanceRatio := float64(absInt(leftMargin-rightMargin)) / float64(maxInt(1, bounds.Dx()))

	return centerOffsetRatio <= stabilityMaxCenterOffsetRatio &&
		marginImbalanceRatio <= stabilityMaxMarginImbalance
}

func motionRegion(card image.Rectangle, bounds image.Rectangle) image.Rectangle {
	region := insetRect(card, int(float64(card.Dx())*0.08), int(float64(card.Dy())*0.10)).Intersect(bounds)
	if region.Empty() {
		return card.Intersect(bounds)
	}
	return region
}

func insetRect(rect image.Rectangle, dx int, dy int) image.Rectangle {
	if rect.Empty() {
		return rect
	}

	left := rect.Min.X + maxInt(0, dx)
	right := rect.Max.X - maxInt(0, dx)
	top := rect.Min.Y + maxInt(0, dy)
	bottom := rect.Max.Y - maxInt(0, dy)
	if right <= left || bottom <= top {
		return image.Rectangle{}
	}

	return image.Rect(left, top, right, bottom)
}

func brightPixelRatio(src image.Image, region image.Rectangle, threshold int) float64 {
	region = region.Intersect(src.Bounds())
	if region.Empty() {
		return 0
	}

	stepX := maxInt(1, region.Dx()/120)
	stepY := maxInt(1, region.Dy()/180)
	samples := 0
	bright := 0
	for y := region.Min.Y; y < region.Max.Y; y += stepY {
		for x := region.Min.X; x < region.Max.X; x += stepX {
			samples++
			if int(pixelLuma(src, x, y)) >= threshold {
				bright++
			}
		}
	}

	if samples == 0 {
		return 0
	}
	return float64(bright) / float64(samples)
}

func meanAbsoluteMotionDelta(
	current image.Image,
	currentRegion image.Rectangle,
	previous image.Image,
	previousRegion image.Rectangle,
) float64 {
	currentRegion = currentRegion.Intersect(current.Bounds())
	previousRegion = previousRegion.Intersect(previous.Bounds())
	if currentRegion.Empty() || previousRegion.Empty() {
		return 0
	}

	width := stabilityMotionGridWidth
	height := stabilityMotionGridHeight
	if width <= 0 || height <= 0 {
		return 0
	}

	sum := 0
	count := 0
	for y := 0; y < height; y++ {
		currentY := sampleCoordinate(currentRegion.Min.Y, currentRegion.Max.Y, y, height)
		previousY := sampleCoordinate(previousRegion.Min.Y, previousRegion.Max.Y, y, height)
		for x := 0; x < width; x++ {
			currentX := sampleCoordinate(currentRegion.Min.X, currentRegion.Max.X, x, width)
			previousX := sampleCoordinate(previousRegion.Min.X, previousRegion.Max.X, x, width)

			currentLuma := int(pixelLuma(current, currentX, currentY))
			previousLuma := int(pixelLuma(previous, previousX, previousY))
			sum += absInt(currentLuma - previousLuma)
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func sampleCoordinate(minValue int, maxValue int, index int, total int) int {
	if maxValue <= minValue {
		return minValue
	}
	if total <= 1 {
		return minValue
	}

	span := maxValue - minValue
	offset := (index*span + span/(2*total)) / total
	value := minValue + offset
	if value >= maxValue {
		value = maxValue - 1
	}
	if value < minValue {
		value = minValue
	}
	return value
}

func pixelLuma(src image.Image, x int, y int) uint8 {
	r, g, b, _ := src.At(x, y).RGBA()
	r8 := int(r >> 8)
	g8 := int(g >> 8)
	b8 := int(b >> 8)
	return uint8((299*r8 + 587*g8 + 114*b8) / 1000)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
