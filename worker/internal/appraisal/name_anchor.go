package appraisal

import (
	"image"
	"math"
	"regexp"
	"sort"
	"strings"
)

const (
	anchorSearchTopRatio    = 0.30
	anchorSearchBottomRatio = 0.64
	anchorSearchLeftRatio   = 0.24
	anchorSearchRightRatio  = 0.78

	anchorTargetCenterYRatio = 0.47
	anchorPositionTolerance  = 0.18

	anchorDefaultThreshold = 0.012
	anchorThresholdOffset  = 0.007

	anchorMinHeightRatio = 0.002
	anchorMaxHeightRatio = 0.03

	anchorProbeWidthRatio           = 0.36
	anchorProbeMinHeightRatio       = 0.016
	anchorProbeMaxHeightRatio       = 0.045
	anchorProbeBandHeightMultiplier = 4
	anchorProbeOffsetPrimaryRatio   = 0.50
	anchorProbeOffsetSecondaryRatio = 1.00
	anchorProbeOffsetUpwardRatio    = -0.50

	anchorMaxCandidates = 6
	anchorSmoothRadius  = 4
)

var hpValuePattern = regexp.MustCompile(`(?i)\d+\s*/\s*\d+\s*hp\b`)

// NameAnchor represents a potential HP-row anchor used for name-region localization.
type NameAnchor struct {
	Band              image.Rectangle
	CenterY           int
	GeometryScore     float64
	OCRScore          float64
	Confidence        float64
	HasHPToken        bool
	HasHPValuePattern bool
}

// NameAnchorOCREvidence captures OCR-derived HP evidence for an anchor candidate.
type NameAnchorOCREvidence struct {
	HasHPToken        bool
	HasHPValuePattern bool
	OCRScore          float64
}

// DetectNameAnchors locates and ranks likely HP-row anchors from a preprocessed image.
func DetectNameAnchors(src image.Image) []NameAnchor {
	if src == nil {
		return nil
	}

	bounds := src.Bounds()
	if bounds.Empty() {
		return nil
	}

	rows := rowDarkRatios(src, bounds)
	if len(rows) == 0 {
		return nil
	}

	smooth := smoothSeries(rows, anchorSmoothRadius)
	if len(smooth) == 0 {
		return nil
	}

	threshold := anchorThreshold(smooth)
	anchors := bandAnchorsFromSeries(bounds, smooth, threshold)
	if len(anchors) == 0 {
		return nil
	}

	sort.SliceStable(anchors, func(i, j int) bool {
		if anchors[i].Confidence == anchors[j].Confidence {
			return anchors[i].CenterY < anchors[j].CenterY
		}
		return anchors[i].Confidence > anchors[j].Confidence
	})

	if len(anchors) > anchorMaxCandidates {
		return anchors[:anchorMaxCandidates]
	}

	return anchors
}

// NameAnchorProbeRegion returns a narrow OCR region centered around an anchor candidate.
func NameAnchorProbeRegion(bounds image.Rectangle, anchor NameAnchor) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	width := bounds.Dx()
	height := bounds.Dy()

	probeWidth := maxInt(1, int(float64(width)*anchorProbeWidthRatio))
	probeHeight := maxInt(
		anchor.Band.Dy()*anchorProbeBandHeightMultiplier,
		int(float64(height)*anchorProbeMinHeightRatio),
	)
	probeHeight = minInt(probeHeight, int(float64(height)*anchorProbeMaxHeightRatio))
	probeHeight = maxInt(1, probeHeight)

	left := bounds.Min.X + (width-probeWidth)/2
	top := anchor.CenterY - probeHeight/2
	region := image.Rect(left, top, left+probeWidth, top+probeHeight).Intersect(bounds)
	if region.Empty() {
		return bounds
	}

	return region
}

// NameAnchorProbeRegions returns candidate probe regions around an anchor.
// The first candidate is biased downward to better capture the HP text row
// when the geometric anchor lands on the HP bar line.
func NameAnchorProbeRegions(bounds image.Rectangle, anchor NameAnchor) []image.Rectangle {
	base := NameAnchorProbeRegion(bounds, anchor)
	if base.Empty() {
		return nil
	}

	height := base.Dy()
	if height <= 0 {
		return []image.Rectangle{base}
	}

	offsets := []int{
		int(float64(height) * anchorProbeOffsetPrimaryRatio),
		0,
		int(float64(height) * anchorProbeOffsetSecondaryRatio),
		int(float64(height) * anchorProbeOffsetUpwardRatio),
	}

	candidates := make([]image.Rectangle, 0, len(offsets))
	for _, offset := range offsets {
		shifted := shiftRegionY(base, bounds, offset)
		if shifted.Empty() {
			continue
		}
		duplicate := false
		for _, existing := range candidates {
			if existing == shifted {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, shifted)
		}
	}

	if len(candidates) == 0 {
		return []image.Rectangle{base}
	}

	return candidates
}

