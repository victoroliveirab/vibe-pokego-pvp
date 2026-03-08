package species

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type levelCPMultiplier struct {
	Level float64
	CPM   float64
}

var defaultLevelCPMultipliers = []levelCPMultiplier{
	{Level: 1.0, CPM: 0.0940000000},
	{Level: 1.5, CPM: 0.1351374320},
	{Level: 2.0, CPM: 0.1663978700},
	{Level: 2.5, CPM: 0.1926509190},
	{Level: 3.0, CPM: 0.2157324700},
	{Level: 3.5, CPM: 0.2365726610},
	{Level: 4.0, CPM: 0.2557200500},
	{Level: 4.5, CPM: 0.2735303810},
	{Level: 5.0, CPM: 0.2902498800},
	{Level: 5.5, CPM: 0.3060573770},
	{Level: 6.0, CPM: 0.3210876000},
	{Level: 6.5, CPM: 0.3354450360},
	{Level: 7.0, CPM: 0.3492126800},
	{Level: 7.5, CPM: 0.3624577510},
	{Level: 8.0, CPM: 0.3752355900},
	{Level: 8.5, CPM: 0.3875924060},
	{Level: 9.0, CPM: 0.3995672800},
	{Level: 9.5, CPM: 0.4111935510},
	{Level: 10.0, CPM: 0.4225000100},
	{Level: 10.5, CPM: 0.4329264190},
	{Level: 11.0, CPM: 0.4431075500},
	{Level: 11.5, CPM: 0.4530599590},
	{Level: 12.0, CPM: 0.4627983900},
	{Level: 12.5, CPM: 0.4723360830},
	{Level: 13.0, CPM: 0.4816849500},
	{Level: 13.5, CPM: 0.4908558000},
	{Level: 14.0, CPM: 0.4998584400},
	{Level: 14.5, CPM: 0.5087017650},
	{Level: 15.0, CPM: 0.5173939500},
	{Level: 15.5, CPM: 0.5259425110},
	{Level: 16.0, CPM: 0.5343543300},
	{Level: 16.5, CPM: 0.5426357670},
	{Level: 17.0, CPM: 0.5507926900},
	{Level: 17.5, CPM: 0.5588305760},
	{Level: 18.0, CPM: 0.5667545200},
	{Level: 18.5, CPM: 0.5745691530},
	{Level: 19.0, CPM: 0.5822789100},
	{Level: 19.5, CPM: 0.5898879170},
	{Level: 20.0, CPM: 0.5974000100},
	{Level: 20.5, CPM: 0.6048188140},
	{Level: 21.0, CPM: 0.6121572900},
	{Level: 21.5, CPM: 0.6193993650},
	{Level: 22.0, CPM: 0.6265671300},
	{Level: 22.5, CPM: 0.6336445330},
	{Level: 23.0, CPM: 0.6406529500},
	{Level: 23.5, CPM: 0.6475764260},
	{Level: 24.0, CPM: 0.6544356300},
	{Level: 24.5, CPM: 0.6612148060},
	{Level: 25.0, CPM: 0.6679340000},
	{Level: 25.5, CPM: 0.6745775370},
	{Level: 26.0, CPM: 0.6811649200},
	{Level: 26.5, CPM: 0.6876806480},
	{Level: 27.0, CPM: 0.6941436500},
	{Level: 27.5, CPM: 0.7005386730},
	{Level: 28.0, CPM: 0.7068842100},
	{Level: 28.5, CPM: 0.7131649960},
	{Level: 29.0, CPM: 0.7193990900},
	{Level: 29.5, CPM: 0.7255715520},
	{Level: 30.0, CPM: 0.7317000000},
	{Level: 30.5, CPM: 0.7347410090},
	{Level: 31.0, CPM: 0.7377694800},
	{Level: 31.5, CPM: 0.7407855740},
	{Level: 32.0, CPM: 0.7437894300},
	{Level: 32.5, CPM: 0.7467812110},
	{Level: 33.0, CPM: 0.7497610400},
	{Level: 33.5, CPM: 0.7527290870},
	{Level: 34.0, CPM: 0.7556855100},
	{Level: 34.5, CPM: 0.7586303780},
	{Level: 35.0, CPM: 0.7615638400},
	{Level: 35.5, CPM: 0.7644860650},
	{Level: 36.0, CPM: 0.7673971700},
	{Level: 36.5, CPM: 0.7702972660},
	{Level: 37.0, CPM: 0.7731865000},
	{Level: 37.5, CPM: 0.7760649620},
	{Level: 38.0, CPM: 0.7789327500},
	{Level: 38.5, CPM: 0.7817900550},
	{Level: 39.0, CPM: 0.7846369700},
	{Level: 39.5, CPM: 0.7874735780},
	{Level: 40.0, CPM: 0.7903000100},
	{Level: 40.5, CPM: 0.7928039500},
	{Level: 41.0, CPM: 0.7953000100},
	{Level: 41.5, CPM: 0.7978039000},
	{Level: 42.0, CPM: 0.8003000000},
	{Level: 42.5, CPM: 0.8028039000},
	{Level: 43.0, CPM: 0.8053000000},
	{Level: 43.5, CPM: 0.8078038700},
	{Level: 44.0, CPM: 0.8102999900},
	{Level: 44.5, CPM: 0.8128038300},
	{Level: 45.0, CPM: 0.8152999900},
	{Level: 45.5, CPM: 0.8178038000},
	{Level: 46.0, CPM: 0.8202999900},
	{Level: 46.5, CPM: 0.8228038000},
	{Level: 47.0, CPM: 0.8252999900},
	{Level: 47.5, CPM: 0.8278038000},
	{Level: 48.0, CPM: 0.8302999900},
	{Level: 48.5, CPM: 0.8328038000},
	{Level: 49.0, CPM: 0.8352999900},
	{Level: 49.5, CPM: 0.8378038000},
	{Level: 50.0, CPM: 0.8402999900},
	{Level: 50.5, CPM: 0.8428038000},
	{Level: 51.0, CPM: 0.8452999900},
}

