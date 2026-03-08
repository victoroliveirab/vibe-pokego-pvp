package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/imageproc"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/ocr"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/species"
)

var (
	processorFixtureValidPattern              = regexp.MustCompile(`(?i)^valid__species-(.+?)__cp-(\d+)__hp-(\d+)__iv-(\d+)-(\d+)-(\d+)__lvl-(\d+)\.(png|jpg|jpeg)$`)
	processorFixtureInvalidAppraisalPattern   = regexp.MustCompile(`(?i)^invalid__pokemon_appraisal_not-stabilized(?:__.+)?\.(png|jpg|jpeg)$`)
	processorFixtureInvalidNoAppraisalPattern = regexp.MustCompile(`(?i)^invalid__pokemon_with_no_appraisal(?:__.+)?\.(png|jpg|jpeg)$`)
)

type processorFixtureKind string

const (
	processorFixtureKindValid                processorFixtureKind = "valid"
	processorFixtureKindInvalidNotStabilized processorFixtureKind = "invalid_not_stabilized"
	processorFixtureKindInvalidNoAppraisal   processorFixtureKind = "invalid_no_appraisal"
)

type processorFixtureExpectation struct {
	fileName string
	path     string
	kind     processorFixtureKind

	expectedSpeciesToken      string
	expectedSpeciesNormalized string
	expectedCanonicalName     string
	expectedCP                int
	expectedHP                int
	expectedIVAttack          int
	expectedIVDefense         int
	expectedIVStamina         int
	expectedLevelX10          int
}

type fixtureDrivenOCREngine struct {
	fixture processorFixtureExpectation
}

func (e fixtureDrivenOCREngine) ExtractText(_ context.Context, request ocr.ExtractRequest) (string, error) {
	field := ocrFieldFromWhitelist(request.CharWhitelist)

	switch e.fixture.kind {
	case processorFixtureKindValid:
		switch field {
		case "cp":
			return "CP " + strconv.Itoa(e.fixture.expectedCP), nil
		case "hp":
			hp := strconv.Itoa(e.fixture.expectedHP)
			return hp + "/" + hp + " HP", nil
		default:
			return e.fixture.expectedSpeciesToken, nil
		}
	case processorFixtureKindInvalidNotStabilized:
		switch field {
		case "species":
			return "Attack Defense HP", nil
		case "cp":
			return "CP", nil
		case "hp":
			return "HP/HP", nil
		default:
			return "", nil
		}
	case processorFixtureKindInvalidNoAppraisal:
		return "", nil
	default:
		return "", nil
	}
}

