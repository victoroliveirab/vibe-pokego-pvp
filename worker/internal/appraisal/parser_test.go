package appraisal

import "testing"

func TestParseCandidateFromOCRNormalizesSpeciesName(t *testing.T) {
	parsed := ParseCandidateFromOCR("  mR.   mime  ")
	if parsed.SpeciesNameRaw == nil {
		t.Fatal("expected species raw name")
	}
	if parsed.SpeciesNameNormalized == nil {
		t.Fatal("expected species normalized name")
	}

	if *parsed.SpeciesNameRaw != "Mr. Mime" {
		t.Fatalf("expected normalized raw %q, got %q", "Mr. Mime", *parsed.SpeciesNameRaw)
	}
	if *parsed.SpeciesNameNormalized != "mr. mime" {
		t.Fatalf("expected normalized value %q, got %q", "mr. mime", *parsed.SpeciesNameNormalized)
	}
}

func TestParseCandidateFromOCRHandlesApostropheAndHyphen(t *testing.T) {
	cases := []struct {
		input          string
		expectedRaw    string
		expectedParsed string
	}{
		{input: "farfetch'd", expectedRaw: "Farfetch'd", expectedParsed: "farfetch'd"},
		{input: "ho-oh", expectedRaw: "Ho-Oh", expectedParsed: "ho-oh"},
	}

	for _, tc := range cases {
		parsed := ParseCandidateFromOCR(tc.input)
		if parsed.SpeciesNameRaw == nil || parsed.SpeciesNameNormalized == nil {
			t.Fatalf("expected non-empty values for input %q", tc.input)
		}
		if *parsed.SpeciesNameRaw != tc.expectedRaw {
			t.Fatalf("input %q expected raw %q, got %q", tc.input, tc.expectedRaw, *parsed.SpeciesNameRaw)
		}
		if *parsed.SpeciesNameNormalized != tc.expectedParsed {
			t.Fatalf("input %q expected normalized %q, got %q", tc.input, tc.expectedParsed, *parsed.SpeciesNameNormalized)
		}
	}
}

func TestParseCandidateFromOCREmptyInput(t *testing.T) {
	parsed := ParseCandidateFromOCR(" \n\t ")
	if parsed.SpeciesNameRaw != nil {
		t.Fatalf("expected nil species raw, got %q", *parsed.SpeciesNameRaw)
	}
	if parsed.SpeciesNameNormalized != nil {
		t.Fatalf("expected nil species normalized, got %q", *parsed.SpeciesNameNormalized)
	}
}

func TestParseCandidateFromOCRSkipsLabelNoiseAndChoosesSpeciesLine(t *testing.T) {
	parsed := ParseCandidateFromOCR(`
LUCKY POKEMON
122 / 122 HP
Zacian
This Zacian was caught on 8/24/2025
`)

	if parsed.SpeciesNameRaw == nil || parsed.SpeciesNameNormalized == nil {
		t.Fatal("expected extracted species values")
	}
	if *parsed.SpeciesNameRaw != "Zacian" {
		t.Fatalf("expected species raw %q, got %q", "Zacian", *parsed.SpeciesNameRaw)
	}
	if *parsed.SpeciesNameNormalized != "zacian" {
		t.Fatalf("expected species normalized %q, got %q", "zacian", *parsed.SpeciesNameNormalized)
	}
}

func TestParseCandidateFromOCRRejectsGarbageToken(t *testing.T) {
	parsed := ParseCandidateFromOCR(".-a")
	if parsed.SpeciesNameRaw != nil {
		t.Fatalf("expected nil species raw, got %q", *parsed.SpeciesNameRaw)
	}
	if parsed.SpeciesNameNormalized != nil {
		t.Fatalf("expected nil species normalized, got %q", *parsed.SpeciesNameNormalized)
	}
}

func TestParseCandidateFromOCRExtractsMunnaFromNoisyOCR(t *testing.T) {
	parsed := ParseCandidateFromOCR(`
00:47 4 | io
cp824
a»
Munna
141/141 HP
Check IV
This Munna was caught on 2/14/2026
around Olinda, PE, Brazil.
`)
	if parsed.SpeciesNameRaw == nil || parsed.SpeciesNameNormalized == nil {
		t.Fatal("expected extracted species values")
	}
	if *parsed.SpeciesNameRaw != "Munna" {
		t.Fatalf("expected species raw %q, got %q", "Munna", *parsed.SpeciesNameRaw)
	}
	if *parsed.SpeciesNameNormalized != "munna" {
		t.Fatalf("expected species normalized %q, got %q", "munna", *parsed.SpeciesNameNormalized)
	}
}

