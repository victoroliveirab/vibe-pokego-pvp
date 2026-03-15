package pvp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEvolutionGraphFromFileBuildsForwardGraph(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pokemon.json")
	fixture := `{
  "seed": {
    "speciesId": "seed",
    "speciesName": "Seed",
    "family": { "evolutions": ["bloom", "thorn"] }
  },
  "bloom": {
    "speciesId": "bloom",
    "speciesName": "Bloom",
    "family": { "evolutions": ["petal"] }
  },
  "thorn": {
    "speciesId": "thorn",
    "speciesName": "Thorn",
    "family": {}
  },
  "petal": {
    "speciesId": "petal",
    "speciesName": "Petal",
    "family": {}
  }
}`
	if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
		t.Fatalf("expected fixture write to succeed: %v", err)
	}

	graph, err := LoadEvolutionGraphFromFile(path)
	if err != nil {
		t.Fatalf("expected graph load to succeed: %v", err)
	}

	speciesID, ok := graph.SpeciesIDForCanonicalName("Seed")
	if !ok || speciesID != "seed" {
		t.Fatalf("expected canonical name lookup to resolve seed, got (%q, %t)", speciesID, ok)
	}
	speciesID, ok = graph.SpeciesIDForNormalizedName("seed")
	if !ok || speciesID != "seed" {
		t.Fatalf("expected normalized name lookup to resolve seed, got (%q, %t)", speciesID, ok)
	}

	assertFamilyEquals(t, graph.ForwardFamily("seed"), []string{"seed", "bloom", "thorn", "petal"})
	assertFamilyEquals(t, graph.ForwardFamily("petal"), []string{"petal"})
}

func TestLoadEvolutionGraphDefaultFileBranchingFamilies(t *testing.T) {
	t.Parallel()

	graph, err := LoadEvolutionGraph()
	if err != nil {
		t.Fatalf("expected default graph load to succeed: %v", err)
	}

	assertFamilyContainsAll(t, graph.ForwardFamily("eevee"), []string{
		"eevee",
		"vaporeon",
		"jolteon",
		"flareon",
		"espeon",
		"umbreon",
		"leafeon",
		"glaceon",
		"sylveon",
	})
	assertFamilyContainsAll(t, graph.ForwardFamily("wurmple"), []string{
		"wurmple",
		"silcoon",
		"cascoon",
		"beautifly",
		"dustox",
	})

	silcoonFamily := graph.ForwardFamily("silcoon")
	assertFamilyContainsAll(t, silcoonFamily, []string{"silcoon", "beautifly"})
	assertFamilyExcludes(t, silcoonFamily, []string{"wurmple", "cascoon", "dustox"})
}

func assertFamilyEquals(t *testing.T, got []string, expected []string) {
	t.Helper()

	if len(got) != len(expected) {
		t.Fatalf("expected family length %d, got %d (%v)", len(expected), len(got), got)
	}
	for idx := range expected {
		if got[idx] != expected[idx] {
			t.Fatalf("unexpected family element at index %d: got %q want %q", idx, got[idx], expected[idx])
		}
	}
}

func assertFamilyContainsAll(t *testing.T, got []string, expected []string) {
	t.Helper()

	set := make(map[string]struct{}, len(got))
	for _, speciesID := range got {
		set[speciesID] = struct{}{}
	}
	for _, speciesID := range expected {
		if _, ok := set[speciesID]; !ok {
			t.Fatalf("expected family to include %q, got %v", speciesID, got)
		}
	}
}

func assertFamilyExcludes(t *testing.T, got []string, excluded []string) {
	t.Helper()

	set := make(map[string]struct{}, len(got))
	for _, speciesID := range got {
		set[speciesID] = struct{}{}
	}
	for _, speciesID := range excluded {
		if _, ok := set[speciesID]; ok {
			t.Fatalf("expected family to exclude %q, got %v", speciesID, got)
		}
	}
}