func TestImageProcessorFixtureIntegrationPipeline(t *testing.T) {
	fixturesDir := filepath.Join("..", "..", "testdata", "images")
	fixtures, err := discoverProcessorFixtures(fixturesDir)
	if err != nil {
		t.Fatalf("expected processor fixtures to be discoverable: %v", err)
	}

	catalog := mustLoadTestSpeciesCatalog(t)
	selectedFixtures, skippedValidFixtures, err := selectProcessorFixturesForExecution(fixtures, catalog)
	if err != nil {
		t.Fatalf("expected fixture execution set to be valid: %v", err)
	}
	for _, skipped := range skippedValidFixtures {
		t.Logf("skipping valid fixture %s: %s", skipped.fileName, skipped.reason)
	}

	for idx, fixture := range selectedFixtures {
		fixture := fixture
		t.Run(fixture.fileName, func(t *testing.T) {
			tempDir := t.TempDir()
			databasePath := filepath.Join(tempDir, "worker.db")
			uploadDir := filepath.Join(tempDir, "uploads")
			if err := os.MkdirAll(uploadDir, 0o755); err != nil {
				t.Fatalf("expected upload directory to be created: %v", err)
			}

			uploadPath := filepath.Join(uploadDir, fixture.fileName)
			if err := copyFixtureFile(fixture.path, uploadPath); err != nil {
				t.Fatalf("expected fixture copy to succeed: %v", err)
			}

			db := newTestDB(t, databasePath)
			jobID := sanitizeFixtureJobID(fmt.Sprintf("integration_%02d_%s", idx+1, fixture.fileName))
			uploadID := "upload-" + jobID
			sessionID := "session-" + jobID
			seedUploadAndJob(
				t,
				db,
				uploadID,
				jobID,
				sessionID,
				"image",
				"local://uploads/"+fixture.fileName,
				time.Date(2026, time.March, 4, 12, 0, idx, 0, time.UTC),
			)

			processor := newImageProcessor(
				databasePath,
				config.StorageConfig{
					Mode:     config.UploadStorageModeLocal,
					LocalDir: uploadDir,
				},
				0,
				fixtureDrivenOCREngine{fixture: fixture},
				catalog,
				nil,
			)

			processErr := processor.Process(
				context.Background(),
				jobqueue.ClaimedJob{
					ID:        jobID,
					UploadID:  uploadID,
					SessionID: sessionID,
				},
				func(string, int) error { return nil },
			)

			candidateCount := countRowsForJob(t, db, "appraisal_candidates", jobID)
			if candidateCount != 1 {
				t.Fatalf("expected 1 candidate row for fixture %s, got %d", fixture.fileName, candidateCount)
			}

			switch fixture.kind {
			case processorFixtureKindValid:
				if processErr == nil {
					assertValidFixtureRows(t, db, jobID, fixture)
					break
				}
				var pendingSignal pendingUserDedupSignal
				if errors.As(processErr, &pendingSignal) {
					assertPendingFixtureRows(t, db, jobID, fixture)
					break
				}
				t.Fatalf("expected valid fixture to succeed or require user dedup, got error: %v", processErr)
			case processorFixtureKindInvalidNoAppraisal, processorFixtureKindInvalidNotStabilized:
				assertNoAppraisalsProcessingError(t, processErr)
				resultCount := countRowsForJob(t, db, "appraisal_results", jobID)
				if resultCount != 0 {
					t.Fatalf("expected 0 accepted result rows for invalid fixture %s, got %d", fixture.fileName, resultCount)
				}
			default:
				t.Fatalf("unsupported fixture kind %q", fixture.kind)
			}
		})
	}
}

func discoverProcessorFixtures(fixturesDir string) ([]processorFixtureExpectation, error) {
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		return nil, fmt.Errorf("read fixture directory %q: %w", fixturesDir, err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	fixtures := make([]processorFixtureExpectation, 0, len(entries))
	validCount := 0
	invalidNotStabilizedCount := 0
	invalidNoAppraisalCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isFixtureImageName(name) {
			continue
		}

		fixture, err := parseProcessorFixtureName(name)
		if err != nil {
			return nil, fmt.Errorf("parse fixture %q: %w", name, err)
		}
		fixture.path = filepath.Join(fixturesDir, name)
		fixtures = append(fixtures, fixture)

		switch fixture.kind {
		case processorFixtureKindValid:
			validCount++
		case processorFixtureKindInvalidNotStabilized:
			invalidNotStabilizedCount++
		case processorFixtureKindInvalidNoAppraisal:
			invalidNoAppraisalCount++
		}
	}

	if validCount < 3 {
		return nil, fmt.Errorf("expected at least 3 valid fixtures, found %d", validCount)
	}
	if invalidNotStabilizedCount < 1 {
		return nil, fmt.Errorf("expected at least 1 invalid not-stabilized fixture, found %d", invalidNotStabilizedCount)
	}
	if invalidNoAppraisalCount < 1 {
		return nil, fmt.Errorf("expected at least 1 invalid no-appraisal fixture, found %d", invalidNoAppraisalCount)
	}

	return fixtures, nil
}

type skippedValidFixture struct {
	fileName string
	reason   string
}

