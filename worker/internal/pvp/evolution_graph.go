package pvp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EvolutionGraph stores forward-only species evolution relationships.
type EvolutionGraph struct {
	childrenBySpeciesID       map[string][]string
	speciesIDByCanonicalName  map[string]string
	speciesIDByNormalizedName map[string]string
}

type rawPokemonEntry struct {
	SpeciesID   string           `json:"speciesId"`
	SpeciesName string           `json:"speciesName"`
	Family      rawPokemonFamily `json:"family"`
}

type rawPokemonFamily struct {
	Evolutions []string `json:"evolutions"`
}

// LoadEvolutionGraph loads evolution data from the default pokemon.json path.
func LoadEvolutionGraph() (EvolutionGraph, error) {
	path, err := resolvePokemonCatalogPath()
	if err != nil {
		return EvolutionGraph{}, err
	}
	return LoadEvolutionGraphFromFile(path)
}

// LoadEvolutionGraphFromFile parses pokemon.json and builds forward traversal indexes.
func LoadEvolutionGraphFromFile(path string) (EvolutionGraph, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return EvolutionGraph{}, fmt.Errorf("pokemon catalog path is required")
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		return EvolutionGraph{}, fmt.Errorf("open pokemon catalog: %w", err)
	}
	defer file.Close()

	var payload map[string]rawPokemonEntry
	if err := json.NewDecoder(file).Decode(&payload); err != nil {
		return EvolutionGraph{}, fmt.Errorf("decode pokemon catalog: %w", err)
	}
	if len(payload) == 0 {
		return EvolutionGraph{}, fmt.Errorf("pokemon catalog contains no entries")
	}

	graph := EvolutionGraph{
		childrenBySpeciesID:       make(map[string][]string, len(payload)),
		speciesIDByCanonicalName:  make(map[string]string, len(payload)),
		speciesIDByNormalizedName: make(map[string]string, len(payload)),
	}

	for rawKey, entry := range payload {
		speciesID := normalizeSpeciesID(entry.SpeciesID)
		if speciesID == "" {
			speciesID = normalizeSpeciesID(rawKey)
		}
		if speciesID == "" {
			continue
		}

		if _, exists := graph.childrenBySpeciesID[speciesID]; !exists {
			graph.childrenBySpeciesID[speciesID] = make([]string, 0, 2)
		}

		canonicalName := strings.TrimSpace(entry.SpeciesName)
		if canonicalName != "" {
			if _, exists := graph.speciesIDByCanonicalName[canonicalName]; !exists {
				graph.speciesIDByCanonicalName[canonicalName] = speciesID
			}
			normalizedName := normalizeSpeciesName(canonicalName)
			if normalizedName != "" {
				if _, exists := graph.speciesIDByNormalizedName[normalizedName]; !exists {
					graph.speciesIDByNormalizedName[normalizedName] = speciesID
				}
			}
		}

		for _, rawChild := range entry.Family.Evolutions {
			childID := normalizeSpeciesID(rawChild)
			if childID == "" {
				continue
			}
			graph.childrenBySpeciesID[speciesID] = append(graph.childrenBySpeciesID[speciesID], childID)
			if _, exists := graph.childrenBySpeciesID[childID]; !exists {
				graph.childrenBySpeciesID[childID] = make([]string, 0, 1)
			}
		}
	}

	if len(graph.childrenBySpeciesID) == 0 {
		return EvolutionGraph{}, fmt.Errorf("pokemon catalog contains no valid species graph rows")
	}
	return graph, nil
}

// SpeciesIDForCanonicalName resolves an exact canonical species name to speciesID.
func (g EvolutionGraph) SpeciesIDForCanonicalName(speciesName string) (string, bool) {
	speciesID, ok := g.speciesIDByCanonicalName[strings.TrimSpace(speciesName)]
	return speciesID, ok
}

// SpeciesIDForNormalizedName resolves a normalized species name to speciesID.
func (g EvolutionGraph) SpeciesIDForNormalizedName(normalized string) (string, bool) {
	speciesID, ok := g.speciesIDByNormalizedName[normalizeSpeciesName(normalized)]
	return speciesID, ok
}

// ForwardFamily returns the species and all forward descendants, excluding pre-evolutions.
func (g EvolutionGraph) ForwardFamily(speciesID string) []string {
	start := normalizeSpeciesID(speciesID)
	if start == "" {
		return nil
	}
	if _, exists := g.childrenBySpeciesID[start]; !exists {
		return nil
	}

	seen := make(map[string]struct{}, 16)
	queue := []string{start}
	family := make([]string, 0, 16)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, visited := seen[current]; visited {
			continue
		}
		seen[current] = struct{}{}
		family = append(family, current)

		for _, child := range g.childrenBySpeciesID[current] {
			if _, visited := seen[child]; visited {
				continue
			}
			queue = append(queue, child)
		}
	}

	return family
}

func normalizeSpeciesName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func resolvePokemonCatalogPath() (string, error) {
	candidates := []string{
		filepath.Join("internal", "pokemon.json"),
		filepath.Join("..", "pokemon.json"),
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