func TestParseCandidateFromOCRWithScorePrefersPlausibleSpecies(t *testing.T) {
	_, noisyScore := ParseCandidateFromOCRWithScore("DL. AKM")
	_, speciesScore := ParseCandidateFromOCRWithScore("Rhyhorn")

	if speciesScore <= noisyScore {
		t.Fatalf("expected species score (%d) > noisy score (%d)", speciesScore, noisyScore)
	}
}

func TestParseCPRawFromOCRParsesLabeledCP(t *testing.T) {
	parsed := ParseCPRawFromOCR("CP 824")
	if parsed == nil {
		t.Fatal("expected parsed CP value")
	}
	if *parsed != "824" {
		t.Fatalf("expected CP %q, got %q", "824", *parsed)
	}
}

func TestParseCPRawFromOCRParsesTrailingCPLabel(t *testing.T) {
	parsed := ParseCPRawFromOCR("1377 CP")
	if parsed == nil {
		t.Fatal("expected parsed CP value")
	}
	if *parsed != "1377" {
		t.Fatalf("expected CP %q, got %q", "1377", *parsed)
	}
}

func TestParseCPRawFromOCRCleansNoisyCharacters(t *testing.T) {
	parsed := ParseCPRawFromOCR("cp: I7O")
	if parsed == nil {
		t.Fatal("expected parsed CP value")
	}
	if *parsed != "170" {
		t.Fatalf("expected CP %q, got %q", "170", *parsed)
	}
}

func TestParseCPRawFromOCRUsesStandaloneNumericLineFallback(t *testing.T) {
	parsed := ParseCPRawFromOCR(`
???
824
`)
	if parsed == nil {
		t.Fatal("expected parsed CP value")
	}
	if *parsed != "824" {
		t.Fatalf("expected CP %q, got %q", "824", *parsed)
	}
}

func TestParseCPRawFromOCRRejectsInvalidNoise(t *testing.T) {
	parsed := ParseCPRawFromOCR(`
?? CP ??
This Munna was caught on 2/14/2026
`)
	if parsed != nil {
		t.Fatalf("expected nil CP parse, got %q", *parsed)
	}
}

func TestParseCPRawFromOCRRejectsZeroOnlyNoise(t *testing.T) {
	parsed := ParseCPRawFromOCR("CP OOO")
	if parsed != nil {
		t.Fatalf("expected nil CP parse, got %q", *parsed)
	}
}

func TestParseCPRawFromOCRRejectsSingleDigitNoise(t *testing.T) {
	parsed := ParseCPRawFromOCR("I")
	if parsed != nil {
		t.Fatalf("expected nil CP parse, got %q", *parsed)
	}
}

func TestParseCPRawFromOCRRejectsLongNoisyDigitRuns(t *testing.T) {
	parsed := ParseCPRawFromOCR("152168")
	if parsed != nil {
		t.Fatalf("expected nil CP parse, got %q", *parsed)
	}
}

func TestParseHPRawFromOCRParsesFractionWithLabel(t *testing.T) {
	parsed := ParseHPRawFromOCR("141/141 HP")
	if parsed == nil {
		t.Fatal("expected parsed HP value")
	}
	if *parsed != "141" {
		t.Fatalf("expected HP %q, got %q", "141", *parsed)
	}
}

func TestParseHPRawFromOCRParsesLeadingLabel(t *testing.T) {
	parsed := ParseHPRawFromOCR("HP I4I")
	if parsed == nil {
		t.Fatal("expected parsed HP value")
	}
	if *parsed != "141" {
		t.Fatalf("expected HP %q, got %q", "141", *parsed)
	}
}

func TestParseHPRawFromOCRParsesFractionFallbackWithoutLabel(t *testing.T) {
	parsed := ParseHPRawFromOCR("122 / 122")
	if parsed == nil {
		t.Fatal("expected parsed HP value")
	}
	if *parsed != "122" {
		t.Fatalf("expected HP %q, got %q", "122", *parsed)
	}
}

func TestParseHPRawFromOCRRejectsDateNoise(t *testing.T) {
	parsed := ParseHPRawFromOCR("This Munna was caught on 2/14/2026")
	if parsed != nil {
		t.Fatalf("expected nil HP parse, got %q", *parsed)
	}
}