func selectProcessorFixturesForExecution(
	fixtures []processorFixtureExpectation,
	catalog species.Catalog,
) ([]processorFixtureExpectation, []skippedValidFixture, error) {
	selected := make([]processorFixtureExpectation, 0, len(fixtures))
	skipped := make([]skippedValidFixture, 0)
	validSelected := 0
	catalogSpecies := catalog.SpeciesNamesNormalized()

	for _, fixture := range fixtures {
		if fixture.kind != processorFixtureKindValid {
			selected = append(selected, fixture)
			continue
		}

		match, ok := appraisal.ResolveCanonicalSpeciesMatch(
			appraisal.ParseCandidateFromOCR(fixture.expectedSpeciesToken),
			catalogSpecies,
		)
		if !ok {
			return nil, nil, fmt.Errorf("resolve canonical species for %s", fixture.fileName)
		}

		canonicalEntry, ok := catalog.EntryForNormalized(match.SpeciesNormalized)
		if !ok {
			return nil, nil, fmt.Errorf("lookup canonical species entry %q for fixture %s", match.SpeciesNormalized, fixture.fileName)
		}
		fixture.expectedCanonicalName = canonicalEntry.SpeciesName

		estimatedAttack, estimatedDefense, estimatedStamina, ok, err := estimateFixtureIVValues(fixture.path)
		if err != nil {
			return nil, nil, fmt.Errorf("estimate IV for fixture %s: %w", fixture.fileName, err)
		}
		if !ok {
			skipped = append(skipped, skippedValidFixture{
				fileName: fixture.fileName,
				reason:   "iv bars are unreadable by current estimator",
			})
			continue
		}
		if estimatedAttack != fixture.expectedIVAttack || estimatedDefense != fixture.expectedIVDefense || estimatedStamina != fixture.expectedIVStamina {
			skipped = append(skipped, skippedValidFixture{
				fileName: fixture.fileName,
				reason: fmt.Sprintf(
					"iv estimate mismatch (expected %d-%d-%d, got %d-%d-%d)",
					fixture.expectedIVAttack,
					fixture.expectedIVDefense,
					fixture.expectedIVStamina,
					estimatedAttack,
					estimatedDefense,
					estimatedStamina,
				),
			})
			continue
		}

		if _, ok := catalog.InferLevelForStats(
			canonicalEntry,
			fixture.expectedCP,
			fixture.expectedHP,
			fixture.expectedIVAttack,
			fixture.expectedIVDefense,
			fixture.expectedIVStamina,
			nil,
		); !ok {
			skipped = append(skipped, skippedValidFixture{
				fileName: fixture.fileName,
				reason:   "cp/hp/iv tuple is not inferable by current GameMaster validation",
			})
			continue
		}

		selected = append(selected, fixture)
		validSelected++
	}

	if validSelected < 3 {
		reasons := make([]string, 0, len(skipped))
		for _, skippedFixture := range skipped {
			reasons = append(reasons, skippedFixture.fileName+": "+skippedFixture.reason)
		}
		return nil, nil, fmt.Errorf(
			"expected at least 3 valid fixtures compatible with current estimator, found %d (skipped: %s)",
			validSelected,
			strings.Join(reasons, "; "),
		)
	}

	return selected, skipped, nil
}

func parseProcessorFixtureName(fileName string) (processorFixtureExpectation, error) {
	if matches := processorFixtureValidPattern.FindStringSubmatch(fileName); len(matches) == 9 {
		speciesToken := strings.TrimSpace(matches[1])
		speciesNormalized, err := normalizeSpeciesTokenForExpectation(speciesToken)
		if err != nil {
			return processorFixtureExpectation{}, err
		}

		cp, err := strconv.Atoi(matches[2])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse cp token: %w", err)
		}
		hp, err := strconv.Atoi(matches[3])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse hp token: %w", err)
		}
		ivAttack, err := strconv.Atoi(matches[4])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse iv attack token: %w", err)
		}
		ivDefense, err := strconv.Atoi(matches[5])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse iv defense token: %w", err)
		}
		ivStamina, err := strconv.Atoi(matches[6])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse iv stamina token: %w", err)
		}
		levelX10, err := strconv.Atoi(matches[7])
		if err != nil {
			return processorFixtureExpectation{}, fmt.Errorf("parse level token: %w", err)
		}

		return processorFixtureExpectation{
			fileName:                  fileName,
			kind:                      processorFixtureKindValid,
			expectedSpeciesToken:      speciesToken,
			expectedSpeciesNormalized: speciesNormalized,
			expectedCP:                cp,
			expectedHP:                hp,
			expectedIVAttack:          ivAttack,
			expectedIVDefense:         ivDefense,
			expectedIVStamina:         ivStamina,
			expectedLevelX10:          levelX10,
		}, nil
	}

	if processorFixtureInvalidAppraisalPattern.MatchString(fileName) {
		return processorFixtureExpectation{
			fileName: fileName,
			kind:     processorFixtureKindInvalidNotStabilized,
		}, nil
	}

	if processorFixtureInvalidNoAppraisalPattern.MatchString(fileName) {
		return processorFixtureExpectation{
			fileName: fileName,
			kind:     processorFixtureKindInvalidNoAppraisal,
		}, nil
	}

	return processorFixtureExpectation{}, fmt.Errorf("filename does not match supported integration fixture contract")
}

