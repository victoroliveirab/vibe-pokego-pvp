package pvp

import (
	"math"
	"testing"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
)

func TestEvaluateSpeciesIVRankingsReturnsAllSpreads(t *testing.T) {
	t.Parallel()

	evaluator := newTestEvaluator(t)
	ranked, err := evaluator.EvaluateSpeciesIVRankings("bulbasaur", 1500)
	if err != nil {
		t.Fatalf("expected ranking to succeed: %v", err)
	}

	if len(ranked) != 4096 {
		t.Fatalf("expected 4096 ranked rows, got %d", len(ranked))
	}
	if ranked[0].Rank != 1 {
		t.Fatalf("expected first rank to be 1, got %d", ranked[0].Rank)
	}
	if ranked[len(ranked)-1].Rank != len(ranked) {
		t.Fatalf("expected final rank to be %d, got %d", len(ranked), ranked[len(ranked)-1].Rank)
	}
	if math.Abs(ranked[0].Percentage-100.0) > 1e-6 {
		t.Fatalf("expected first percentage to be 100, got %.6f", ranked[0].Percentage)
	}

	for idx := 1; idx < len(ranked); idx++ {
		if !isOrdered(ranked[idx-1], ranked[idx]) {
			t.Fatalf("rank order violated between indices %d and %d: %#v then %#v", idx-1, idx, ranked[idx-1], ranked[idx])
		}
	}
}

func TestEvaluateMaxCPReturnsRequestedTuple(t *testing.T) {
	t.Parallel()

	evaluator := newTestEvaluator(t)
	evaluation, err := evaluator.EvaluateMaxCP("bulbasaur", 0, 15, 0, 1500)
	if err != nil {
		t.Fatalf("expected tuple evaluation to succeed: %v", err)
	}

	ranked, err := evaluator.EvaluateSpeciesIVRankings("bulbasaur", 1500)
	if err != nil {
		t.Fatalf("expected ranking to succeed: %v", err)
	}

	var expected RankedSpread
	found := false
	for _, row := range ranked {
		if row.IVAttack == 0 && row.IVDefense == 15 && row.IVStamina == 0 {
			expected = row
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find tuple 0/15/0 in ranked rows")
	}

	if evaluation.MaxCP != 1500 {
		t.Fatalf("expected maxCP 1500, got %d", evaluation.MaxCP)
	}
	if evaluation.EvaluatedSpeciesID != "bulbasaur" {
		t.Fatalf("expected species id %q, got %q", "bulbasaur", evaluation.EvaluatedSpeciesID)
	}
	if evaluation.Rank != expected.Rank {
		t.Fatalf("expected rank %d, got %d", expected.Rank, evaluation.Rank)
	}
	if math.Abs(evaluation.StatProduct-expected.StatProduct) > 1e-6 {
		t.Fatalf("expected stat product %.6f, got %.6f", expected.StatProduct, evaluation.StatProduct)
	}
	if math.Abs(evaluation.Percentage-expected.Percentage) > 1e-6 {
		t.Fatalf("expected percentage %.6f, got %.6f", expected.Percentage, evaluation.Percentage)
	}
	if evaluation.BestLevel > maxPlayableLevel {
		t.Fatalf("expected best level <= %.1f, got %.1f", maxPlayableLevel, evaluation.BestLevel)
	}
}

func TestEvaluateMaxCPRejectsInvalidTuple(t *testing.T) {
	t.Parallel()

	evaluator := newTestEvaluator(t)
	if _, err := evaluator.EvaluateMaxCP("bulbasaur", -1, 10, 10, 1500); err == nil {
		t.Fatal("expected invalid iv to fail")
	}
}

func TestEvaluateSpeciesIVRankingsRejectsUnknownSpecies(t *testing.T) {
	t.Parallel()

	evaluator := newTestEvaluator(t)
	if _, err := evaluator.EvaluateSpeciesIVRankings("missingno", 1500); err == nil {
		t.Fatal("expected unknown species to fail")
	}
}

func TestSortRankedSpreadsTieBreakers(t *testing.T) {
	t.Parallel()

	rows := []RankedSpread{
		{IVAttack: 10, IVDefense: 10, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 11, IVDefense: 10, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 10, IVDefense: 11, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 10, IVDefense: 10, IVStamina: 11, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 0, IVDefense: 0, IVStamina: 0, BestCP: 501, StatProduct: 120.0},
		{IVAttack: 15, IVDefense: 15, IVStamina: 15, BestCP: 500, StatProduct: 119.0},
	}

	sortRankedSpreads(rows)

	expected := []RankedSpread{
		{IVAttack: 0, IVDefense: 0, IVStamina: 0, BestCP: 501, StatProduct: 120.0},
		{IVAttack: 11, IVDefense: 10, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 10, IVDefense: 11, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 10, IVDefense: 10, IVStamina: 11, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 10, IVDefense: 10, IVStamina: 10, BestCP: 500, StatProduct: 120.0},
		{IVAttack: 15, IVDefense: 15, IVStamina: 15, BestCP: 500, StatProduct: 119.0},
	}

	for idx := range expected {
		if rows[idx].IVAttack != expected[idx].IVAttack ||
			rows[idx].IVDefense != expected[idx].IVDefense ||
			rows[idx].IVStamina != expected[idx].IVStamina ||
			rows[idx].BestCP != expected[idx].BestCP {
			t.Fatalf("unexpected ordering at index %d: got %#v want %#v", idx, rows[idx], expected[idx])
		}
	}
}

func newTestEvaluator(t *testing.T) Evaluator {
	t.Helper()

	catalog, err := species.NewCatalog([]species.Entry{
		{
			SpeciesID:         "bulbasaur",
			SpeciesName:       "Bulbasaur",
			SpeciesNormalized: "bulbasaur",
			BaseStats: species.BaseStats{
				Attack:  118,
				Defense: 111,
				Stamina: 128,
			},
		},
	})
	if err != nil {
		t.Fatalf("expected catalog construction to succeed: %v", err)
	}
	return NewEvaluator(catalog)
}

func isOrdered(left RankedSpread, right RankedSpread) bool {
	if left.StatProduct > right.StatProduct+epsilon {
		return true
	}
	if right.StatProduct > left.StatProduct+epsilon {
		return false
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
	return left.BestLevel >= right.BestLevel
}