func TestParseHPRawFromOCRRejectsOutOfRangeDigits(t *testing.T) {
	parsed := ParseHPRawFromOCR("1411/1411 HP")
	if parsed != nil {
		t.Fatalf("expected nil HP parse, got %q", *parsed)
	}
}

func TestParseIVRawFromOCRParsesLabeledValues(t *testing.T) {
	parsed := ParseIVRawFromOCR(`
Attack 15
Defense 14
HP 13
`)

	if parsed.AttackRaw == nil || *parsed.AttackRaw != "15" {
		t.Fatalf("expected attack iv %q, got %#v", "15", parsed.AttackRaw)
	}
	if parsed.DefenseRaw == nil || *parsed.DefenseRaw != "14" {
		t.Fatalf("expected defense iv %q, got %#v", "14", parsed.DefenseRaw)
	}
	if parsed.StaminaRaw == nil || *parsed.StaminaRaw != "13" {
		t.Fatalf("expected stamina iv %q, got %#v", "13", parsed.StaminaRaw)
	}
}

func TestParseIVRawFromOCRParsesTrailingLabelValues(t *testing.T) {
	parsed := ParseIVRawFromOCR("15 ATK 14 DEF 13 HP")
	if parsed.AttackRaw == nil || *parsed.AttackRaw != "15" {
		t.Fatalf("expected attack iv %q, got %#v", "15", parsed.AttackRaw)
	}
	if parsed.DefenseRaw == nil || *parsed.DefenseRaw != "14" {
		t.Fatalf("expected defense iv %q, got %#v", "14", parsed.DefenseRaw)
	}
	if parsed.StaminaRaw == nil || *parsed.StaminaRaw != "13" {
		t.Fatalf("expected stamina iv %q, got %#v", "13", parsed.StaminaRaw)
	}
}

func TestParseIVRawFromOCRParsesTripleFallback(t *testing.T) {
	parsed := ParseIVRawFromOCR(`
IV
15 / 14 / 13
`)

	if parsed.AttackRaw == nil || *parsed.AttackRaw != "15" {
		t.Fatalf("expected attack iv %q, got %#v", "15", parsed.AttackRaw)
	}
	if parsed.DefenseRaw == nil || *parsed.DefenseRaw != "14" {
		t.Fatalf("expected defense iv %q, got %#v", "14", parsed.DefenseRaw)
	}
	if parsed.StaminaRaw == nil || *parsed.StaminaRaw != "13" {
		t.Fatalf("expected stamina iv %q, got %#v", "13", parsed.StaminaRaw)
	}
}

func TestParseIVRawFromOCRCleansNoisyCharacters(t *testing.T) {
	parsed := ParseIVRawFromOCR("Atk I5 Def O9 HP l2")
	if parsed.AttackRaw == nil || *parsed.AttackRaw != "15" {
		t.Fatalf("expected attack iv %q, got %#v", "15", parsed.AttackRaw)
	}
	if parsed.DefenseRaw == nil || *parsed.DefenseRaw != "9" {
		t.Fatalf("expected defense iv %q, got %#v", "9", parsed.DefenseRaw)
	}
	if parsed.StaminaRaw == nil || *parsed.StaminaRaw != "12" {
		t.Fatalf("expected stamina iv %q, got %#v", "12", parsed.StaminaRaw)
	}
}

func TestParseIVRawFromOCRRejectsOutOfRangeValues(t *testing.T) {
	parsed := ParseIVRawFromOCR("Attack 20 Defense 14 HP 13")
	if parsed.AttackRaw != nil {
		t.Fatalf("expected nil attack iv for out-of-range value, got %#v", parsed.AttackRaw)
	}
	if parsed.DefenseRaw == nil || *parsed.DefenseRaw != "14" {
		t.Fatalf("expected defense iv %q, got %#v", "14", parsed.DefenseRaw)
	}
	if parsed.StaminaRaw == nil || *parsed.StaminaRaw != "13" {
		t.Fatalf("expected stamina iv %q, got %#v", "13", parsed.StaminaRaw)
	}
}