// EvaluateNameAnchorOCREvidence checks OCR text for HP-row evidence.
func EvaluateNameAnchorOCREvidence(rawText string) NameAnchorOCREvidence {
	text := strings.TrimSpace(rawText)
	if text == "" {
		return NameAnchorOCREvidence{}
	}

	upper := strings.ToUpper(text)
	hasHPToken := strings.Contains(upper, "HP")
	hasPattern := hpValuePattern.MatchString(text)

	score := 0.0
	if hasHPToken {
		score = 0.45
	}
	if hasPattern {
		score = 0.85
	}

	return NameAnchorOCREvidence{
		HasHPToken:        hasHPToken,
		HasHPValuePattern: hasPattern,
		OCRScore:          score,
	}
}

// BlendNameAnchorConfidence combines geometry score with OCR-derived evidence.
func BlendNameAnchorConfidence(anchor NameAnchor, evidence NameAnchorOCREvidence) NameAnchor {
	confidence := 0.55*clamp01(anchor.GeometryScore) + 0.45*clamp01(evidence.OCRScore)
	if !evidence.HasHPToken {
		confidence *= 0.75
	}
	if evidence.HasHPToken {
		confidence += 0.10
	}
	if evidence.HasHPValuePattern {
		confidence += 0.20
	}

	anchor.HasHPToken = evidence.HasHPToken
	anchor.HasHPValuePattern = evidence.HasHPValuePattern
	anchor.OCRScore = evidence.OCRScore
	anchor.Confidence = clamp01(confidence)
	return anchor
}

func rowDarkRatios(src image.Image, bounds image.Rectangle) []float64 {
	height := bounds.Dy()
	if height <= 0 {
		return nil
	}
	width := bounds.Dx()
	if width <= 0 {
		return nil
	}

	startX := bounds.Min.X + int(float64(width)*anchorSearchLeftRatio)
	endX := bounds.Min.X + int(float64(width)*anchorSearchRightRatio)
	if startX < bounds.Min.X {
		startX = bounds.Min.X
	}
	if endX > bounds.Max.X {
		endX = bounds.Max.X
	}
	if startX >= endX {
		return nil
	}

	searchWidth := endX - startX

	startY := bounds.Min.Y + int(float64(height)*anchorSearchTopRatio)
	endY := bounds.Min.Y + int(float64(height)*anchorSearchBottomRatio)
	if startY < bounds.Min.Y {
		startY = bounds.Min.Y
	}
	if endY > bounds.Max.Y {
		endY = bounds.Max.Y
	}
	if startY >= endY {
		return nil
	}

	darkRows := make([]float64, 0, endY-startY)
	lightRows := make([]float64, 0, endY-startY)
	totalDark := 0
	totalPixels := searchWidth * (endY - startY)

	for y := startY; y < endY; y++ {
		dark := 0
		for x := startX; x < endX; x++ {
			if pixelLuma(src, x, y) < 128 {
				dark++
			}
		}
		totalDark += dark
		darkRatio := float64(dark) / float64(searchWidth)
		darkRows = append(darkRows, darkRatio)
		lightRows = append(lightRows, 1.0-darkRatio)
	}

	if totalPixels <= 0 {
		return nil
	}

	// Pick the minority polarity as foreground so this works for both
	// dark-on-light and light-on-dark preprocessed screenshots.
	globalDarkRatio := float64(totalDark) / float64(totalPixels)
	if globalDarkRatio >= 0.5 {
		return lightRows
	}

	return darkRows
}

func smoothSeries(values []float64, radius int) []float64 {
	if len(values) == 0 {
		return nil
	}
	if radius <= 0 {
		out := make([]float64, len(values))
		copy(out, values)
		return out
	}

	out := make([]float64, len(values))
	for i := range values {
		start := maxInt(0, i-radius)
		end := minInt(len(values)-1, i+radius)
		sum := 0.0
		count := 0
		for idx := start; idx <= end; idx++ {
			sum += values[idx]
			count++
		}
		if count == 0 {
			continue
		}
		out[i] = sum / float64(count)
	}
	return out
}