// BaseStats describes canonical species base stats from GameMaster.
type BaseStats struct {
	Attack  int
	Defense int
	Stamina int
}

// Entry is a canonical species row loaded from GameMaster.
type Entry struct {
	SpeciesID         string
	SpeciesName       string
	SpeciesNormalized string
	BaseStats         BaseStats
}

// LevelInference represents a validated inferred level from CP/HP/IV stats.
type LevelInference struct {
	LevelEstimate float64
	Confidence    float64
	Matches       int
}

type statLevelMatch struct {
	Level float64
}

// Catalog is a canonical species and stat-constraint index loaded from GameMaster.
type Catalog struct {
	entriesByNormalized map[string]Entry
	speciesNormalized   []string
	levels              []float64
	cpmByLevelKey       map[string]float64
}

// NewCatalog builds an immutable catalog from parsed canonical species entries.
func NewCatalog(entries []Entry) (Catalog, error) {
	if len(entries) == 0 {
		return Catalog{}, fmt.Errorf("catalog entries are required")
	}

	catalog := Catalog{
		entriesByNormalized: make(map[string]Entry, len(entries)),
		speciesNormalized:   make([]string, 0, len(entries)),
		levels:              make([]float64, 0, len(defaultLevelCPMultipliers)),
		cpmByLevelKey:       make(map[string]float64, len(defaultLevelCPMultipliers)),
	}

	for _, entry := range defaultLevelCPMultipliers {
		key := levelKey(entry.Level)
		catalog.cpmByLevelKey[key] = entry.CPM
		catalog.levels = append(catalog.levels, entry.Level)
	}
	sort.Float64s(catalog.levels)

	for _, entry := range entries {
		normalized := normalizeSpeciesName(entry.SpeciesNormalized)
		if normalized == "" {
			normalized = normalizeSpeciesName(entry.SpeciesName)
		}
		if normalized == "" {
			continue
		}
		if entry.BaseStats.Attack <= 0 || entry.BaseStats.Defense <= 0 || entry.BaseStats.Stamina <= 0 {
			continue
		}
		if _, exists := catalog.entriesByNormalized[normalized]; exists {
			continue
		}

		speciesName := strings.TrimSpace(entry.SpeciesName)
		if speciesName == "" {
			speciesName = normalized
		}

		normalizedEntry := Entry{
			SpeciesID:         strings.TrimSpace(entry.SpeciesID),
			SpeciesName:       speciesName,
			SpeciesNormalized: normalized,
			BaseStats:         entry.BaseStats,
		}
		if normalizedEntry.SpeciesID == "" {
			normalizedEntry.SpeciesID = normalized
		}

		catalog.entriesByNormalized[normalized] = normalizedEntry
		catalog.speciesNormalized = append(catalog.speciesNormalized, normalized)
	}

	if len(catalog.speciesNormalized) == 0 {
		return Catalog{}, fmt.Errorf("catalog contains no valid species entries")
	}

	return catalog, nil
}