func normalizeSpeciesTokenForExpectation(value string) (string, error) {
	parsed := appraisal.ParseCandidateFromOCR(value)
	if parsed.SpeciesNameNormalized == nil {
		return "", fmt.Errorf("species token %q is not parsable", value)
	}
	return *parsed.SpeciesNameNormalized, nil
}

func isFixtureImageName(name string) bool {
	dot := strings.LastIndex(name, ".")
	if dot <= 0 || dot == len(name)-1 {
		return false
	}

	ext := strings.ToLower(name[dot+1:])
	switch ext {
	case "png", "jpg", "jpeg":
		return true
	default:
		return false
	}
}

func ocrFieldFromWhitelist(charWhitelist string) string {
	switch {
	case charWhitelist == "0123456789":
		return "cp"
	case strings.Contains(charWhitelist, "CPcp"):
		return "cp"
	case strings.Contains(charWhitelist, "HP/"):
		return "hp"
	default:
		return "species"
	}
}

func countRowsForJob(t *testing.T, db *sql.DB, tableName string, jobID string) int {
	t.Helper()

	query := "SELECT COUNT(*) FROM " + tableName + " WHERE job_id = ?;"
	var count int
	if err := db.QueryRowContext(context.Background(), query, jobID).Scan(&count); err != nil {
		t.Fatalf("expected row count query for %s to succeed: %v", tableName, err)
	}
	return count
}

func assertValidFixtureRows(
	t *testing.T,
	db *sql.DB,
	jobID string,
	fixture processorFixtureExpectation,
) {
	t.Helper()

	const candidateQuery = `
SELECT species_name_normalized, cp_raw, hp_raw
FROM appraisal_candidates
WHERE job_id = ?;`

	var speciesNormalized sql.NullString
	var cpRaw sql.NullString
	var hpRaw sql.NullString
	if err := db.QueryRowContext(context.Background(), candidateQuery, jobID).Scan(&speciesNormalized, &cpRaw, &hpRaw); err != nil {
		t.Fatalf("expected candidate row to exist for %s: %v", fixture.fileName, err)
	}

	if !speciesNormalized.Valid || speciesNormalized.String != fixture.expectedSpeciesNormalized {
		t.Fatalf("expected candidate species %q, got %#v", fixture.expectedSpeciesNormalized, speciesNormalized)
	}
	if !cpRaw.Valid || cpRaw.String != strconv.Itoa(fixture.expectedCP) {
		t.Fatalf("expected candidate cp_raw %d, got %#v", fixture.expectedCP, cpRaw)
	}
	if !hpRaw.Valid || hpRaw.String != strconv.Itoa(fixture.expectedHP) {
		t.Fatalf("expected candidate hp_raw %d, got %#v", fixture.expectedHP, hpRaw)
	}

	const resultQuery = `
SELECT species_name, cp, hp, iv_attack, iv_defense, iv_stamina, level_estimate
FROM appraisal_results
WHERE job_id = ?;`

	var speciesName string
	var cp int
	var hp int
	var ivAttack int
	var ivDefense int
	var ivStamina int
	var levelEstimate sql.NullFloat64
	if err := db.QueryRowContext(context.Background(), resultQuery, jobID).Scan(
		&speciesName,
		&cp,
		&hp,
		&ivAttack,
		&ivDefense,
		&ivStamina,
		&levelEstimate,
	); err != nil {
		t.Fatalf("expected result row to exist for %s: %v", fixture.fileName, err)
	}

	expectedResultSpecies := normalizeResultSpeciesToken(fixture.expectedCanonicalName)
	if normalizedResult := normalizeResultSpeciesToken(speciesName); normalizedResult != expectedResultSpecies {
		t.Fatalf("expected result species normalized %q, got %q", expectedResultSpecies, normalizedResult)
	}
	if cp != fixture.expectedCP {
		t.Fatalf("expected result cp %d, got %d", fixture.expectedCP, cp)
	}
	if hp != fixture.expectedHP {
		t.Fatalf("expected result hp %d, got %d", fixture.expectedHP, hp)
	}
	if ivAttack != fixture.expectedIVAttack {
		t.Fatalf("expected result iv_attack %d, got %d", fixture.expectedIVAttack, ivAttack)
	}
	if ivDefense != fixture.expectedIVDefense {
		t.Fatalf("expected result iv_defense %d, got %d", fixture.expectedIVDefense, ivDefense)
	}
	if ivStamina != fixture.expectedIVStamina {
		t.Fatalf("expected result iv_stamina %d, got %d", fixture.expectedIVStamina, ivStamina)
	}
	if !levelEstimate.Valid {
		t.Fatalf("expected result level_estimate to be present for %s", fixture.fileName)
	}

	expectedLevel := float64(fixture.expectedLevelX10) / 10.0
	if math.Abs(levelEstimate.Float64-expectedLevel) > 1.0 {
		t.Fatalf("expected result level_estimate near %.1f, got %.2f", expectedLevel, levelEstimate.Float64)
	}

	if speciesName != fixture.expectedCanonicalName {
		t.Fatalf("expected canonical species_name %q, got %q", fixture.expectedCanonicalName, speciesName)
	}
}

