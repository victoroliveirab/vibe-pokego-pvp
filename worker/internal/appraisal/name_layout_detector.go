package appraisal

import "image"

const (
	layoutSearchTopRatio      = 0.22
	layoutSearchBottomRatio   = 0.64
	layoutBrightnessThreshold = 188

	layoutFallbackCardLeftRatio   = 0.08
	layoutFallbackCardRightRatio  = 0.92
	layoutFallbackCardTopRatio    = 0.30
	layoutFallbackCardHeightRatio = 0.63

	layoutHPTopRatio    = 0.12
	layoutHPBottomRatio = 0.45
)

// NameLayout captures deterministic appraisal geometry for species-name extraction.
type NameLayout struct {
	ImageBounds    image.Rectangle
	CardBounds     image.Rectangle
	NameBand       image.Rectangle
	HPBarY         int
	HasCard        bool
	HasHPBar       bool
	CardConfidence float64
	HPConfidence   float64
	Confidence     float64
	FailureReason  string
}

// DetectNameLayout detects appraisal card geometry and the HP bar row.
func DetectNameLayout(src image.Image) NameLayout {
	layout := NameLayout{}
	if src == nil {
		layout.FailureReason = "nil_image"
		return layout
	}

	bounds := src.Bounds()
	layout.ImageBounds = bounds
	if bounds.Empty() {
		layout.FailureReason = "empty_bounds"
		return layout
	}

	card, cardConfidence, hasCard := detectAppraisalCardBounds(src, bounds)
	layout.CardBounds = card
	layout.CardConfidence = cardConfidence
	layout.HasCard = hasCard

	hpY, hpConfidence, hasHP := detectHPBarY(src, card)
	if !hasHP {
		hpY = fallbackHPBarY(card)
	}
	layout.HPBarY = hpY
	layout.HPConfidence = hpConfidence
	layout.HasHPBar = hasHP

	layout.NameBand = deriveNameBand(bounds, card, hpY)
	layout.Confidence = clamp01(0.55*cardConfidence + 0.45*hpConfidence)

	if !hasCard && !hasHP {
		layout.FailureReason = "card_and_hp_fallback"
	} else if !hasCard {
		layout.FailureReason = "card_fallback"
	} else if !hasHP {
		layout.FailureReason = "hp_fallback"
	}

	return layout
}

