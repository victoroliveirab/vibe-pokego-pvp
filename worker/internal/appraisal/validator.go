package appraisal

import (
	"strconv"
	"strings"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
)

// AcceptedResultCandidate is a canonical appraisal candidate accepted for persistence.
type AcceptedResultCandidate struct {
	SpeciesName           string
	SpeciesNameNormalized string
	CP                    int
	HP                    int
	IVAttack              int
	IVDefense             int
	IVStamina             int
	LevelEstimate         *float64
	LevelConfidence       *float64
	LevelMethod           string
	MatchMode             string
	MatchDistance         int
}

// ValidationDecision describes canonical status and optional accepted-result data.
type ValidationDecision struct {
	SpeciesIsCanonical bool
	AcceptedResults    []AcceptedResultCandidate
}

// ValidateCandidate enforces canonical species and stat consistency checks.
// Level is inferred from CP/HP/IV + species base stats when possible.
func ValidateCandidate(parsed ParsedCandidate, catalog species.Catalog, levelHint *float64) ValidationDecision {
	speciesNames := catalog.SpeciesNamesNormalized()
	if len(speciesNames) == 0 {
		return ValidationDecision{}
	}

	matches := ResolveCanonicalSpeciesMatches(parsed, speciesNames)
	if len(matches) == 0 {
		return ValidationDecision{}
	}

	decision := ValidationDecision{SpeciesIsCanonical: true}

	cp, ok := parseNumericField(parsed.CPRaw, 10, 20000)
	if !ok {
		return decision
	}
	hp, ok := parseNumericField(parsed.HPRaw, 10, 2000)
	if !ok {
		return decision
	}
	ivAttack, ok := parseNumericField(parsed.IVAttackRaw, 0, 15)
	if !ok {
		return decision
	}
	ivDefense, ok := parseNumericField(parsed.IVDefenseRaw, 0, 15)
	if !ok {
		return decision
	}
	ivStamina, ok := parseNumericField(parsed.IVStaminaRaw, 0, 15)
	if !ok {
		return decision
	}

	accepted := make([]AcceptedResultCandidate, 0, len(matches))
	for _, match := range matches {
		entry, ok := catalog.EntryForNormalized(match.SpeciesNormalized)
		if !ok {
			continue
		}

		inferredLevel, ok := catalog.InferLevelForStats(entry, cp, hp, ivAttack, ivDefense, ivStamina, levelHint)
		if !ok {
			continue
		}

		levelEstimate := inferredLevel.LevelEstimate
		levelConfidence := inferredLevel.Confidence
		accepted = append(accepted, AcceptedResultCandidate{
			SpeciesName:           entry.SpeciesName,
			SpeciesNameNormalized: entry.SpeciesNormalized,
			CP:                    cp,
			HP:                    hp,
			IVAttack:              ivAttack,
			IVDefense:             ivDefense,
			IVStamina:             ivStamina,
			LevelEstimate:         &levelEstimate,
			LevelConfidence:       &levelConfidence,
			LevelMethod:           LevelMethodUnknown,
			MatchMode:             match.Mode,
			MatchDistance:         match.Distance,
		})
	}
	decision.AcceptedResults = accepted

	return decision
}

func parseNumericField(raw *string, minValue int, maxValue int) (int, bool) {
	if raw == nil {
		return 0, false
	}

	value := strings.TrimSpace(*raw)
	if value == "" {
		return 0, false
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	if parsed < minValue || parsed > maxValue {
		return 0, false
	}

	return parsed, true
}
