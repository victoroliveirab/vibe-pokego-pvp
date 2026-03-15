package pvp

// MaxCPEvaluation captures PvP max-CP ranking output for one IV tuple.
type MaxCPEvaluation struct {
	MaxCP              int
	EvaluatedSpeciesID string
	BestLevel          float64
	BestCP             int
	StatProduct        float64
	Rank               int
	Percentage         float64
}

// RankedSpread stores one IV tuple row within the species+maxCP ranking table.
type RankedSpread struct {
	IVAttack    int
	IVDefense   int
	IVStamina   int
	BestLevel   float64
	BestCP      int
	StatProduct float64
	Rank        int
	Percentage  float64
}
