package species

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogFromFileParsesEntries(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "gamemaster.json")
	content := `{
  "munna": {
    "speciesName": "Munna",
    "speciesId": "munna",
    "baseStats": { "atk": 98, "def": 138, "hp": 183 }
  },
  "zacian_hero": {
    "speciesName": "Zacian",
    "speciesId": "zacian_hero",
    "baseStats": { "baseAttack": 254, "baseDefense": 236, "baseStamina": 192 }
  },
  "broken_entry": {
    "speciesName": "Brokenmon",
    "speciesId": "broken_entry",
    "baseStats": {}
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("expected fixture write to succeed: %v", err)
	}

	catalog, err := LoadCatalogFromFile(path)
	if err != nil {
		t.Fatalf("expected gamemaster load to succeed: %v", err)
	}

	names := catalog.SpeciesNamesNormalized()
	if len(names) != 2 {
		t.Fatalf("expected 2 valid species entries, got %d (%v)", len(names), names)
	}

	munna, ok := catalog.EntryForNormalized("munna")
	if !ok {
		t.Fatal("expected munna entry in catalog")
	}
	if munna.SpeciesID != "munna" {
		t.Fatalf("expected munna species id %q, got %q", "munna", munna.SpeciesID)
	}
	if munna.BaseStats.Attack != 98 || munna.BaseStats.Defense != 138 || munna.BaseStats.Stamina != 183 {
		t.Fatalf("unexpected munna base stats: %#v", munna.BaseStats)
	}
}

func TestLoadCatalogFromFileRejectsMissingPath(t *testing.T) {
	t.Parallel()

	_, err := LoadCatalogFromFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected missing gamemaster path to fail")
	}
}

func TestLoadCatalogFromFileRejectsInvalidRoot(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "gamemaster.json")
	if err := os.WriteFile(path, []byte(`["not","an","object"]`), 0o644); err != nil {
		t.Fatalf("expected fixture write to succeed: %v", err)
	}

	_, err := LoadCatalogFromFile(path)
	if err == nil {
		t.Fatal("expected invalid gamemaster root to fail")
	}
}

func TestInferLevelForStatsFindsExpectedLevel(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog([]Entry{
		{
			SpeciesID:         "munna",
			SpeciesName:       "Munna",
			SpeciesNormalized: "munna",
			BaseStats: BaseStats{
				Attack:  98,
				Defense: 138,
				Stamina: 183,
			},
		},
	})
	if err != nil {
		t.Fatalf("expected catalog build to succeed: %v", err)
	}

	entry, ok := catalog.EntryForNormalized("munna")
	if !ok {
		t.Fatal("expected munna entry")
	}

	cp, hp, ok := catalog.ComputeCPHP(entry, 20.0, 10, 12, 13)
	if !ok {
		t.Fatal("expected cp/hp computation for level 20")
	}

	inferred, ok := catalog.InferLevelForStats(entry, cp, hp, 10, 12, 13, nil)
	if !ok {
		t.Fatalf("expected level inference to succeed for cp=%d hp=%d", cp, hp)
	}
	if math.Abs(inferred.LevelEstimate-20.0) > 0.001 {
		t.Fatalf("expected inferred level 20.0, got %.3f", inferred.LevelEstimate)
	}
	if inferred.Confidence <= 0 || inferred.Confidence > 1 {
		t.Fatalf("expected confidence in (0,1], got %.3f", inferred.Confidence)
	}
}

func TestInferLevelForStatsRejectsImpossibleTuple(t *testing.T) {
	t.Parallel()

	catalog, err := NewCatalog([]Entry{
		{
			SpeciesID:         "munna",
			SpeciesName:       "Munna",
			SpeciesNormalized: "munna",
			BaseStats: BaseStats{
				Attack:  98,
				Defense: 138,
				Stamina: 183,
			},
		},
	})
	if err != nil {
		t.Fatalf("expected catalog build to succeed: %v", err)
	}

	entry, ok := catalog.EntryForNormalized("munna")
	if !ok {
		t.Fatal("expected munna entry")
	}

	if _, ok := catalog.InferLevelForStats(entry, 9999, 999, 15, 15, 15, nil); ok {
		t.Fatal("expected impossible tuple to be rejected")
	}
}
