package worker

import (
	"testing"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
)

func TestDeduplicateCanonicalAcceptedReadingsCollapsesDuplicatesByIdentity(t *testing.T) {
	timestamp900 := int64(900)
	timestamp300 := int64(300)
	timestamp600 := int64(600)

	readings := []canonicalAcceptedReading{
		{
			Accepted:         makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
			FrameTimestampMS: &timestamp900,
		},
		{
			Accepted:         makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
			FrameTimestampMS: &timestamp300,
		},
		{
			Accepted:         makeAcceptedResultCandidate("Munna", 824, 141, 0, 13, 13),
			FrameTimestampMS: &timestamp600,
		},
	}

	deduped := deduplicateCanonicalAcceptedReadings(readings)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated readings, got %d", len(deduped))
	}

	if deduped[0].FrameTimestampMS == nil || *deduped[0].FrameTimestampMS != 300 {
		t.Fatalf("expected earliest duplicate timestamp 300 to be kept, got %#v", deduped[0].FrameTimestampMS)
	}
	if deduped[0].Accepted.SpeciesName != "Darumaka" {
		t.Fatalf("expected first deduped species Darumaka, got %q", deduped[0].Accepted.SpeciesName)
	}

	if deduped[1].FrameTimestampMS == nil || *deduped[1].FrameTimestampMS != 600 {
		t.Fatalf("expected second deduped timestamp 600, got %#v", deduped[1].FrameTimestampMS)
	}
	if deduped[1].Accepted.SpeciesName != "Munna" {
		t.Fatalf("expected second deduped species Munna, got %q", deduped[1].Accepted.SpeciesName)
	}
}

func TestDeduplicateCanonicalAcceptedReadingsKeepsDistinctStats(t *testing.T) {
	timestamp0 := int64(0)
	timestamp300 := int64(300)

	readings := []canonicalAcceptedReading{
		{
			Accepted:         makeAcceptedResultCandidate("Darumaka", 712, 120, 10, 11, 12),
			FrameTimestampMS: &timestamp0,
		},
		{
			Accepted:         makeAcceptedResultCandidate("Darumaka", 713, 120, 10, 11, 12),
			FrameTimestampMS: &timestamp300,
		},
	}

	deduped := deduplicateCanonicalAcceptedReadings(readings)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated readings for distinct stats, got %d", len(deduped))
	}
}

func TestReduceAcceptedVideoReadingsCollapsesAdjacentFrameIVDrift(t *testing.T) {
	timestamp21300 := int64(21300)
	timestamp21600 := int64(21600)
	timestamp24000 := int64(24000)

	readings := []canonicalAcceptedReading{
		{
			Accepted:         makeAcceptedResultCandidate("Feebas", 12, 21, 0, 10, 8),
			FrameTimestampMS: &timestamp21300,
		},
		{
			Accepted:         makeAcceptedResultCandidate("Feebas", 12, 21, 0, 7, 14),
			FrameTimestampMS: &timestamp21600,
		},
		{
			Accepted:         makeAcceptedResultCandidate("Feebas", 12, 21, 0, 10, 8),
			FrameTimestampMS: &timestamp24000,
		},
	}

	reduced := reduceAcceptedVideoReadings(readings)
	if len(reduced) != 1 {
		t.Fatalf("expected 1 reading after strict+adjacent-frame reduction, got %d", len(reduced))
	}

	if reduced[0].FrameTimestampMS == nil || *reduced[0].FrameTimestampMS != 21300 {
		t.Fatalf("expected first retained timestamp 21300, got %#v", reduced[0].FrameTimestampMS)
	}
}

func TestDeduplicatePendingVideoReadingsCollapsesRepeatedFrameAmbiguity(t *testing.T) {
	timestamp5100 := int64(5100)
	timestamp5400 := int64(5400)

	options := []appraisal.AcceptedResultCandidate{
		makeAcceptedResultCandidate("Darumaka", 980, 124, 7, 15, 15),
		makeAcceptedResultCandidate("Darumaka (Galarian)", 980, 124, 7, 15, 15),
	}

	readings := []pendingVideoReading{
		{AcceptedOptions: append([]appraisal.AcceptedResultCandidate(nil), options...), FrameTimestampMS: &timestamp5100},
		{AcceptedOptions: append([]appraisal.AcceptedResultCandidate(nil), options...), FrameTimestampMS: &timestamp5400},
	}

	deduped := deduplicatePendingVideoReadings(readings)
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated pending reading, got %d", len(deduped))
	}

	if deduped[0].FrameTimestampMS == nil || *deduped[0].FrameTimestampMS != 5100 {
		t.Fatalf("expected earliest pending timestamp 5100, got %#v", deduped[0].FrameTimestampMS)
	}
}

func makeAcceptedResultCandidate(speciesName string, cp int, hp int, ivAttack int, ivDefense int, ivStamina int) appraisal.AcceptedResultCandidate {
	return appraisal.AcceptedResultCandidate{
		SpeciesName:           speciesName,
		SpeciesNameNormalized: speciesName,
		CP:                    cp,
		HP:                    hp,
		IVAttack:              ivAttack,
		IVDefense:             ivDefense,
		IVStamina:             ivStamina,
		LevelMethod:           appraisal.LevelMethodUnknown,
		MatchMode:             "exact",
		MatchDistance:         0,
	}
}
