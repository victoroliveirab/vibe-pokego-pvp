package appraisal

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var excludedSpeciesLineTokens = []string{
	"LUCKY POKEMON",
	"REMOTE TRADE",
	"ATTACK",
	"DEFENSE",
	"HP",
	"WEIGHT",
	"HEIGHT",
	"CANDY",
	"CHECK IV",
	"CAUGHT",
	"AROUND",
}

// ParsedCandidate contains normalized OCR outputs for a raw appraisal candidate.
type ParsedCandidate struct {
	SpeciesNameRaw        *string
	SpeciesNameNormalized *string
	CPRaw                 *string
	HPRaw                 *string
	IVAttackRaw           *string
	IVDefenseRaw          *string
	IVStaminaRaw          *string
}

// ParsedIVRaw contains parsed IV raw values for attack/defense/stamina.
type ParsedIVRaw struct {
	AttackRaw  *string
	DefenseRaw *string
	StaminaRaw *string
}

// CanonicalSpeciesMatch describes a catalog match for a parsed species candidate.
type CanonicalSpeciesMatch struct {
	SpeciesNormalized string
	Mode              string
	Distance          int
}

type canonicalScoredMatch struct {
	modeRank int
	match    CanonicalSpeciesMatch
}

// ParseCandidateFromOCR parses and normalizes OCR text for the current extraction phase.
func ParseCandidateFromOCR(speciesText string) ParsedCandidate {
	parsed, _ := ParseCandidateFromOCRWithScore(speciesText)
	return parsed
}

// ParseCandidateFromOCRWithScore parses OCR text and returns a relative quality score
// for the selected species-like line. Higher scores indicate a more plausible match.
func ParseCandidateFromOCRWithScore(speciesText string) (ParsedCandidate, int) {
	selectedLine, score := extractSpeciesLineWithScore(speciesText)
	raw := normalizeSpeciesRaw(selectedLine)
	if raw == "" {
		return ParsedCandidate{}, 0
	}

	normalized := strings.ToLower(raw)

	return ParsedCandidate{
		SpeciesNameRaw:        stringPtr(raw),
		SpeciesNameNormalized: stringPtr(normalized),
	}, max(score, 0)
}

var cpTokenPattern = regexp.MustCompile(`\d+`)
var hpFractionPattern = regexp.MustCompile(`(\d{1,4})\s*[/|]\s*(\d{1,4})`)
var ivTokenPattern = regexp.MustCompile(`\d{1,3}`)

var ivAttackLabels = []string{"ATTACK", "ATK"}
var ivDefenseLabels = []string{"DEFENSE", "DEFENCE", "DEF"}
var ivStaminaLabels = []string{"STAMINA", "STA", "HP"}

// ParseCPRawFromOCR parses OCR text from a CP-focused region and returns
// an integer-compatible CP token when available.
func ParseCPRawFromOCR(rawOCR string) *string {
	text := strings.ReplaceAll(rawOCR, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		candidate := parseCPFromLabeledLine(line)
		if candidate != "" {
			return stringPtr(candidate)
		}
	}

	for _, line := range lines {
		candidate := parseCPFromStandaloneLine(line)
		if candidate != "" {
			return stringPtr(candidate)
		}
	}

	return nil
}

// ParseHPRawFromOCR parses OCR text from an HP-focused region and returns
// the current HP token when available.
func ParseHPRawFromOCR(rawOCR string) *string {
	text := strings.ReplaceAll(rawOCR, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		candidate := parseHPFromLabeledLine(line)
		if candidate != "" {
			return stringPtr(candidate)
		}
	}

	for _, line := range lines {
		candidate := parseHPFromFractionLine(line)
		if candidate != "" {
			return stringPtr(candidate)
		}
	}

	for _, line := range lines {
		candidate := parseHPFromStandaloneLine(line)
		if candidate != "" {
			return stringPtr(candidate)
		}
	}

	return nil
}