// SpeciesNamesNormalized returns canonical species names normalized for matching.
func (c Catalog) SpeciesNamesNormalized() []string {
	names := make([]string, len(c.speciesNormalized))
	copy(names, c.speciesNormalized)
	return names
}

// EntryForNormalized returns the catalog entry for a normalized species name.
func (c Catalog) EntryForNormalized(normalized string) (Entry, bool) {
	entry, ok := c.entriesByNormalized[normalizeSpeciesName(normalized)]
	return entry, ok
}

// Levels returns a sorted copy of known playable levels.
func (c Catalog) Levels() []float64 {
	levels := make([]float64, len(c.levels))
	copy(levels, c.levels)
	return levels
}

// CPMultiplierForLevel returns the CPM value for a specific level.
func (c Catalog) CPMultiplierForLevel(level float64) (float64, bool) {
	cpm, ok := c.cpmByLevelKey[levelKey(level)]
	return cpm, ok
}

// ComputeCPHP computes expected CP and HP for a species at an exact level+IV tuple.
func (c Catalog) ComputeCPHP(entry Entry, level float64, ivAttack int, ivDefense int, ivStamina int) (int, int, bool) {
	cpm, ok := c.cpmByLevelKey[levelKey(level)]
	if !ok {
		return 0, 0, false
	}
	return calculateCP(entry.BaseStats, cpm, ivAttack, ivDefense, ivStamina),
		calculateHP(entry.BaseStats, cpm, ivStamina),
		true
}

// InferLevelForStats infers an exact playable level from species+CP+HP+IV values.
// It returns false when no feasible level exists.
func (c Catalog) InferLevelForStats(
	entry Entry,
	cp int,
	hp int,
	ivAttack int,
	ivDefense int,
	ivStamina int,
	levelHint *float64,
) (LevelInference, bool) {
	if cp <= 0 || hp <= 0 {
		return LevelInference{}, false
	}
	if ivAttack < 0 || ivAttack > 15 || ivDefense < 0 || ivDefense > 15 || ivStamina < 0 || ivStamina > 15 {
		return LevelInference{}, false
	}

	matches := make([]statLevelMatch, 0, 4)
	for _, level := range c.levels {
		expectedCP, expectedHP, ok := c.ComputeCPHP(entry, level, ivAttack, ivDefense, ivStamina)
		if !ok {
			continue
		}
		if absInt(expectedCP-cp) > 1 {
			continue
		}
		if absInt(expectedHP-hp) > 1 {
			continue
		}
		matches = append(matches, statLevelMatch{Level: level})
	}

	if len(matches) == 0 {
		return LevelInference{}, false
	}

	selected := matches[len(matches)/2]
	if levelHint != nil {
		target := *levelHint
		minDistance := math.MaxFloat64
		for _, match := range matches {
			distance := math.Abs(match.Level - target)
			if distance < minDistance {
				minDistance = distance
				selected = match
			}
		}
	}

	confidence := 1.0 / float64(len(matches))
	if len(matches) == 1 {
		confidence = 1.0
	}

	return LevelInference{
		LevelEstimate: selected.Level,
		Confidence:    confidence,
		Matches:       len(matches),
	}, true
}

func calculateCP(stats BaseStats, cpm float64, ivAttack int, ivDefense int, ivStamina int) int {
	attack := float64(stats.Attack + ivAttack)
	defense := float64(stats.Defense + ivDefense)
	stamina := float64(stats.Stamina + ivStamina)

	cp := int(math.Floor((attack * math.Sqrt(defense) * math.Sqrt(stamina) * cpm * cpm) / 10.0))
	if cp < 10 {
		cp = 10
	}
	return cp
}

func calculateHP(stats BaseStats, cpm float64, ivStamina int) int {
	hp := int(math.Floor(float64(stats.Stamina+ivStamina) * cpm))
	if hp < 10 {
		hp = 10
	}
	return hp
}

func normalizeSpeciesName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func levelKey(level float64) string {
	return strconv.FormatFloat(level, 'f', 1, 64)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