func detectAppraisalCardBounds(src image.Image, bounds image.Rectangle) (image.Rectangle, float64, bool) {
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return fallbackCardBounds(bounds), 0, false
	}

	searchTop := bounds.Min.Y + int(float64(height)*layoutSearchTopRatio)
	searchBottom := bounds.Min.Y + int(float64(height)*layoutSearchBottomRatio)
	if searchTop < bounds.Min.Y {
		searchTop = bounds.Min.Y
	}
	if searchBottom > bounds.Max.Y {
		searchBottom = bounds.Max.Y
	}
	if searchTop >= searchBottom {
		return fallbackCardBounds(bounds), 0, false
	}

	xStart := bounds.Min.X + int(float64(width)*layoutFallbackCardLeftRatio)
	xEnd := bounds.Min.X + int(float64(width)*layoutFallbackCardRightRatio)
	if xStart < bounds.Min.X {
		xStart = bounds.Min.X
	}
	if xEnd > bounds.Max.X {
		xEnd = bounds.Max.X
	}
	if xStart >= xEnd {
		return fallbackCardBounds(bounds), 0, false
	}

	bestTop := -1
	bestRatio := 0.0
	for y := searchTop; y < searchBottom; y++ {
		ratio := brightRowRatio(src, y, xStart, xEnd, layoutBrightnessThreshold)
		if ratio > bestRatio {
			bestRatio = ratio
		}
		if ratio < 0.57 {
			continue
		}
		stable := 0
		for yy := y; yy < minInt(searchBottom, y+14); yy++ {
			if brightRowRatio(src, yy, xStart, xEnd, layoutBrightnessThreshold) >= 0.54 {
				stable++
			}
		}
		if stable >= 8 {
			bestTop = y
			break
		}
	}

	if bestTop < 0 {
		fallback := fallbackCardBounds(bounds)
		return fallback, clamp01(bestRatio), false
	}

	probeTop := minInt(bounds.Max.Y-1, bestTop+maxInt(2, int(0.02*float64(height))))
	probeBottom := minInt(bounds.Max.Y, bestTop+maxInt(10, int(0.18*float64(height))))
	if probeBottom <= probeTop {
		probeBottom = minInt(bounds.Max.Y, probeTop+8)
	}

	left, right, spanConfidence, ok := detectCardHorizontalSpan(src, bounds, probeTop, probeBottom)
	if !ok {
		fallback := fallbackCardBounds(bounds)
		fallback = image.Rect(fallback.Min.X, bestTop, fallback.Max.X, fallback.Max.Y).Intersect(bounds)
		if fallback.Empty() {
			fallback = fallbackCardBounds(bounds)
		}
		return fallback, clamp01((bestRatio + 0.4) / 1.4), false
	}

	bottom := bestTop + int(float64(height)*layoutFallbackCardHeightRatio)
	if bottom <= bestTop {
		bottom = bestTop + maxInt(20, int(float64(height)*0.40))
	}
	if bottom > bounds.Max.Y {
		bottom = bounds.Max.Y
	}

	card := image.Rect(left, bestTop, right, bottom).Intersect(bounds)
	if card.Empty() {
		return fallbackCardBounds(bounds), clamp01(bestRatio), false
	}

	confidence := clamp01(0.5*bestRatio + 0.5*spanConfidence)
	return card, confidence, true
}

func detectCardHorizontalSpan(src image.Image, bounds image.Rectangle, top int, bottom int) (int, int, float64, bool) {
	if top >= bottom {
		return 0, 0, 0, false
	}

	width := bounds.Dx()
	height := bottom - top
	if width <= 0 || height <= 0 {
		return 0, 0, 0, false
	}

	bestStart := -1
	bestEnd := -1
	runStart := -1
	brightCols := 0

	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		bright := 0
		for y := top; y < bottom; y++ {
			if int(pixelLuma(src, x, y)) >= layoutBrightnessThreshold {
				bright++
			}
		}
		ratio := float64(bright) / float64(height)
		if ratio >= 0.63 {
			brightCols++
			if runStart < 0 {
				runStart = x
			}
			continue
		}

		if runStart >= 0 {
			if bestStart < 0 || x-runStart > bestEnd-bestStart {
				bestStart = runStart
				bestEnd = x
			}
			runStart = -1
		}
	}

	if runStart >= 0 {
		if bestStart < 0 || bounds.Max.X-runStart > bestEnd-bestStart {
			bestStart = runStart
			bestEnd = bounds.Max.X
		}
	}

	if bestStart < 0 || bestEnd <= bestStart {
		return 0, 0, 0, false
	}

	if bestEnd-bestStart < int(float64(width)*0.54) {
		return 0, 0, 0, false
	}

	confidence := clamp01(float64(brightCols) / float64(width))
	return bestStart, bestEnd, confidence, true
}

