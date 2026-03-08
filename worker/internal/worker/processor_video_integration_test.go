package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/jobqueue"
)

func TestVideoFixtureIntegrationPersistsExpectedTerminalOutcomes(t *testing.T) {
	fixturesDir := filepath.Join("..", "..", "testdata", "videos")

	testCases := []struct {
		name                    string
		fixture                 string
		frames                  []processedFrame
		expectAccepted          int
		expectPending           bool
		expectPendingRows       int
		expectPendingOptionRows int
		expectOutcome           string
	}{
		{
			name:    "dedup-identical-frames",
			fixture: "valid_short_1s.mp4",
			frames: []processedFrame{
				makeVideoProcessedFrame("Darumaka", 712, 120, 10, 11, 12, 0, false),
				makeVideoProcessedFrame("Darumaka", 712, 120, 10, 11, 12, 300, false),
			},
			expectAccepted: 1,
			expectPending:  false,
			expectOutcome:  "success",
		},
		{
			name:    "distinct-appraisals",
			fixture: "valid_video_6s_9s.mp4",
			frames: []processedFrame{
				makeVideoProcessedFrame("Darumaka", 712, 120, 10, 11, 12, 0, false),
				makeVideoProcessedFrame("Munna", 824, 141, 0, 13, 13, 300, false),
			},
			expectAccepted: 2,
			expectPending:  false,
			expectOutcome:  "success",
		},
		{
			name:    "ambiguous-species",
			fixture: "valid_video_20s_24s.mp4",
			frames: []processedFrame{
				makeVideoProcessedFrame("Darumaka", 712, 120, 10, 11, 12, 300, true),
				makeVideoProcessedFrame("Darumaka", 712, 120, 10, 11, 12, 600, true),
			},
			expectAccepted:          0,
			expectPending:           true,
			expectPendingRows:       1,
			expectPendingOptionRows: 2,
			expectOutcome:           "pending",
		},
		{
			name:           "no-appraisal-found",
			fixture:        "corrupt.mp4",
			frames:         []processedFrame{},
			expectAccepted: 0,
			expectPending:  false,
			expectOutcome:  "no_appraisals",
		},
	}

	for idx, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(fixturesDir, tc.fixture)); err != nil {
				t.Fatalf("expected video fixture %q to exist: %v", tc.fixture, err)
			}

			tempDir := t.TempDir()
			databasePath := filepath.Join(tempDir, "worker.db")
			db := newTestDB(t, databasePath)
			jobID := "job-video-integration-" + strings.ReplaceAll(tc.name, "_", "-")
			seedUploadAndJob(
				t,
				db,
				"upload-"+jobID,
				jobID,
				"session-"+jobID,
				"video",
				"local://uploads/"+tc.fixture,
				time.Date(2026, time.March, 6, 17, 0, idx, 0, time.UTC),
			)

			processor := imageProcessor{databasePath: databasePath}
			hasPending, acceptedCount, err := processor.persistVideoFrames(
				context.Background(),
				jobqueue.ClaimedJob{
					ID:        jobID,
					UploadID:  "upload-" + jobID,
					SessionID: "session-" + jobID,
				},
				tc.frames,
			)
			if err != nil {
				t.Fatalf("expected video frame persistence to succeed: %v", err)
			}

			if hasPending != tc.expectPending {
				t.Fatalf("expected hasPending=%v, got %v", tc.expectPending, hasPending)
			}
			if acceptedCount != tc.expectAccepted {
				t.Fatalf("expected acceptedCount=%d, got %d", tc.expectAccepted, acceptedCount)
			}

			outcomeErr := finalizeProcessingOutcome(hasPending, acceptedCount)
			switch tc.expectOutcome {
			case "success":
				if outcomeErr != nil {
					t.Fatalf("expected successful terminal outcome, got %v", outcomeErr)
				}
			case "pending":
				var pendingSignal pendingUserDedupSignal
				if !errors.As(outcomeErr, &pendingSignal) {
					t.Fatalf("expected pending-user-dedup terminal outcome, got %v", outcomeErr)
				}
			case "no_appraisals":
				assertNoAppraisalsProcessingError(t, outcomeErr)
			default:
				t.Fatalf("unsupported expected outcome %q", tc.expectOutcome)
			}

			assertTableRowCount(t, db, "appraisal_candidates", len(tc.frames))
			assertTableRowCount(t, db, "appraisal_results", tc.expectAccepted)
			assertTableRowCount(t, db, "appraisal_pending_readings", tc.expectPendingRows)
			assertTableRowCount(t, db, "appraisal_pending_species_options", tc.expectPendingOptionRows)
		})
	}
}

func makeVideoProcessedFrame(
	speciesName string,
	cp int,
	hp int,
	ivAttack int,
	ivDefense int,
	ivStamina int,
	timestampMS int64,
	ambiguous bool,
) processedFrame {
	speciesRaw := speciesName
	speciesNormalized := strings.ToLower(speciesName)
	cpRaw := strconvFromInt(cp)
	hpRaw := strconvFromInt(hp)
	ivAttackRaw := strconvFromInt(ivAttack)
	ivDefenseRaw := strconvFromInt(ivDefense)
	ivStaminaRaw := strconvFromInt(ivStamina)

	accepted := []appraisal.AcceptedResultCandidate{
		makeAcceptedResultCandidate(speciesName, cp, hp, ivAttack, ivDefense, ivStamina),
	}
	if ambiguous {
		accepted = append(accepted, makeAcceptedResultCandidate("Darumaka (Galarian)", cp, hp, ivAttack, ivDefense, ivStamina))
	}

	return processedFrame{
		parsed: appraisal.ParsedCandidate{
			SpeciesNameRaw:        &speciesRaw,
			SpeciesNameNormalized: &speciesNormalized,
			CPRaw:                 &cpRaw,
			HPRaw:                 &hpRaw,
			IVAttackRaw:           &ivAttackRaw,
			IVDefenseRaw:          &ivDefenseRaw,
			IVStaminaRaw:          &ivStaminaRaw,
		},
		rawOCRText: speciesName,
		validation: appraisal.ValidationDecision{
			SpeciesIsCanonical: true,
			AcceptedResults:    accepted,
		},
		timestampMS: int64Ptr(timestampMS),
	}
}

func strconvFromInt(value int) string {
	return strconv.Itoa(value)
}
