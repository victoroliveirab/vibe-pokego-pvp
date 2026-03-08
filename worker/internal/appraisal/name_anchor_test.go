package appraisal

import (
	"image"
	"image/color"
	"testing"
)

func TestDetectNameAnchorsReturnsRankedCandidates(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 640, 2400))
	fillGrayRect(img, image.Rect(0, 0, 640, 2400), 255)

	// A weaker upper band that should rank lower due to vertical position.
	fillGrayRect(img, image.Rect(40, 720, 600, 760), 0)
	// Primary HP-like band around mid-lower center target.
	fillGrayRect(img, image.Rect(80, 1120, 560, 1168), 0)
	// Lower band in search range but away from expected HP row location.
	fillGrayRect(img, image.Rect(120, 1540, 520, 1582), 0)

	anchors := DetectNameAnchors(img)
	if len(anchors) == 0 {
		t.Fatal("expected at least one anchor")
	}

	best := anchors[0]
	if best.CenterY < 1080 || best.CenterY > 1220 {
		t.Fatalf("expected top anchor center near HP band, got %d", best.CenterY)
	}

	if best.Confidence <= 0 {
		t.Fatalf("expected positive confidence, got %.4f", best.Confidence)
	}
}

func TestDetectNameAnchorsReturnsNoneForBlankImage(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 320, 640))
	fillGrayRect(img, img.Bounds(), 255)

	anchors := DetectNameAnchors(img)
	if len(anchors) != 0 {
		t.Fatalf("expected no anchors for blank image, got %d", len(anchors))
	}
}

func TestEvaluateAndBlendNameAnchorOCREvidence(t *testing.T) {
	evidence := EvaluateNameAnchorOCREvidence("141 / 141 HP")
	if !evidence.HasHPToken {
		t.Fatal("expected HP token evidence")
	}
	if !evidence.HasHPValuePattern {
		t.Fatal("expected HP numeric pattern evidence")
	}
	if evidence.OCRScore <= 0.8 {
		t.Fatalf("expected strong OCR score, got %.4f", evidence.OCRScore)
	}

	anchor := NameAnchor{
		GeometryScore: 0.42,
		Confidence:    0.42,
	}
	blended := BlendNameAnchorConfidence(anchor, evidence)
	if blended.Confidence <= anchor.Confidence {
		t.Fatalf("expected blended confidence %.4f to exceed base %.4f", blended.Confidence, anchor.Confidence)
	}
	if blended.Confidence > 1 {
		t.Fatalf("expected confidence <= 1, got %.4f", blended.Confidence)
	}
}

func TestNameAnchorProbeRegionUsesBoundedTightWindow(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	anchor := NameAnchor{
		Band:    image.Rect(0, 2430, 2412, 2460),
		CenterY: 2445,
	}

	probe := NameAnchorProbeRegion(bounds, anchor)
	if probe.Empty() {
		t.Fatal("expected non-empty probe region")
	}

	if probe.Dx() >= int(float64(bounds.Dx())*0.5) {
		t.Fatalf("expected tighter probe width, got %d for image width %d", probe.Dx(), bounds.Dx())
	}
	if probe.Dy() > int(float64(bounds.Dy())*anchorProbeMaxHeightRatio)+1 {
		t.Fatalf("expected bounded probe height, got %d", probe.Dy())
	}
	if probe.Min.Y < bounds.Min.Y || probe.Max.Y > bounds.Max.Y {
		t.Fatalf("expected probe to stay within bounds, got %v outside %v", probe, bounds)
	}
}

func TestNameAnchorProbeRegionsIncludesDownwardCandidate(t *testing.T) {
	bounds := image.Rect(0, 0, 2412, 5244)
	anchor := NameAnchor{
		Band:    image.Rect(0, 2366, 2412, 2392),
		CenterY: 2379,
	}

	regions := NameAnchorProbeRegions(bounds, anchor)
	if len(regions) < 2 {
		t.Fatalf("expected multiple probe candidates, got %d", len(regions))
	}

	base := NameAnchorProbeRegion(bounds, anchor)
	foundDownward := false
	for _, candidate := range regions {
		if candidate.Min.Y > base.Min.Y {
			foundDownward = true
			break
		}
	}

	if !foundDownward {
		t.Fatalf("expected at least one downward shifted probe candidate from base %v, got %#v", base, regions)
	}
}

func fillGrayRect(img *image.Gray, rect image.Rectangle, value uint8) {
	clipped := rect.Intersect(img.Bounds())
	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		for x := clipped.Min.X; x < clipped.Max.X; x++ {
			img.SetGray(x, y, color.Gray{Y: value})
		}
	}
}