func anchorThreshold(values []float64) float64 {
	if len(values) == 0 {
		return anchorDefaultThreshold
	}

	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))

	threshold := mean + anchorThresholdOffset
	if threshold < anchorDefaultThreshold {
		threshold = anchorDefaultThreshold
	}
	return threshold
}

func bandAnchorsFromSeries(bounds image.Rectangle, smooth []float64, threshold float64) []NameAnchor {
	searchTop := bounds.Min.Y + int(float64(bounds.Dy())*anchorSearchTopRatio)

	minBandHeight := maxInt(1, int(float64(bounds.Dy())*anchorMinHeightRatio))
	maxBandHeight := maxInt(minBandHeight, int(float64(bounds.Dy())*anchorMaxHeightRatio))

	var anchors []NameAnchor

	activeStart := -1
	peak := 0.0
	lastAbove := -1

	flush := func(endIndex int) {
		if activeStart < 0 {
			return
		}
		if endIndex < activeStart {
			activeStart = -1
			peak = 0
			lastAbove = -1
			return
		}

		bandHeight := endIndex - activeStart + 1
		if bandHeight < minBandHeight || bandHeight > maxBandHeight {
			activeStart = -1
			peak = 0
			lastAbove = -1
			return
		}

		startY := searchTop + activeStart
		endY := searchTop + endIndex + 1
		centerY := startY + (endY-startY)/2
		geometry := geometryScore(bounds, centerY, bandHeight, peak)

		anchor := NameAnchor{
			Band:          image.Rect(bounds.Min.X, startY, bounds.Max.X, endY),
			CenterY:       centerY,
			GeometryScore: geometry,
			Confidence:    geometry,
		}
		anchors = append(anchors, anchor)

		activeStart = -1
		peak = 0
		lastAbove = -1
	}

	for i, value := range smooth {
		if value >= threshold {
			if activeStart < 0 {
				activeStart = i
				peak = value
			}
			if value > peak {
				peak = value
			}
			lastAbove = i
			continue
		}

		if activeStart >= 0 && lastAbove >= 0 && i-lastAbove <= anchorSmoothRadius {
			continue
		}

		flush(i - 1)
	}

	flush(len(smooth) - 1)

	return anchors
}

func geometryScore(bounds image.Rectangle, centerY int, bandHeight int, peakSignalRatio float64) float64 {
	height := float64(maxInt(1, bounds.Dy()))
	normalizedY := float64(centerY-bounds.Min.Y) / height
	positionDelta := math.Abs(normalizedY-anchorTargetCenterYRatio) / anchorPositionTolerance
	positionScore := clamp01(1.0 - positionDelta)

	signalScore := clamp01((peakSignalRatio - 0.006) / 0.09)

	normalizedBandHeight := float64(bandHeight) / height
	heightScore := clamp01(1.0 - math.Abs(normalizedBandHeight-0.02)/0.04)

	return clamp01(0.50*signalScore + 0.35*positionScore + 0.15*heightScore)
}

func pixelLuma(src image.Image, x int, y int) uint8 {
	switch typed := src.(type) {
	case *image.Gray:
		return typed.GrayAt(x, y).Y
	case *image.RGBA:
		c := typed.RGBAAt(x, y)
		return uint8((299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000)
	default:
		r, g, b, _ := src.At(x, y).RGBA()
		r8 := int(r >> 8)
		g8 := int(g >> 8)
		b8 := int(b >> 8)
		return uint8((299*r8 + 587*g8 + 114*b8) / 1000)
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func shiftRegionY(region image.Rectangle, bounds image.Rectangle, offsetY int) image.Rectangle {
	if region.Empty() || bounds.Empty() {
		return image.Rectangle{}
	}

	height := region.Dy()
	if height <= 0 {
		return image.Rectangle{}
	}
	if height >= bounds.Dy() {
		return bounds
	}

	top := region.Min.Y + offsetY
	if top < bounds.Min.Y {
		top = bounds.Min.Y
	}
	if top+height > bounds.Max.Y {
		top = bounds.Max.Y - height
	}

	return image.Rect(region.Min.X, top, region.Max.X, top+height).Intersect(bounds)
}

func minInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}

	result := values[0]
	for idx := 1; idx < len(values); idx++ {
		if values[idx] < result {
			result = values[idx]
		}
	}

	return result
}

func maxInt(values ...int) int {
	if len(values) == 0 {
		return 0
	}

	result := values[0]
	for idx := 1; idx < len(values); idx++ {
		if values[idx] > result {
			result = values[idx]
		}
	}

	return result
}
