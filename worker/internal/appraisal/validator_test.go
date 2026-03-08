package appraisal

import (
	"fmt"
	"math"
	"testing"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
)

func TestValidateCandidateAcceptsCanonicalConsistentTupleAndInfersLevel(t *testing.T) {
	t.Parallel()

	catalog := mustBuildValidationCatalog(t)
	entry, ok := catalog.EntryForNormalized("munna")
	if !ok {
		t.Fatal("expected munna entry in catalog")
	}

	expectedLevel := 20.0
	cp, hp, ok := catalog.ComputeCPHP(entry, expectedLevel, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation to succeed")
	}

	parsed := ParseCandidateFromOCR("Munna")
	parsed.CPRaw = stringPtr(fmt.Sprintf("%d", cp))
	parsed.HPRaw = stringPtr(fmt.Sprintf("%d", hp))
	parsed.IVAttackRaw = stringPtr("10")
	parsed.IVDefenseRaw = stringPtr("12")
	parsed.IVStaminaRaw = stringPtr("13")

	decision := ValidateCandidate(parsed, catalog, nil)
	if !decision.SpeciesIsCanonical {
		t.Fatal("expected species to be canonical")
	}
	if len(decision.AcceptedResults) != 1 {
		t.Fatalf("expected 1 accepted candidate, got %d", len(decision.AcceptedResults))
	}
	accepted := decision.AcceptedResults[0]
	if accepted.SpeciesName != "Munna" {
		t.Fatalf("expected species name %q, got %q", "Munna", accepted.SpeciesName)
	}
	if accepted.LevelEstimate == nil {
		t.Fatal("expected inferred level estimate")
	}
	if math.Abs(*accepted.LevelEstimate-expectedLevel) > 0.001 {
		t.Fatalf("expected inferred level %.1f, got %.3f", expectedLevel, *accepted.LevelEstimate)
	}
	if accepted.LevelMethod != LevelMethodUnknown {
		t.Fatalf("expected level method %q, got %q", LevelMethodUnknown, accepted.LevelMethod)
	}
}

func TestValidateCandidateRejectsImpossibleCombination(t *testing.T) {
	t.Parallel()

	catalog := mustBuildValidationCatalog(t)

	parsed := ParseCandidateFromOCR("Munna")
	parsed.CPRaw = stringPtr("9999")
	parsed.HPRaw = stringPtr("999")
	parsed.IVAttackRaw = stringPtr("15")
	parsed.IVDefenseRaw = stringPtr("15")
	parsed.IVStaminaRaw = stringPtr("15")

	decision := ValidateCandidate(parsed, catalog, nil)
	if !decision.SpeciesIsCanonical {
		t.Fatal("expected canonical species match")
	}
	if len(decision.AcceptedResults) != 0 {
		t.Fatal("expected impossible combination to be rejected")
	}
}

func TestValidateCandidateRejectsUnknownSpecies(t *testing.T) {
	t.Parallel()

	catalog := mustBuildValidationCatalog(t)

	parsed := ParseCandidateFromOCR("DefinitelyNotAPokemon")
	parsed.CPRaw = stringPtr("500")
	parsed.HPRaw = stringPtr("100")
	parsed.IVAttackRaw = stringPtr("10")
	parsed.IVDefenseRaw = stringPtr("10")
	parsed.IVStaminaRaw = stringPtr("10")

	decision := ValidateCandidate(parsed, catalog, nil)
	if decision.SpeciesIsCanonical {
		t.Fatal("expected unknown species to be non-canonical")
	}
	if len(decision.AcceptedResults) != 0 {
		t.Fatal("expected unknown species to be rejected")
	}
}

func TestValidateCandidateAcceptsMultipleCanonicalOptionsAndExcludesShadow(t *testing.T) {
	t.Parallel()

	catalog := mustBuildValidationCatalogWithAmbiguousForms(t)
	entry, ok := catalog.EntryForNormalized("mr. mime")
	if !ok {
		t.Fatal("expected Mr. Mime entry in catalog")
	}

	expectedLevel := 20.0
	cp, hp, ok := catalog.ComputeCPHP(entry, expectedLevel, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation to succeed")
	}

	parsed := ParseCandidateFromOCR("Mr. Mime")
	parsed.CPRaw = stringPtr(fmt.Sprintf("%d", cp))
	parsed.HPRaw = stringPtr(fmt.Sprintf("%d", hp))
	parsed.IVAttackRaw = stringPtr("10")
	parsed.IVDefenseRaw = stringPtr("12")
	parsed.IVStaminaRaw = stringPtr("13")

	decision := ValidateCandidate(parsed, catalog, nil)
	if !decision.SpeciesIsCanonical {
		t.Fatal("expected species to be canonical")
	}
	if len(decision.AcceptedResults) != 2 {
		t.Fatalf("expected 2 accepted options (Mr. Mime + Mr. Mime (Galarian)), got %d", len(decision.AcceptedResults))
	}

	if decision.AcceptedResults[0].SpeciesName != "Mr. Mime" {
		t.Fatalf("expected first option %q, got %q", "Mr. Mime", decision.AcceptedResults[0].SpeciesName)
	}
	if decision.AcceptedResults[0].MatchMode != "exact" || decision.AcceptedResults[0].MatchDistance != 0 {
		t.Fatalf("expected first option to be exact distance 0, got mode=%q distance=%d", decision.AcceptedResults[0].MatchMode, decision.AcceptedResults[0].MatchDistance)
	}
	if decision.AcceptedResults[1].SpeciesName != "Mr. Mime (Galarian)" {
		t.Fatalf("expected second option %q, got %q", "Mr. Mime (Galarian)", decision.AcceptedResults[1].SpeciesName)
	}
	if decision.AcceptedResults[1].MatchMode != "prefix" || decision.AcceptedResults[1].MatchDistance != 0 {
		t.Fatalf("expected second option to be prefix distance 0, got mode=%q distance=%d", decision.AcceptedResults[1].MatchMode, decision.AcceptedResults[1].MatchDistance)
	}
}