// ParseIVRawFromOCR parses OCR text from IV-focused regions and returns
// integer-compatible IV tokens when available.
func ParseIVRawFromOCR(rawOCR string) ParsedIVRaw {
	text := strings.ReplaceAll(rawOCR, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	parsed := ParsedIVRaw{}

	for _, line := range lines {
		attack, defense, stamina := parseIVValueBeforeLabelLine(line)
		if attack == "" || defense == "" || stamina == "" {
			continue
		}
		parsed.AttackRaw = stringPtr(attack)
		parsed.DefenseRaw = stringPtr(defense)
		parsed.StaminaRaw = stringPtr(stamina)
		return parsed
	}

	for _, line := range lines {
		if parsed.AttackRaw == nil {
			if attack := parseIVFromLabeledLine(line, ivAttackLabels); attack != "" {
				parsed.AttackRaw = stringPtr(attack)
			}
		}
		if parsed.DefenseRaw == nil {
			if defense := parseIVFromLabeledLine(line, ivDefenseLabels); defense != "" {
				parsed.DefenseRaw = stringPtr(defense)
			}
		}
		if parsed.StaminaRaw == nil {
			if stamina := parseIVFromLabeledLine(line, ivStaminaLabels); stamina != "" {
				parsed.StaminaRaw = stringPtr(stamina)
			}
		}
	}

	if parsed.AttackRaw != nil && parsed.DefenseRaw != nil && parsed.StaminaRaw != nil {
		return parsed
	}

	for _, line := range lines {
		attack, defense, stamina := parseIVTripleLine(line)
		if attack == "" || defense == "" || stamina == "" {
			continue
		}

		if parsed.AttackRaw == nil {
			parsed.AttackRaw = stringPtr(attack)
		}
		if parsed.DefenseRaw == nil {
			parsed.DefenseRaw = stringPtr(defense)
		}
		if parsed.StaminaRaw == nil {
			parsed.StaminaRaw = stringPtr(stamina)
		}

		if parsed.AttackRaw != nil && parsed.DefenseRaw != nil && parsed.StaminaRaw != nil {
			return parsed
		}
	}

	return parsed
}

func parseCPFromLabeledLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	mappedLine := mapCPConfusableRunes(trimmed)
	upper := strings.ToUpper(mappedLine)
	cpIndex := strings.Index(upper, "CP")
	if cpIndex < 0 {
		return ""
	}

	match := cpTokenPattern.FindString(mappedLine[cpIndex+2:])
	if match == "" {
		beforeMatches := cpTokenPattern.FindAllString(mappedLine[:cpIndex], -1)
		if len(beforeMatches) == 0 {
			return ""
		}
		match = beforeMatches[len(beforeMatches)-1]
	}

	return normalizeCPDigits(match)
}

func parseCPFromStandaloneLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "/") {
		return ""
	}

	mapped := mapCPConfusableRunes(trimmed)
	if strings.TrimSpace(mapped) == "" {
		return ""
	}

	for _, r := range mapped {
		if unicode.IsLetter(r) {
			return ""
		}
	}

	matches := cpTokenPattern.FindAllString(mapped, -1)
	if len(matches) != 1 {
		return ""
	}

	return normalizeCPDigits(matches[0])
}

func parseHPFromLabeledLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	mappedLine := mapHPConfusableRunes(trimmed)
	upper := strings.ToUpper(mappedLine)
	hpIndex := strings.Index(upper, "HP")
	if hpIndex < 0 {
		return ""
	}

	if match := hpFractionPattern.FindStringSubmatch(mappedLine[hpIndex+2:]); len(match) == 3 {
		return normalizeHPDigits(match[1])
	}
	if match := hpFractionPattern.FindStringSubmatch(mappedLine[:hpIndex]); len(match) == 3 {
		return normalizeHPDigits(match[1])
	}

	if match := cpTokenPattern.FindString(mappedLine[hpIndex+2:]); match != "" {
		return normalizeHPDigits(match)
	}
	beforeMatches := cpTokenPattern.FindAllString(mappedLine[:hpIndex], -1)
	if len(beforeMatches) == 0 {
		return ""
	}

	return normalizeHPDigits(beforeMatches[len(beforeMatches)-1])
}

func parseHPFromFractionLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	mapped := mapHPConfusableRunes(trimmed)
	match := hpFractionPattern.FindStringSubmatch(mapped)
	if len(match) != 3 {
		return ""
	}

	return normalizeHPDigits(match[1])
}

func parseHPFromStandaloneLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "/") {
		return ""
	}

	mapped := mapHPConfusableRunes(trimmed)
	if strings.TrimSpace(mapped) == "" {
		return ""
	}
	for _, r := range mapped {
		if unicode.IsLetter(r) {
			return ""
		}
	}

	matches := cpTokenPattern.FindAllString(mapped, -1)
	if len(matches) != 1 {
		return ""
	}

	return normalizeHPDigits(matches[0])
}

func parseIVFromLabeledLine(line string, labels []string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	mapped := strings.ToUpper(mapHPConfusableRunes(trimmed))
	for _, label := range labels {
		searchStart := 0
		for searchStart < len(mapped) {
			labelIndex := strings.Index(mapped[searchStart:], label)
			if labelIndex < 0 {
				break
			}
			labelIndex += searchStart

			if match := ivTokenPattern.FindString(mapped[labelIndex+len(label):]); match != "" {
				if normalized := normalizeIVDigits(match); normalized != "" {
					return normalized
				}
			}

			beforeMatches := ivTokenPattern.FindAllString(mapped[:labelIndex], -1)
			if len(beforeMatches) > 0 {
				if normalized := normalizeIVDigits(beforeMatches[len(beforeMatches)-1]); normalized != "" {
					return normalized
				}
			}

			searchStart = labelIndex + len(label)
		}
	}

	return ""
}

func parseIVTripleLine(line string) (string, string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", ""
	}

	mapped := mapHPConfusableRunes(trimmed)
	matches := ivTokenPattern.FindAllString(mapped, -1)
	if len(matches) < 3 {
		return "", "", ""
	}

	parsed := make([]string, 0, 3)
	for _, match := range matches {
		value := normalizeIVDigits(match)
		if value == "" {
			continue
		}

		parsed = append(parsed, value)
		if len(parsed) == 3 {
			return parsed[0], parsed[1], parsed[2]
		}
	}

	return "", "", ""
}

func parseIVValueBeforeLabelLine(line string) (string, string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", "", ""
	}

	mapped := strings.ToUpper(mapHPConfusableRunes(trimmed))
	tokens := strings.FieldsFunc(mapped, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(tokens) < 2 {
		return "", "", ""
	}

	attack := ""
	defense := ""
	stamina := ""
	for idx := 0; idx+1 < len(tokens); idx++ {
		value := normalizeIVDigits(tokens[idx])
		if value == "" {
			continue
		}

		label := tokens[idx+1]
		switch {
		case attack == "" && isIVAttackLabel(label):
			attack = value
		case defense == "" && isIVDefenseLabel(label):
			defense = value
		case stamina == "" && isIVStaminaLabel(label):
			stamina = value
		}
	}

	return attack, defense, stamina
}

func isIVAttackLabel(label string) bool {
	switch label {
	case "ATTACK", "ATK":
		return true
	default:
		return false
	}
}

func isIVDefenseLabel(label string) bool {
	switch label {
	case "DEFENSE", "DEFENCE", "DEF":
		return true
	default:
		return false
	}
}

func isIVStaminaLabel(label string) bool {
	switch label {
	case "STAMINA", "STA", "HP":
		return true
	default:
		return false
	}
}

func mapCPConfusableRunes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		switch unicode.ToUpper(r) {
		case 'O', 'Q':
			builder.WriteRune('0')
		case 'I', 'L', '|', '!':
			builder.WriteRune('1')
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func mapHPConfusableRunes(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		switch unicode.ToUpper(r) {
		case 'O', 'Q':
			builder.WriteRune('0')
		case 'I', 'L', '|', '!':
			builder.WriteRune('1')
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func normalizeCPDigits(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}

	digits := builder.String()
	if digits == "" {
		return ""
	}

	// Keep at least one digit to preserve integer-compatibility.
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return ""
	}

	if len(digits) > 5 {
		return ""
	}
	if len(digits) < 2 {
		return ""
	}

	return digits
}

func normalizeHPDigits(value string) string {
	if value == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}

	digits := builder.String()
	if digits == "" {
		return ""
	}

	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return ""
	}

	if len(digits) > 3 {
		return ""
	}
	if len(digits) < 2 {
		return ""
	}

	return digits
}