func TestResolveCanonicalSpeciesMatchPrefersExact(t *testing.T) {
	parsed := ParseCandidateFromOCR("Rhyhorn")
	match, ok := ResolveCanonicalSpeciesMatch(parsed, []string{"zacian", "rhyhorn", "nidoqueen"})
	if !ok {
		t.Fatal("expected canonical match")
	}

	if match.SpeciesNormalized != "rhyhorn" {
		t.Fatalf("expected exact match rhyhorn, got %q", match.SpeciesNormalized)
	}
	if match.Mode != "exact" {
		t.Fatalf("expected exact mode, got %q", match.Mode)
	}
	if match.Distance != 0 {
		t.Fatalf("expected distance 0, got %d", match.Distance)
	}
}

func TestResolveCanonicalSpeciesMatchUsesBoundedFuzzyMatch(t *testing.T) {
	parsed := ParseCandidateFromOCR("Nidoqveen")
	match, ok := ResolveCanonicalSpeciesMatch(parsed, []string{"rhyhorn", "nidoqueen", "zacian"})
	if !ok {
		t.Fatal("expected fuzzy canonical match")
	}

	if match.SpeciesNormalized != "nidoqueen" {
		t.Fatalf("expected nidoqueen fuzzy match, got %q", match.SpeciesNormalized)
	}
	if match.Mode != "fuzzy" {
		t.Fatalf("expected fuzzy mode, got %q", match.Mode)
	}
	if match.Distance <= 0 {
		t.Fatalf("expected non-zero distance for fuzzy match, got %d", match.Distance)
	}
}

func TestResolveCanonicalSpeciesMatchRejectsDistantNoise(t *testing.T) {
	parsed := ParseCandidateFromOCR("Qzxvtrnm")
	_, ok := ResolveCanonicalSpeciesMatch(parsed, []string{"rhyhorn", "nidoqueen", "zacian"})
	if ok {
		t.Fatal("expected no canonical match for distant noise")
	}
}

func TestResolveCanonicalSpeciesMatchUsesPrefixForFormNames(t *testing.T) {
	parsed := ParseCandidateFromOCR("Thundurus")
	match, ok := ResolveCanonicalSpeciesMatch(parsed, []string{"thundurus incarnate", "thundurus therian"})
	if !ok {
		t.Fatal("expected prefix canonical match")
	}
	if match.Mode != "prefix" {
		t.Fatalf("expected prefix mode, got %q", match.Mode)
	}
	if match.SpeciesNormalized != "thundurus incarnate" {
		t.Fatalf("expected first prefix canonical species, got %q", match.SpeciesNormalized)
	}
}

func TestResolveCanonicalSpeciesMatchRejectsFuzzyForThreeWordNoise(t *testing.T) {
	parsed := ParseCandidateFromOCR("Se En Ee")
	_, ok := ResolveCanonicalSpeciesMatch(parsed, []string{"steenee"})
	if ok {
		t.Fatal("expected no fuzzy match for fragmented three-word noise")
	}
}

func TestResolveCanonicalSpeciesMatchesMrMimeOrdersAndExcludesShadow(t *testing.T) {
	parsed := ParseCandidateFromOCR("Mr. Mime")
	matches := ResolveCanonicalSpeciesMatches(parsed, []string{
		"mr. mime (galarian)",
		"mr. mime (shadow)",
		"mr. mime",
	})

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (base + galarian), got %d", len(matches))
	}

	if matches[0].SpeciesNormalized != "mr. mime" || matches[0].Mode != "exact" || matches[0].Distance != 0 {
		t.Fatalf("expected first match to be exact Mr. Mime, got %#v", matches[0])
	}

	if matches[1].SpeciesNormalized != "mr. mime (galarian)" || matches[1].Mode != "prefix" || matches[1].Distance != 0 {
		t.Fatalf("expected second match to be prefix Mr. Mime (Galarian), got %#v", matches[1])
	}
}

func TestResolveCanonicalSpeciesMatchesDarumakaOrdersAndExcludesShadow(t *testing.T) {
	parsed := ParseCandidateFromOCR("Darumaka")
	matches := ResolveCanonicalSpeciesMatches(parsed, []string{
		"darumaka (shadow)",
		"darumaka (galarian)",
		"darumaka",
	})

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (base + galarian), got %d", len(matches))
	}

	if matches[0].SpeciesNormalized != "darumaka" || matches[0].Mode != "exact" || matches[0].Distance != 0 {
		t.Fatalf("expected first match to be exact Darumaka, got %#v", matches[0])
	}
	if matches[1].SpeciesNormalized != "darumaka (galarian)" || matches[1].Mode != "prefix" || matches[1].Distance != 0 {
		t.Fatalf("expected second match to be prefix Darumaka (Galarian), got %#v", matches[1])
	}
}
