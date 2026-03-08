package species

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type rawGameMasterEntry struct {
	SpeciesName string            `json:"speciesName"`
	SpeciesID   string            `json:"speciesId"`
	BaseStats   rawGameMasterStat `json:"baseStats"`
}

type rawGameMasterStat struct {
	Atk         int `json:"atk"`
	Def         int `json:"def"`
	HP          int `json:"hp"`
	BaseAttack  int `json:"baseAttack"`
	BaseDefense int `json:"baseDefense"`
	BaseStamina int `json:"baseStamina"`
	Attack      int `json:"attack"`
	Defense     int `json:"defense"`
	Stamina     int `json:"stamina"`
}

// LoadCatalogFromFile loads canonical species and base stats from a local GameMaster-like JSON file.
func LoadCatalogFromFile(path string) (Catalog, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return Catalog{}, fmt.Errorf("gamemaster path is required")
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		return Catalog{}, fmt.Errorf("open gamemaster file: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	token, err := decoder.Token()
	if err != nil {
		return Catalog{}, fmt.Errorf("read gamemaster root token: %w", err)
	}

	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return Catalog{}, fmt.Errorf("gamemaster root must be an object")
	}

	entries := make([]Entry, 0, 2048)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return Catalog{}, fmt.Errorf("read gamemaster key: %w", err)
		}

		key, ok := keyToken.(string)
		if !ok {
			return Catalog{}, fmt.Errorf("gamemaster key token must be a string")
		}

		var decoded rawGameMasterEntry
		if err := decoder.Decode(&decoded); err != nil {
			return Catalog{}, fmt.Errorf("decode gamemaster entry %q: %w", key, err)
		}

		normalizedSpecies := normalizeSpeciesName(decoded.SpeciesName)
		if normalizedSpecies == "" {
			continue
		}

		stats := decoded.BaseStats.normalized()
		if stats.Attack <= 0 || stats.Defense <= 0 || stats.Stamina <= 0 {
			continue
		}

		speciesID := strings.TrimSpace(decoded.SpeciesID)
		if speciesID == "" {
			speciesID = strings.TrimSpace(key)
		}

		entries = append(entries, Entry{
			SpeciesID:         speciesID,
			SpeciesName:       strings.TrimSpace(decoded.SpeciesName),
			SpeciesNormalized: normalizedSpecies,
			BaseStats:         stats,
		})
	}

	if _, err := decoder.Token(); err != nil {
		return Catalog{}, fmt.Errorf("read gamemaster closing token: %w", err)
	}

	catalog, err := NewCatalog(entries)
	if err != nil {
		return Catalog{}, fmt.Errorf("build gamemaster catalog: %w", err)
	}
	return catalog, nil
}

func (s rawGameMasterStat) normalized() BaseStats {
	attack := firstPositiveInt(s.Atk, s.BaseAttack, s.Attack)
	defense := firstPositiveInt(s.Def, s.BaseDefense, s.Defense)
	stamina := firstPositiveInt(s.HP, s.BaseStamina, s.Stamina)
	return BaseStats{
		Attack:  attack,
		Defense: defense,
		Stamina: stamina,
	}
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