func normalizeIVDigits(value string) string {
	if value == "" {
		return ""
	}

	mapped := mapHPConfusableRunes(value)
	var builder strings.Builder
	builder.Grow(len(mapped))
	for _, r := range mapped {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}

	digits := builder.String()
	if digits == "" {
		return ""
	}

	parsed, err := strconv.Atoi(digits)
	if err != nil {
		return ""
	}
	if parsed < 0 || parsed > 15 {
		return ""
	}

	return strconv.Itoa(parsed)
}

func extractSpeciesLine(rawOCR string) string {
	bestLine, _ := extractSpeciesLineWithScore(rawOCR)
	return bestLine
}

func extractSpeciesLineWithScore(rawOCR string) (string, int) {
	text := strings.ReplaceAll(rawOCR, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	bestScore := -1
	bestLine := ""

	for _, line := range lines {
		candidate := sanitizeSpeciesLine(line)
		if candidate == "" {
			continue
		}

		score := scoreSpeciesLine(candidate)
		if score > bestScore {
			bestScore = score
			bestLine = candidate
		}
	}

	return bestLine, bestScore
}

func normalizeSpeciesRaw(value string) string {
	trimmed := strings.Join(strings.Fields(value), " ")
	if trimmed == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(trimmed))

	newWord := true
	for _, r := range trimmed {
		switch {
		case unicode.IsLetter(r):
			if newWord {
				builder.WriteRune(unicode.ToUpper(r))
			} else {
				builder.WriteRune(unicode.ToLower(r))
			}
			newWord = false
		default:
			builder.WriteRune(r)
			newWord = unicode.IsSpace(r) || r == '-'
		}
	}

	return builder.String()
}

func sanitizeSpeciesLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(trimmed))

	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '.' || r == '\'' || r == '-' {
			builder.WriteRune(r)
		}
	}

	cleaned := strings.Join(strings.Fields(builder.String()), " ")
	if cleaned == "" {
		return ""
	}
	cleaned = trimNonLetterEdges(cleaned)
	if cleaned == "" {
		return ""
	}

	if strings.ContainsAny(cleaned, "0123456789") {
		return ""
	}

	letterCount := countLetters(cleaned)
	if letterCount < 2 {
		return ""
	}

	if !containsAnyLetter(cleaned) {
		return ""
	}

	return cleaned
}

