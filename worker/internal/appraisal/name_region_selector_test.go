package appraisal

import (
	"image"
	"testing"
)

func TestBuildSpeciesNameRegionsUsesAnchorRelativeWindows(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	anchors := []NameAnchor{
		{
			Band:       image.Rect(0, 2366, 2412, 2392),
			CenterY:    2379,
			Confidence: 0.95,
		},
	}

	regions := BuildSpeciesNameRegions(bounds, anchors)
	if len(regions) == 0 {
		t.Fatal("expected at least one name region")
	}

	// Name windows should be above the anchor center for normal confidence anchors.
	aboveAnchorCount := 0
	for _, region := range regions {
		if region.Max.Y <= anchors[0].CenterY {
			aboveAnchorCount++
		}
		if region.Min.X < bounds.Min.X || region.Max.X > bounds.Max.X {
			t.Fatalf("region %v is outside horizontal bounds %v", region, bounds)
		}
	}

	if aboveAnchorCount == 0 {
		t.Fatalf("expected at least one region above anchor center %d, got %#v", anchors[0].CenterY, regions)
	}
}

func TestBuildSpeciesNameRegionsAddsGuardedFallbackForLowConfidenceAnchor(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	anchors := []NameAnchor{
		{
			Band:       image.Rect(0, 2366, 2412, 2392),
			CenterY:    2379,
			Confidence: 0.30,
		},
	}

	regions := BuildSpeciesNameRegions(bounds, anchors)
	if len(regions) < 2 {
		t.Fatalf("expected multiple regions including guarded fallback, got %d", len(regions))
	}

	hasWideFallback := false
	for _, region := range regions {
		if region.Dx() >= int(float64(bounds.Dx())*0.70) {
			hasWideFallback = true
			break
		}
	}

	if !hasWideFallback {
		t.Fatalf("expected a guarded wide fallback region, got %#v", regions)
	}
}

func TestBuildSpeciesNameRegionsAddsHigherCoverageForVeryLowConfidenceAnchor(t *testing.T) {
	bounds := image.Rect(0, 0, 1176, 2560)
	anchors := []NameAnchor{
		{
			Band:       image.Rect(0, 1287, 1176, 1347),
			CenterY:    1317,
			Confidence: 0.36,
		},
	}

	regions := BuildSpeciesNameRegions(bounds, anchors)
	if len(regions) == 0 {
		t.Fatal("expected regions for low-confidence anchor")
	}

	minTop := bounds.Max.Y
	for _, region := range regions {
		if region.Min.Y < minTop {
			minTop = region.Min.Y
		}
	}

	// Deerling-like fixtures need coverage noticeably above the HP row.
	if minTop > 1085 {
		t.Fatalf("expected at least one higher-up region for low-confidence anchor, got min top %d", minTop)
	}
}

func TestBuildSpeciesNameRegionsReturnsGlobalFallbackWhenNoAnchors(t *testing.T) {
	bounds := image.Rect(0, 0, 1200, 2600)
	regions := BuildSpeciesNameRegions(bounds, nil)
	if len(regions) != 1 {
		t.Fatalf("expected one global fallback region, got %d", len(regions))
	}

	region := regions[0]
	if region.Empty() {
		t.Fatal("expected non-empty global fallback region")
	}
	if region.Dx() <= 0 || region.Dy() <= 0 {
		t.Fatalf("expected positive fallback dimensions, got %v", region)
	}
}
