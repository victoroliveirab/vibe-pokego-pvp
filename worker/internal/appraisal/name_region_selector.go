package appraisal

import "image"

const (
	nameRegionPrimaryWidthRatio = 0.62

	nameRegionHeightSmallRatio = 0.037
	nameRegionHeightLargeRatio = 0.046

	nameRegionOffsetPrimaryRatio    = 0.034
	nameRegionOffsetSecondaryRatio  = 0.046
	nameRegionOffsetTertiaryRatio   = 0.026
	nameRegionOffsetQuaternaryRatio = 0.058

	nameRegionMinHeightRatio = 0.024
	nameRegionMaxHeightRatio = 0.070

	nameRegionMaxAnchors                = 2
	nameRegionLowConfidenceCutoff       = 0.72
	nameRegionVeryLowConfidenceCutoff   = 0.50
	nameRegionGuardedFallbackWidth      = 0.78
	nameRegionGuardedFallbackHeight     = 0.100
	nameRegionGuardedFallbackOffset     = 0.026
	nameRegionGuardedFallbackHighOffset = 0.085

	nameRegionGlobalFallbackLeftRatio   = 0.20
	nameRegionGlobalFallbackRightRatio  = 0.80
	nameRegionGlobalFallbackTopRatio    = 0.33
	nameRegionGlobalFallbackBottomRatio = 0.50
)

// BuildSpeciesNameRegions returns prioritized OCR regions for species-name extraction.
// Regions are derived from HP-anchor candidates and constrained to stay tight.
func BuildSpeciesNameRegions(bounds image.Rectangle, anchors []NameAnchor) []image.Rectangle {
	if bounds.Empty() {
		return nil
	}

	var regions []image.Rectangle

	width := bounds.Dx()
	height := bounds.Dy()

	anchorCount := minInt(len(anchors), nameRegionMaxAnchors)
	for idx := 0; idx < anchorCount; idx++ {
		anchor := anchors[idx]
		regions = append(regions, anchorNameRegions(bounds, anchor, width, height)...)
	}

	needsGuardedFallback := anchorCount == 0
	if anchorCount > 0 && anchors[0].Confidence < nameRegionLowConfidenceCutoff {
		needsGuardedFallback = true
	}
	if needsGuardedFallback {
		if anchorCount > 0 {
			regions = append(regions, guardedAnchorFallbackRegion(bounds, anchors[0], width, height))
			if anchors[0].Confidence < nameRegionVeryLowConfidenceCutoff {
				regions = append(regions, guardedAnchorHighFallbackRegion(bounds, anchors[0], width, height))
			}
		} else {
			regions = append(regions, globalFallbackRegion(bounds, width, height))
		}
	}

	return dedupeNonEmptyRegions(bounds, regions)
}

func anchorNameRegions(bounds image.Rectangle, anchor NameAnchor, width int, height int) []image.Rectangle {
	regionWidth := maxInt(1, int(float64(width)*nameRegionPrimaryWidthRatio))
	heightCandidates := []int{
		boundedRegionHeight(height, anchor.Band.Dy(), nameRegionHeightSmallRatio),
		boundedRegionHeight(height, anchor.Band.Dy(), nameRegionHeightLargeRatio),
	}
	offsetRatios := []float64{
		nameRegionOffsetPrimaryRatio,
		nameRegionOffsetSecondaryRatio,
		nameRegionOffsetTertiaryRatio,
		nameRegionOffsetQuaternaryRatio,
	}
	if anchor.Confidence < nameRegionVeryLowConfidenceCutoff {
		offsetRatios = append(offsetRatios, 0.075, 0.090)
	}

	regions := make([]image.Rectangle, 0, len(heightCandidates)*len(offsetRatios))
	left := bounds.Min.X + (width-regionWidth)/2
	for _, regionHeight := range heightCandidates {
		for _, offsetRatio := range offsetRatios {
			centerY := anchor.CenterY - int(float64(height)*offsetRatio)
			top := centerY - regionHeight/2
			region := image.Rect(left, top, left+regionWidth, top+regionHeight).Intersect(bounds)
			regions = append(regions, region)
		}
	}

	return regions
}

func guardedAnchorFallbackRegion(bounds image.Rectangle, anchor NameAnchor, width int, height int) image.Rectangle {
	regionWidth := maxInt(1, int(float64(width)*nameRegionGuardedFallbackWidth))
	regionHeight := maxInt(
		int(float64(height)*nameRegionGuardedFallbackHeight),
		boundedRegionHeight(height, anchor.Band.Dy(), nameRegionHeightLargeRatio),
	)
	centerY := anchor.CenterY - int(float64(height)*nameRegionGuardedFallbackOffset)

	left := bounds.Min.X + (width-regionWidth)/2
	top := centerY - regionHeight/2
	return image.Rect(left, top, left+regionWidth, top+regionHeight).Intersect(bounds)
}

func guardedAnchorHighFallbackRegion(bounds image.Rectangle, anchor NameAnchor, width int, height int) image.Rectangle {
	regionWidth := maxInt(1, int(float64(width)*nameRegionGuardedFallbackWidth))
	regionHeight := maxInt(
		int(float64(height)*nameRegionGuardedFallbackHeight),
		boundedRegionHeight(height, anchor.Band.Dy(), nameRegionHeightLargeRatio),
	)
	centerY := anchor.CenterY - int(float64(height)*nameRegionGuardedFallbackHighOffset)

	left := bounds.Min.X + (width-regionWidth)/2
	top := centerY - regionHeight/2
	return image.Rect(left, top, left+regionWidth, top+regionHeight).Intersect(bounds)
}

func globalFallbackRegion(bounds image.Rectangle, width int, height int) image.Rectangle {
	left := bounds.Min.X + int(float64(width)*nameRegionGlobalFallbackLeftRatio)
	right := bounds.Min.X + int(float64(width)*nameRegionGlobalFallbackRightRatio)
	top := bounds.Min.Y + int(float64(height)*nameRegionGlobalFallbackTopRatio)
	bottom := bounds.Min.Y + int(float64(height)*nameRegionGlobalFallbackBottomRatio)
	return image.Rect(left, top, right, bottom).Intersect(bounds)
}

func boundedRegionHeight(imageHeight int, anchorBandHeight int, targetRatio float64) int {
	target := int(float64(imageHeight) * targetRatio)
	minHeight := maxInt(1, int(float64(imageHeight)*nameRegionMinHeightRatio))
	maxHeight := maxInt(minHeight, int(float64(imageHeight)*nameRegionMaxHeightRatio))
	anchorScaled := maxInt(anchorBandHeight*7, minHeight)
	return minInt(maxInt(target, anchorScaled), maxHeight)
}

func dedupeNonEmptyRegions(bounds image.Rectangle, regions []image.Rectangle) []image.Rectangle {
	if len(regions) == 0 {
		return nil
	}

	seen := make(map[image.Rectangle]struct{}, len(regions))
	result := make([]image.Rectangle, 0, len(regions))
	for _, region := range regions {
		clipped := region.Intersect(bounds)
		if clipped.Empty() {
			continue
		}
		if _, ok := seen[clipped]; ok {
			continue
		}
		seen[clipped] = struct{}{}
		result = append(result, clipped)
	}
	return result
}
