package pvp

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
)

const (
	minIV            = 0
	maxIV            = 15
	maxPlayableLevel = 50.0
	epsilon          = 1e-9
)

// Evaluator calculates max-CP rankings for species and IV tuples.
type Evaluator struct {
	catalog          species.Catalog
	entriesBySpecies map[string]species.Entry
}

// NewEvaluator creates a ranking evaluator using canonical species catalog data.
func NewEvaluator(catalog species.Catalog) Evaluator {
	entriesBySpecies := make(map[string]species.Entry)
	for _, normalized := range catalog.SpeciesNamesNormalized() {
		entry, ok := catalog.EntryForNormalized(normalized)
		if !ok {
			continue
		}

		speciesID := normalizeSpeciesID(entry.SpeciesID)
		if speciesID == "" {
			speciesID = normalizeSpeciesID(normalized)
		}
		if speciesID == "" {
			continue
		}
		if _, exists := entriesBySpecies[speciesID]; exists {
			continue
		}
		entriesBySpecies[speciesID] = entry
	}
	return Evaluator{
		catalog:          catalog,
		entriesBySpecies: entriesBySpecies,
	}
}

// EvaluateSpeciesIVRankings ranks all 4096 IV tuples for one species and maxCP cap.
func (e Evaluator) EvaluateSpeciesIVRankings(speciesID string, maxCP int) ([]RankedSpread, error) {
	if maxCP <= 0 {
		return nil, fmt.Errorf("maxCP must be positive")
	}

	entry, err := e.resolveEntry(speciesID)
	if err != nil {
		return nil, err
	}

	levels := make([]float64, 0, 128)
	for _, level := range e.catalog.Levels() {
		if level > maxPlayableLevel {
			continue
		}
		levels = append(levels, level)
	}
	if len(levels) == 0 {
		return nil, fmt.Errorf("catalog has no playable levels")
	}

	ranked := make([]RankedSpread, 0, 4096)
	for ivAttack := minIV; ivAttack <= maxIV; ivAttack++ {
		for ivDefense := minIV; ivDefense <= maxIV; ivDefense++ {
			for ivStamina := minIV; ivStamina <= maxIV; ivStamina++ {
				best, ok := e.bestForTuple(entry, levels, ivAttack, ivDefense, ivStamina, maxCP)
				if !ok {
					continue
				}
				ranked = append(ranked, best)
			}
		}
	}

	if len(ranked) == 0 {
		return nil, fmt.Errorf("no feasible IV tuples for species %q at maxCP %d", speciesID, maxCP)
	}

	sortRankedSpreads(ranked)

	maxProduct := ranked[0].StatProduct
	for idx := range ranked {
		ranked[idx].Rank = idx + 1
		if maxProduct > 0 {
			ranked[idx].Percentage = (ranked[idx].StatProduct / maxProduct) * 100.0
		}
	}

	return ranked, nil
}

// EvaluateMaxCP returns ranking output for one species+IV tuple under one maxCP cap.
func (e Evaluator) EvaluateMaxCP(
	speciesID string,
	ivAttack int,
	ivDefense int,
	ivStamina int,
	maxCP int,
) (MaxCPEvaluation, error) {
	if !isValidIV(ivAttack) || !isValidIV(ivDefense) || !isValidIV(ivStamina) {
		return MaxCPEvaluation{}, fmt.Errorf("ivs must be in [0, 15]")
	}

	ranked, err := e.EvaluateSpeciesIVRankings(speciesID, maxCP)
	if err != nil {
		return MaxCPEvaluation{}, err
	}

	for _, row := range ranked {
		if row.IVAttack != ivAttack || row.IVDefense != ivDefense || row.IVStamina != ivStamina {
			continue
		}
		return MaxCPEvaluation{
			MaxCP:              maxCP,
			EvaluatedSpeciesID: normalizeSpeciesID(speciesID),
			BestLevel:          row.BestLevel,
			BestCP:             row.BestCP,
			StatProduct:        row.StatProduct,
			Rank:               row.Rank,
			Percentage:         row.Percentage,
		}, nil
	}

	return MaxCPEvaluation{}, fmt.Errorf("iv tuple %d/%d/%d not found in ranking", ivAttack, ivDefense, ivStamina)
}

func (e Evaluator) resolveEntry(speciesID string) (species.Entry, error) {
	normalizedID := normalizeSpeciesID(speciesID)
	entry, ok := e.entriesBySpecies[normalizedID]
	if !ok {
		return species.Entry{}, fmt.Errorf("species %q not found in catalog", speciesID)
	}
	return entry, nil
}

func (e Evaluator) bestForTuple(
	entry species.Entry,
	levels []float64,
	ivAttack int,
	ivDefense int,
	ivStamina int,
	maxCP int,
) (RankedSpread, bool) {
	best := RankedSpread{
		IVAttack:  ivAttack,
		IVDefense: ivDefense,
		IVStamina: ivStamina,
	}
	found := false

	for _, level := range levels {
		cp, hp, ok := e.catalog.ComputeCPHP(entry, level, ivAttack, ivDefense, ivStamina)
		if !ok || cp > maxCP {
			continue
		}

		cpm, ok := e.catalog.CPMultiplierForLevel(level)
		if !ok {
			continue
		}
		statProduct := computeStatProduct(entry.BaseStats, cpm, ivAttack, ivDefense, hp)

		if !found {
			best.BestLevel = level
			best.BestCP = cp
			best.StatProduct = statProduct
			found = true
			continue
		}
		if statProduct > best.StatProduct+epsilon {
			best.BestLevel = level
			best.BestCP = cp
			best.StatProduct = statProduct
			continue
		}
		if math.Abs(statProduct-best.StatProduct) <= epsilon {
			if cp > best.BestCP || (cp == best.BestCP && level > best.BestLevel) {
				best.BestLevel = level
				best.BestCP = cp
				best.StatProduct = statProduct
			}
		}
	}

	return best, found
}

func computeStatProduct(base species.BaseStats, cpm float64, ivAttack int, ivDefense int, hp int) float64 {
	attack := float64(base.Attack+ivAttack) * cpm
	defense := float64(base.Defense+ivDefense) * cpm
	return attack * defense * float64(hp)
}

func sortRankedSpreads(rows []RankedSpread) {
	sort.Slice(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]

		if math.Abs(left.StatProduct-right.StatProduct) > epsilon {
			return left.StatProduct > right.StatProduct
		}
		if left.BestCP != right.BestCP {
			return left.BestCP > right.BestCP
		}
		if left.IVAttack != right.IVAttack {
			return left.IVAttack > right.IVAttack
		}
		if left.IVDefense != right.IVDefense {
			return left.IVDefense > right.IVDefense
		}
		if left.IVStamina != right.IVStamina {
			return left.IVStamina > right.IVStamina
		}
		return left.BestLevel > right.BestLevel
	})
}

func normalizeSpeciesID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isValidIV(value int) bool {
	return value >= minIV && value <= maxIV
}
