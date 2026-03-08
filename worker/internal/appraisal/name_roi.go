package appraisal

import "image"

// NameROI represents deterministic text-localized ROI candidates.
type NameROI struct {
	Band          image.Rectangle
	Region        image.Rectangle
	Method        string
	Confidence    float64
	FailureReason string
}

// DetectSpeciesNameROI locates a tight name-text ROI within the layout band.
func DetectSpeciesNameROI(preprocessed image.Image, layout NameLayout) NameROI {
	result := NameROI{Method: "projection"}
	if preprocessed == nil {
		result.FailureReason = "nil_image"
		return result
	}

	bounds := preprocessed.Bounds()
	if bounds.Empty() {
		result.FailureReason = "empty_bounds"
		return result
	}

	band := layout.NameBand.Intersect(bounds)
	if band.Empty() {
		card := layout.CardBounds.Intersect(bounds)
		if card.Empty() {
			card = fallbackCardBounds(bounds)
		}
		fallbackHP := layout.HPBarY
		if fallbackHP <= card.Min.Y || fallbackHP >= card.Max.Y {
			fallbackHP = fallbackHPBarY(card)
		}
		band = deriveNameBand(bounds, card, fallbackHP)
		result.FailureReason = "band_fallback"
	}

	if band.Empty() {
		result.Band = fallbackGlobalNameBand(bounds)
		result.Region = result.Band
		result.Confidence = 0
		result.FailureReason = "global_fallback"
		return result
	}

	result.Band = band

	innerLeft := band.Min.X + int(float64(band.Dx())*0.08)
	innerRight := band.Max.X - int(float64(band.Dx())*0.08)
	if innerRight <= innerLeft {
		innerLeft = band.Min.X
		innerRight = band.Max.X
	}

	rows := band.Dy()
	rowInk := make([]int, rows)
	rowTransitions := make([]int, rows)
	bestScore := 0.0
	bestRow := -1
	bestInk := 0

	for y := band.Min.Y; y < band.Max.Y; y++ {
		idx := y - band.Min.Y
		ink := 0
		transitions := 0
		lastDark := false
		hasLast := false

		for x := innerLeft; x < innerRight; x++ {
			dark := isDarkPixel(preprocessed, x, y)
			if dark {
				ink++
			}
			if hasLast && dark != lastDark {
				transitions++
			}
			lastDark = dark
			hasLast = true
		}

		rowInk[idx] = ink
		rowTransitions[idx] = transitions
		transitionFactor := clamp01(float64(transitions) / 14.0)
		score := float64(ink) * transitionFactor
		if score > bestScore {
			bestScore = score
			bestRow = y
			bestInk = ink
		}
	}

	if bestRow < 0 || bestScore < 10 || bestInk < maxInt(4, (innerRight-innerLeft)/40) {
		result.Region = band
		result.Confidence = clamp01(bestScore / 120.0)
		if result.FailureReason == "" {
			result.FailureReason = "weak_projection_signal"
		}
		return result
	}

	rowThreshold := maxInt(2, bestInk/8)
	top := bestRow
	for y := bestRow - 1; y >= band.Min.Y; y-- {
		if rowInk[y-band.Min.Y] < rowThreshold {
			break
		}
		top = y
	}

	bottom := bestRow + 1
	for y := bestRow + 1; y < band.Max.Y; y++ {
		if rowInk[y-band.Min.Y] < rowThreshold {
			break
		}
		bottom = y + 1
	}

	if bottom <= top {
		result.Region = band
		result.Confidence = clamp01(bestScore / 120.0)
		if result.FailureReason == "" {
			result.FailureReason = "collapsed_vertical_roi"
		}
		return result
	}

	colThreshold := maxInt(2, (bottom-top)/6)
	left := band.Max.X
	right := band.Min.X
	for x := innerLeft; x < innerRight; x++ {
		ink := 0
		for y := top; y < bottom; y++ {
			if isDarkPixel(preprocessed, x, y) {
				ink++
			}
		}
		if ink >= colThreshold {
			if x < left {
				left = x
			}
			if x+1 > right {
				right = x + 1
			}
		}
	}

	if right <= left {
		left = innerLeft
		right = innerRight
		if result.FailureReason == "" {
			result.FailureReason = "collapsed_horizontal_roi"
		}
	}

	padX := maxInt(2, int(float64(bounds.Dx())*0.015))
	padY := maxInt(2, int(float64(bounds.Dy())*0.008))
	roi := image.Rect(left-padX, top-padY, right+padX, bottom+padY).Intersect(bounds)
	if roi.Empty() {
		roi = band
		if result.FailureReason == "" {
			result.FailureReason = "empty_roi_fallback"
		}
	}

	result.Region = roi
	result.Confidence = clamp01(bestScore / 140.0)
	return result
}

func isDarkPixel(src image.Image, x int, y int) bool {
	return pixelLuma(src, x, y) < 128
}

func fallbackGlobalNameBand(bounds image.Rectangle) image.Rectangle {
	if bounds.Empty() {
		return bounds
	}

	left := bounds.Min.X + int(float64(bounds.Dx())*0.20)
	right := bounds.Min.X + int(float64(bounds.Dx())*0.80)
	top := bounds.Min.Y + int(float64(bounds.Dy())*0.30)
	bottom := bounds.Min.Y + int(float64(bounds.Dy())*0.42)

	if right <= left {
		left = bounds.Min.X
		right = minInt(bounds.Max.X, left+maxInt(1, bounds.Dx()))
	}
	if bottom <= top {
		top = bounds.Min.Y
		bottom = minInt(bounds.Max.Y, top+maxInt(1, bounds.Dy()))
	}

	rect := image.Rect(left, top, right, bottom).Intersect(bounds)
	if rect.Empty() {
		return bounds
	}
	return rect
}