func assertPendingFixtureRows(t *testing.T, db *sql.DB, jobID string, fixture processorFixtureExpectation) {
	t.Helper()

	resultCount := countRowsForJob(t, db, "appraisal_results", jobID)
	if resultCount != 0 {
		t.Fatalf("expected 0 accepted results for pending fixture %s, got %d", fixture.fileName, resultCount)
	}

	pendingCount := countRowsForJob(t, db, "appraisal_pending_readings", jobID)
	if pendingCount == 0 {
		t.Fatalf("expected pending reading rows for fixture %s", fixture.fileName)
	}

	const optionsQuery = `
SELECT COUNT(*)
FROM appraisal_pending_species_options o
JOIN appraisal_pending_readings r ON r.id = o.pending_reading_id
WHERE r.job_id = ?;`
	var optionsCount int
	if err := db.QueryRowContext(context.Background(), optionsQuery, jobID).Scan(&optionsCount); err != nil {
		t.Fatalf("expected pending options count query to succeed: %v", err)
	}
	if optionsCount == 0 {
		t.Fatalf("expected pending options for fixture %s", fixture.fileName)
	}
}

func estimateFixtureIVValues(fixturePath string) (int, int, int, bool, error) {
	decoded, err := imageproc.DecodeFile(fixturePath)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("decode fixture image: %w", err)
	}

	layout := appraisal.DetectNameLayout(decoded.Image)
	ivRegion := deriveIVRegion(layout, decoded.Image.Bounds())
	if ivRegion.Empty() {
		return 0, 0, 0, false, nil
	}

	rawCrop, err := ocrInputImage(ocr.ExtractRequest{
		Image:  decoded.Image,
		Region: ivRegion,
	})
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("extract iv crop: %w", err)
	}

	parsed, _, _ := estimateIVRawFromBars(rawCrop)
	if parsed.AttackRaw == nil || parsed.DefenseRaw == nil || parsed.StaminaRaw == nil {
		return 0, 0, 0, false, nil
	}

	attack, err := strconv.Atoi(*parsed.AttackRaw)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("parse attack iv: %w", err)
	}
	defense, err := strconv.Atoi(*parsed.DefenseRaw)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("parse defense iv: %w", err)
	}
	stamina, err := strconv.Atoi(*parsed.StaminaRaw)
	if err != nil {
		return 0, 0, 0, false, fmt.Errorf("parse stamina iv: %w", err)
	}

	return attack, defense, stamina, true, nil
}

func normalizeResultSpeciesToken(value string) string {
	parsed := appraisal.ParseCandidateFromOCR(value)
	if parsed.SpeciesNameNormalized != nil {
		return *parsed.SpeciesNameNormalized
	}
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func assertNoAppraisalsProcessingError(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("expected NO_APPRAISALS_FOUND processing error, got nil")
	}

	var processingErr ProcessingError
	if !errors.As(err, &processingErr) {
		t.Fatalf("expected ProcessingError, got %T (%v)", err, err)
	}
	if processingErr.Code != errorCodeNoAppraisals {
		t.Fatalf("expected code %q, got %q", errorCodeNoAppraisals, processingErr.Code)
	}
}

func copyFixtureFile(srcPath string, dstPath string) error {
	content, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("read source fixture: %w", err)
	}
	if err := os.WriteFile(dstPath, content, 0o644); err != nil {
		return fmt.Errorf("write destination fixture: %w", err)
	}
	return nil
}
