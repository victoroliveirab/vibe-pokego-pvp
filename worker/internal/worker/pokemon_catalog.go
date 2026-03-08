package worker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
)

var (
	pokemonCatalogOnce sync.Once
	pokemonCatalog     []string
	pokemonCatalogErr  error
)

func loadPokemonSpeciesCatalog() ([]string, error) {
	pokemonCatalogOnce.Do(func() {
		path, err := resolvePokemonCatalogPath()
		if err != nil {
			pokemonCatalogErr = err
			return
		}

		species, err := readPokemonSpeciesCatalog(path)
		if err != nil {
			pokemonCatalogErr = err
			return
		}

		pokemonCatalog = species
	})

	if pokemonCatalogErr != nil {
		return nil, pokemonCatalogErr
	}

	if len(pokemonCatalog) == 0 {
		return nil, fmt.Errorf("pokemon species catalog is empty")
	}

	return pokemonCatalog, nil
}

func resolvePokemonCatalogPath() (string, error) {
	candidates := []string{
		filepath.Join("internal", "pokemon.json"),
		filepath.Join("..", "pokemon.json"),
		filepath.Join("..", "internal", "pokemon.json"),
		filepath.Join("worker", "internal", "pokemon.json"),
		filepath.Join("..", "worker", "internal", "pokemon.json"),
		filepath.Join("..", "..", "worker", "internal", "pokemon.json"),
	}
	if _, sourceFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "pokemon.json")))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("pokemon catalog file not found (checked: %v)", candidates)
}

func readPokemonSpeciesCatalog(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pokemon catalog: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read pokemon catalog root token: %w", err)
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("pokemon catalog root must be an object")
	}

	type entry struct {
		SpeciesName string `json:"speciesName"`
	}

	species := make([]string, 0, 4096)
	for decoder.More() {
		if _, err := decoder.Token(); err != nil {
			return nil, fmt.Errorf("read pokemon catalog key: %w", err)
		}

		var parsedEntry entry
		if err := decoder.Decode(&parsedEntry); err != nil {
			return nil, fmt.Errorf("decode pokemon catalog entry: %w", err)
		}

		parsedSpecies := appraisal.ParseCandidateFromOCR(parsedEntry.SpeciesName)
		if parsedSpecies.SpeciesNameNormalized == nil {
			continue
		}
		species = append(species, *parsedSpecies.SpeciesNameNormalized)
	}

	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("read pokemon catalog closing token: %w", err)
	}

	if len(species) == 0 {
		return nil, fmt.Errorf("pokemon catalog contains no parsable species names")
	}

	return species, nil
}