func containsAnyLetter(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func scoreSpeciesLine(candidate string) int {
	score := len(candidate)
	score += countLetters(candidate) * 3

	wordCount := len(strings.Fields(candidate))
	switch {
	case wordCount == 1:
		score += 15
	case wordCount == 2:
		score += 8
	case wordCount > 3:
		score -= 20
	}

	upper := strings.ToUpper(candidate)
	for _, excluded := range excludedSpeciesLineTokens {
		if strings.Contains(upper, excluded) {
			score -= 100
		}
	}

	return score
}

func trimNonLetterEdges(value string) string {
	runes := []rune(value)
	start := 0
	for start < len(runes) && !unicode.IsLetter(runes[start]) {
		start++
	}

	end := len(runes) - 1
	for end >= start && !unicode.IsLetter(runes[end]) {
		end--
	}

	if start > end {
		return ""
	}

	return string(runes[start : end+1])
}

func countLetters(value string) int {
	count := 0
	for _, r := range value {
		if unicode.IsLetter(r) {
			count++
		}
	}

	return count
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// ResolveCanonicalSpeciesMatch resolves a parsed species to a canonical catalog entry.
// It prioritizes exact matches and falls back to bounded fuzzy matching.
func ResolveCanonicalSpeciesMatch(parsed ParsedCandidate, catalogSpecies []string) (CanonicalSpeciesMatch, bool) {
	matches := ResolveCanonicalSpeciesMatches(parsed, catalogSpecies)
	if len(matches) == 0 {
		return CanonicalSpeciesMatch{}, false
	}

	return matches[0], true
}

// ResolveCanonicalSpeciesMatches resolves a parsed species to canonical catalog entries.
// Matches are returned in deterministic rank order: exact, prefix, fuzzy, then distance,
// then lexical species name. Shadow forms are excluded.
func ResolveCanonicalSpeciesMatches(parsed ParsedCandidate, catalogSpecies []string) []CanonicalSpeciesMatch {
	if parsed.SpeciesNameNormalized == nil {
		return nil
	}
	if len(catalogSpecies) == 0 {
		return nil
	}

	candidate := strings.TrimSpace(*parsed.SpeciesNameNormalized)
	if candidate == "" {
		return nil
	}

	bestBySpecies := make(map[string]canonicalScoredMatch, len(catalogSpecies))
	consider := func(canonical string, mode string, distance int) {
		if isShadowCanonicalSpecies(canonical) {
			return
		}

		modeRank := canonicalModeSortRank(mode)
		if modeRank < 0 {
			return
		}

		current, exists := bestBySpecies[canonical]
		if exists {
			if current.modeRank < modeRank {
				return
			}
			if current.modeRank == modeRank && current.match.Distance <= distance {
				return
			}
		}

		bestBySpecies[canonical] = canonicalScoredMatch{
			modeRank: modeRank,
			match: CanonicalSpeciesMatch{
				SpeciesNormalized: canonical,
				Mode:              mode,
				Distance:          distance,
			},
		}
	}

	for _, canonical := range catalogSpecies {
		if canonical == candidate {
			consider(canonical, "exact", 0)
		}
	}

	// Accept canonical form variants where OCR returns the base species token only
	// (for example "thundurus" vs "thundurus incarnate").
	for _, canonical := range catalogSpecies {
		if strings.HasPrefix(canonical, candidate+" ") {
			consider(canonical, "prefix", 0)
		}
	}

	candidateKey := canonicalComparisonKey(candidate)
	if candidateKey == "" {
		return nil
	}
	candidateWords := len(strings.Fields(candidate))
	if candidateWords > 2 || len(candidateKey) < 5 {
		matches := collectCanonicalMatches(bestBySpecies)
		sortCanonicalMatches(matches)
		return matches
	}

	for _, canonical := range catalogSpecies {
		canonicalKey := canonicalComparisonKey(canonical)
		if canonicalKey == "" {
			continue
		}

		distance := levenshteinDistance(candidateKey, canonicalKey)
		threshold := fuzzyDistanceThreshold(max(len(candidateKey), len(canonicalKey)))
		if distance > threshold {
			continue
		}
		consider(canonical, "fuzzy", distance)
	}

	matches := collectCanonicalMatches(bestBySpecies)
	sortCanonicalMatches(matches)
	return matches
}

func collectCanonicalMatches(bestBySpecies map[string]canonicalScoredMatch) []CanonicalSpeciesMatch {
	if len(bestBySpecies) == 0 {
		return nil
	}

	matches := make([]CanonicalSpeciesMatch, 0, len(bestBySpecies))
	for _, candidate := range bestBySpecies {
		matches = append(matches, candidate.match)
	}
	return matches
}

func sortCanonicalMatches(matches []CanonicalSpeciesMatch) {
	sort.SliceStable(matches, func(i int, j int) bool {
		left := matches[i]
		right := matches[j]

		leftRank := canonicalModeSortRank(left.Mode)
		rightRank := canonicalModeSortRank(right.Mode)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if left.Distance != right.Distance {
			return left.Distance < right.Distance
		}

		return left.SpeciesNormalized < right.SpeciesNormalized
	})
}

func canonicalModeSortRank(mode string) int {
	switch mode {
	case "exact":
		return 0
	case "prefix":
		return 1
	case "fuzzy":
		return 2
	default:
		return -1
	}
}

func isShadowCanonicalSpecies(canonical string) bool {
	return strings.Contains(strings.ToLower(canonical), "(shadow)")
}

func canonicalComparisonKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func fuzzyDistanceThreshold(tokenLength int) int {
	switch {
	case tokenLength <= 5:
		return 1
	case tokenLength <= 9:
		return 2
	default:
		return 3
	}
}

func levenshteinDistance(a string, b string) int {
	ar := []rune(a)
	br := []rune(b)

	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}

	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := 0; j <= len(br); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}

			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min(del, min(ins, sub))
		}
		prev, curr = curr, prev
	}

	return prev[len(br)]
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