func TestValidateCandidateAcceptsDarumakaBaseAndGalarian(t *testing.T) {
	t.Parallel()

	catalog := mustBuildValidationCatalogWithAmbiguousForms(t)
	entry, ok := catalog.EntryForNormalized("darumaka")
	if !ok {
		t.Fatal("expected Darumaka entry in catalog")
	}

	cp, hp, ok := catalog.ComputeCPHP(entry, 20.0, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation to succeed")
	}

	parsed := ParseCandidateFromOCR("Darumaka")
	parsed.CPRaw = stringPtr(fmt.Sprintf("%d", cp))
	parsed.HPRaw = stringPtr(fmt.Sprintf("%d", hp))
	parsed.IVAttackRaw = stringPtr("10")
	parsed.IVDefenseRaw = stringPtr("12")
	parsed.IVStaminaRaw = stringPtr("13")

	decision := ValidateCandidate(parsed, catalog, nil)
	if !decision.SpeciesIsCanonical {
		t.Fatal("expected species to be canonical")
	}
	if len(decision.AcceptedResults) != 2 {
		t.Fatalf("expected 2 accepted options (Darumaka + Darumaka (Galarian)), got %d", len(decision.AcceptedResults))
	}
	if decision.AcceptedResults[0].SpeciesName != "Darumaka" {
		t.Fatalf("expected first option %q, got %q", "Darumaka", decision.AcceptedResults[0].SpeciesName)
	}
	if decision.AcceptedResults[1].SpeciesName != "Darumaka (Galarian)" {
		t.Fatalf("expected second option %q, got %q", "Darumaka (Galarian)", decision.AcceptedResults[1].SpeciesName)
	}
}

func mustBuildValidationCatalog(t *testing.T) species.Catalog {
	t.Helper()

	catalog, err := species.NewCatalog([]species.Entry{
		{
			SpeciesID:         "munna",
			SpeciesName:       "Munna",
			SpeciesNormalized: "munna",
			BaseStats: species.BaseStats{
				Attack:  98,
				Defense: 138,
				Stamina: 183,
			},
		},
		{
			SpeciesID:         "zacian_hero",
			SpeciesName:       "Zacian",
			SpeciesNormalized: "zacian",
			BaseStats: species.BaseStats{
				Attack:  254,
				Defense: 236,
				Stamina: 192,
			},
		},
	})
	if err != nil {
		t.Fatalf("expected test catalog build to succeed: %v", err)
	}
	return catalog
}

func mustBuildValidationCatalogWithAmbiguousForms(t *testing.T) species.Catalog {
	t.Helper()

	sharedStats := species.BaseStats{
		Attack:  120,
		Defense: 120,
		Stamina: 120,
	}

	catalog, err := species.NewCatalog([]species.Entry{
		{
			SpeciesID:         "mr_mime",
			SpeciesName:       "Mr. Mime",
			SpeciesNormalized: "mr. mime",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "mr_mime_galarian",
			SpeciesName:       "Mr. Mime (Galarian)",
			SpeciesNormalized: "mr. mime (galarian)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "mr_mime_shadow",
			SpeciesName:       "Mr. Mime (Shadow)",
			SpeciesNormalized: "mr. mime (shadow)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka",
			SpeciesName:       "Darumaka",
			SpeciesNormalized: "darumaka",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka_galarian",
			SpeciesName:       "Darumaka (Galarian)",
			SpeciesNormalized: "darumaka (galarian)",
			BaseStats:         sharedStats,
		},
		{
			SpeciesID:         "darumaka_shadow",
			SpeciesName:       "Darumaka (Shadow)",
			SpeciesNormalized: "darumaka (shadow)",
			BaseStats:         sharedStats,
		},
	})
	if err != nil {
		t.Fatalf("expected ambiguous test catalog build to succeed: %v", err)
	}
	return catalog
}
