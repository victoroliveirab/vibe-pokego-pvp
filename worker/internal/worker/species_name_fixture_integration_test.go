package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/imageproc"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/ocr"
)

var validFixtureNamePattern = regexp.MustCompile(`(?i)^valid__species-(.+?)__cp-\d+__hp-\d+__iv-\d+-\d+-\d+__lvl-\d+\.(png|jpg|jpeg)$`)

func TestSpeciesNameExtractionFromValidFixtures(t *testing.T) {
	if os.Getenv("RUN_SPECIES_FIXTURE_SUITE") != "1" {
		t.Skip("set RUN_SPECIES_FIXTURE_SUITE=1 to run full OCR fixture species suite")
	}

	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skipf("tesseract not available in PATH: %v", err)
	}

	fixturesDir := filepath.Join("..", "..", "testdata", "images")
	entries, err := os.ReadDir(fixturesDir)
	if err != nil {
		t.Fatalf("expected fixture directory %s to be readable: %v", fixturesDir, err)
	}

	type fixtureCase struct {
		fileName           string
		path               string
		expectedNormalized string
	}

	fixtures := make([]fixtureCase, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), "valid__") {
			continue
		}

		expected, err := expectedSpeciesFromValidFixtureName(entry.Name())
		if err != nil {
			t.Fatalf("expected valid fixture name to parse (%s): %v", entry.Name(), err)
		}

		fixtures = append(fixtures, fixtureCase{
			fileName:           entry.Name(),
			path:               filepath.Join(fixturesDir, entry.Name()),
			expectedNormalized: expected,
		})
	}

	if len(fixtures) == 0 {
		t.Fatalf("expected at least one valid fixture under %s", fixturesDir)
	}

	sort.Slice(fixtures, func(i, j int) bool {
		return fixtures[i].fileName < fixtures[j].fileName
	})

	tempDir := t.TempDir()
	processor := imageProcessor{
		databasePath:   filepath.Join(tempDir, "worker.db"),
		ocrEngine:      ocr.NewTesseractEngine(),
		speciesCatalog: mustLoadTestSpeciesCatalog(t),
	}

	for index, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.fileName, func(t *testing.T) {
			decoded, err := imageproc.DecodeFile(fixture.path)
			if err != nil {
				t.Fatalf("expected fixture decode to succeed: %v", err)
			}

			preprocessed, err := imageproc.PreprocessForOCR(decoded.Image)
			if err != nil {
				t.Fatalf("expected fixture preprocess to succeed: %v", err)
			}

			jobID := sanitizeFixtureJobID(fmt.Sprintf("%02d_%s", index+1, fixture.fileName))
			artifactWriter, err := newImageArtifactWriter(processor.databasePath, jobID)
			if err != nil {
				t.Fatalf("expected artifact writer creation to succeed: %v", err)
			}

			rawText, err := processor.extractSpeciesName(context.Background(), decoded.Image, preprocessed, artifactWriter)
			if err != nil {
				t.Fatalf("expected species extraction to succeed: %v", err)
			}

			parsed := appraisal.ParseCandidateFromOCR(rawText)
			if parsed.SpeciesNameNormalized == nil {
				t.Fatalf("expected normalized species for fixture %s, raw OCR text: %q", fixture.fileName, rawText)
			}
			if *parsed.SpeciesNameNormalized != fixture.expectedNormalized {
				t.Fatalf("expected normalized species %q, got %q (raw OCR: %q)", fixture.expectedNormalized, *parsed.SpeciesNameNormalized, rawText)
			}
		})
	}
}

func expectedSpeciesFromValidFixtureName(fileName string) (string, error) {
	matches := validFixtureNamePattern.FindStringSubmatch(fileName)
	if len(matches) != 3 {
		return "", fmt.Errorf("filename does not match valid fixture contract")
	}

	token := strings.TrimSpace(matches[1])
	parsed := appraisal.ParseCandidateFromOCR(token)
	if parsed.SpeciesNameNormalized == nil {
		return "", fmt.Errorf("species token %q is not parsable", token)
	}

	return *parsed.SpeciesNameNormalized, nil
}

func sanitizeFixtureJobID(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}

	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "fixture"
	}

	return result
}
