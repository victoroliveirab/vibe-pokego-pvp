package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPokemonSpeciesCatalogPreservesObjectOrder(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "pokemon.json")

	content := `{
  "entry_a": { "speciesName": "Lallall" },
  "entry_b": { "speciesName": "Zacian" },
  "entry_c": { "speciesName": "Mr. Mime" }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("expected temp catalog write to succeed: %v", err)
	}

	species, err := readPokemonSpeciesCatalog(path)
	if err != nil {
		t.Fatalf("expected catalog parse to succeed: %v", err)
	}

	if len(species) != 3 {
		t.Fatalf("expected 3 species entries, got %d", len(species))
	}
	if species[0] != "lallall" || species[1] != "zacian" || species[2] != "mr. mime" {
		t.Fatalf("unexpected species order/content: %#v", species)
	}
}

func TestReadPokemonSpeciesCatalogRejectsInvalidRoot(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "pokemon.json")

	content := `["not", "an", "object"]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("expected temp catalog write to succeed: %v", err)
	}

	if _, err := readPokemonSpeciesCatalog(path); err == nil {
		t.Fatal("expected invalid root to fail")
	}
}