func detectHPBarY(src image.Image, card image.Rectangle) (int, float64, bool) {
	if card.Empty() {
		return 0, 0, false
	}

	height := card.Dy()
	width := card.Dx()
	if height <= 0 || width <= 0 {
		return 0, 0, false
	}

	xStart := card.Min.X + int(float64(width)*0.16)
	xEnd := card.Max.X - int(float64(width)*0.16)
	if xEnd <= xStart {
		xStart = card.Min.X
		xEnd = card.Max.X
	}

	yStart := card.Min.Y + int(float64(height)*layoutHPTopRatio)
	yEnd := card.Min.Y + int(float64(height)*layoutHPBottomRatio)
	if yStart < card.Min.Y {
		yStart = card.Min.Y
	}
	if yEnd > card.Max.Y {
		yEnd = card.Max.Y
	}
	if yEnd <= yStart {
		return 0, 0, false
	}

	bestY := -1
	bestRatio := 0.0
	for y := yStart; y < yEnd; y++ {
		green := 0
		for x := xStart; x < xEnd; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			r8 := int(r >> 8)
			g8 := int(g >> 8)
			b8 := int(b >> 8)
			if g8 >= 95 && g8 >= r8+18 && g8 >= b8+18 && (g8-r8)+(g8-b8) >= 36 {
				green++
			}
		}

		ratio := float64(green) / float64(maxInt(1, xEnd-xStart))
		if ratio > bestRatio {
			bestRatio = ratio
			bestY = y
		}
	}

	if bestY < 0 || bestRatio < 0.06 {
		return 0, clamp01(bestRatio), false
	}

	return bestY, clamp01(bestRatio * 2.4), true
}

func deriveNameBand(bounds image.Rectangle, card image.Rectangle, hpY int) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}
	if card.Empty() {
		card = fallbackCardBounds(bounds)
	}

	imageHeight := bounds.Dy()
	if imageHeight <= 0 {
		return bounds
	}

	cardHeight := maxInt(1, card.Dy())
	left := card.Min.X + int(float64(card.Dx())*0.12)
	right := card.Max.X - int(float64(card.Dx())*0.12)
	if right <= left {
		left = card.Min.X
		right = card.Max.X
	}

	bottom := hpY - maxInt(8, int(float64(imageHeight)*0.018))
	topFloor := card.Min.Y + int(float64(cardHeight)*0.08)
	top := hpY - int(float64(imageHeight)*0.13)
	if top < topFloor {
		top = topFloor
	}
	if bottom <= top {
		top = hpY - int(float64(imageHeight)*0.11)
		bottom = hpY - maxInt(4, int(float64(imageHeight)*0.010))
	}
	if bottom <= top {
		top = card.Min.Y + int(float64(cardHeight)*0.06)
		bottom = card.Min.Y + int(float64(cardHeight)*0.26)
	}

	band := image.Rect(left, top, right, bottom).Intersect(bounds)
	if band.Empty() {
		band = image.Rect(
			bounds.Min.X+int(float64(bounds.Dx())*0.20),
			bounds.Min.Y+int(float64(bounds.Dy())*0.30),
			bounds.Min.X+int(float64(bounds.Dx())*0.80),
			bounds.Min.Y+int(float64(bounds.Dy())*0.42),
		).Intersect(bounds)
	}

	return band
}

func fallbackCardBounds(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	left := bounds.Min.X + int(float64(bounds.Dx())*layoutFallbackCardLeftRatio)
	right := bounds.Min.X + int(float64(bounds.Dx())*layoutFallbackCardRightRatio)
	top := bounds.Min.Y + int(float64(bounds.Dy())*layoutFallbackCardTopRatio)
	bottom := top + int(float64(bounds.Dy())*layoutFallbackCardHeightRatio)
	if bottom > bounds.Max.Y {
		bottom = bounds.Max.Y
	}

	fallback := image.Rect(left, top, right, bottom).Intersect(bounds)
	if fallback.Empty() {
		return bounds
	}
	return fallback
}

func fallbackHPBarY(card image.Rectangle) int {
	if card.Empty() {
		return 0
	}
	return card.Min.Y + int(float64(card.Dy())*0.27)
}

func brightRowRatio(src image.Image, y int, xStart int, xEnd int, threshold int) float64 {
	if xEnd <= xStart {
		return 0
	}

	bright := 0
	for x := xStart; x < xEnd; x++ {
		if int(pixelLuma(src, x, y)) >= threshold {
			bright++
		}
	}

	return float64(bright) / float64(xEnd-xStart)
}
