package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/pvp"
)

func TestPVPEvalRunnerProcessQueueSucceedsAndPersistsAllFamilyCaps(t *testing.T) {
	now := time.Date(2026, time.March, 8, 15, 0, 0, 0, time.UTC)
	store := &fakePVPEvalStore{
		claimItems: []appraisal.PvPEvaluationQueueItem{
			{
				ID:                "queue-1",
				AppraisalResultID: "result-1",
				Status:            appraisal.PvPEvalQueueStatusProcessing,
			},
		},
		resultsByID: map[string]appraisal.Result{
			"result-1": {
				ID:          "result-1",
				SpeciesName: "Bulbasaur",
				IVAttack:    1,
				IVDefense:   2,
				IVStamina:   3,
			},
		},
	}

	graph := &fakeForwardGraph{
		canonical: map[string]string{"Bulbasaur": "bulbasaur"},
		families: map[string][]string{
			"bulbasaur": {"bulbasaur", "ivysaur"},
		},
	}

	runner := newPVPEvalRunner(
		store,
		fakeMaxCPEvaluator{},
		graph,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	runner.nowFn = func() time.Time { return now }

	processed, err := runner.ProcessQueue(context.Background(), 10, now)
	if err != nil {
		t.Fatalf("expected queue processing to succeed, got: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 succeeded queue item, got %d", processed)
	}
	if len(store.upsertCalls) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(store.upsertCalls))
	}
	if len(store.upsertCalls[0].evaluations) != 6 {
		t.Fatalf("expected 6 evaluations (2 species x 3 caps), got %d", len(store.upsertCalls[0].evaluations))
	}
	if len(store.markSucceededIDs) != 1 || store.markSucceededIDs[0] != "queue-1" {
		t.Fatalf("expected queue-1 to be marked succeeded, got %#v", store.markSucceededIDs)
	}
	if len(store.markFailedCalls) != 0 {
		t.Fatalf("expected no failed marks, got %#v", store.markFailedCalls)
	}
}

func TestPVPEvalRunnerProcessQueueMarksFailedWithRetryWhenEvaluationErrors(t *testing.T) {
	now := time.Date(2026, time.March, 8, 16, 0, 0, 0, time.UTC)
	store := &fakePVPEvalStore{
		claimItems: []appraisal.PvPEvaluationQueueItem{
			{
				ID:                "queue-2",
				AppraisalResultID: "result-2",
				Status:            appraisal.PvPEvalQueueStatusProcessing,
				RetryCount:        2,
			},
		},
		resultsByID: map[string]appraisal.Result{
			"result-2": {
				ID:          "result-2",
				SpeciesName: "Bulbasaur",
				IVAttack:    1,
				IVDefense:   2,
				IVStamina:   3,
			},
		},
	}

	graph := &fakeForwardGraph{
		canonical: map[string]string{"Bulbasaur": "bulbasaur"},
		families: map[string][]string{
			"bulbasaur": {"bulbasaur", "ivysaur"},
		},
	}

	runner := newPVPEvalRunner(
		store,
		fakeMaxCPEvaluator{failSpeciesID: "ivysaur"},
		graph,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	runner.nowFn = func() time.Time { return now }

	processed, err := runner.ProcessQueue(context.Background(), 10, now)
	if err != nil {
		t.Fatalf("expected queue processing to handle item-level errors, got: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 succeeded queue items, got %d", processed)
	}
	if len(store.markSucceededIDs) != 0 {
		t.Fatalf("expected no succeeded marks, got %#v", store.markSucceededIDs)
	}
	if len(store.markFailedCalls) != 1 {
		t.Fatalf("expected one failed mark, got %#v", store.markFailedCalls)
	}
	failed := store.markFailedCalls[0]
	if failed.queueItemID != "queue-2" {
		t.Fatalf("expected failed queue id %q, got %q", "queue-2", failed.queueItemID)
	}
	if failed.retryCount != 3 {
		t.Fatalf("expected retry_count to increment to 3, got %d", failed.retryCount)
	}
	if failed.nextRetryAt == nil || !failed.nextRetryAt.After(now) {
		t.Fatalf("expected next_retry_at to be set in the future, got %#v", failed.nextRetryAt)
	}
}

func TestPVPEvalRunnerProcessQueueMarksFailedWhenSpeciesResolutionFails(t *testing.T) {
	now := time.Date(2026, time.March, 8, 17, 0, 0, 0, time.UTC)
	store := &fakePVPEvalStore{
		claimItems: []appraisal.PvPEvaluationQueueItem{
			{
				ID:                "queue-3",
				AppraisalResultID: "result-3",
				Status:            appraisal.PvPEvalQueueStatusProcessing,
			},
		},
		resultsByID: map[string]appraisal.Result{
			"result-3": {
				ID:          "result-3",
				SpeciesName: "Definitely Not A Pokemon",
				IVAttack:    1,
				IVDefense:   2,
				IVStamina:   3,
			},
		},
	}

	runner := newPVPEvalRunner(
		store,
		fakeMaxCPEvaluator{},
		&fakeForwardGraph{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	runner.nowFn = func() time.Time { return now }

	processed, err := runner.ProcessQueue(context.Background(), 10, now)
	if err != nil {
		t.Fatalf("expected queue processing to continue on species resolution failure, got: %v", err)
	}
	if processed != 0 {
		t.Fatalf("expected 0 succeeded queue items, got %d", processed)
	}
	if len(store.markFailedCalls) != 1 {
		t.Fatalf("expected one failed mark, got %#v", store.markFailedCalls)
	}
}

type fakePVPEvalStore struct {
	claimItems  []appraisal.PvPEvaluationQueueItem
	resultsByID map[string]appraisal.Result

	upsertCalls      []fakeUpsertCall
	markSucceededIDs []string
	markFailedCalls  []fakeMarkFailedCall
}

type fakeUpsertCall struct {
	appraisalResultID string
	evaluations       []appraisal.UpsertPvPEvaluationParams
}

type fakeMarkFailedCall struct {
	queueItemID string
	retryCount  int
	lastError   string
	nextRetryAt *time.Time
}

func (f *fakePVPEvalStore) ClaimPvPEvaluationQueueItems(
	_ context.Context,
	_ int,
	_ time.Time,
) ([]appraisal.PvPEvaluationQueueItem, error) {
	items := append([]appraisal.PvPEvaluationQueueItem(nil), f.claimItems...)
	f.claimItems = nil
	return items, nil
}

func (f *fakePVPEvalStore) GetResultByID(
	_ context.Context,
	resultID string,
) (appraisal.Result, bool, error) {
	row, ok := f.resultsByID[resultID]
	return row, ok, nil
}

func (f *fakePVPEvalStore) UpsertResultPvPEvaluations(
	_ context.Context,
	appraisalResultID string,
	evaluations []appraisal.UpsertPvPEvaluationParams,
	_ time.Time,
) error {
	f.upsertCalls = append(f.upsertCalls, fakeUpsertCall{
		appraisalResultID: appraisalResultID,
		evaluations:       append([]appraisal.UpsertPvPEvaluationParams(nil), evaluations...),
	})
	return nil
}

func (f *fakePVPEvalStore) MarkPvPEvaluationQueueItemSucceeded(
	_ context.Context,
	queueItemID string,
	_ time.Time,
) (bool, error) {
	f.markSucceededIDs = append(f.markSucceededIDs, queueItemID)
	return true, nil
}

func (f *fakePVPEvalStore) MarkPvPEvaluationQueueItemFailed(
	_ context.Context,
	queueItemID string,
	retryCount int,
	lastError string,
	nextRetryAt *time.Time,
	_ time.Time,
) (bool, error) {
	f.markFailedCalls = append(f.markFailedCalls, fakeMarkFailedCall{
		queueItemID: queueItemID,
		retryCount:  retryCount,
		lastError:   lastError,
		nextRetryAt: nextRetryAt,
	})
	return true, nil
}

type fakeMaxCPEvaluator struct {
	failSpeciesID string
}

func (f fakeMaxCPEvaluator) EvaluateMaxCP(
	speciesID string,
	ivAttack int,
	ivDefense int,
	ivStamina int,
	maxCP int,
) (pvp.MaxCPEvaluation, error) {
	if f.failSpeciesID != "" && f.failSpeciesID == speciesID {
		return pvp.MaxCPEvaluation{}, fmt.Errorf("forced evaluation error for %s", speciesID)
	}

	return pvp.MaxCPEvaluation{
		MaxCP:              maxCP,
		EvaluatedSpeciesID: speciesID,
		BestLevel:          float64(maxCP) / 100.0,
		BestCP:             maxCP - 1,
		StatProduct:        float64((ivAttack + ivDefense + ivStamina) * maxCP),
		Rank:               1,
		Percentage:         100,
	}, nil
}

type fakeForwardGraph struct {
	canonical  map[string]string
	normalized map[string]string
	families   map[string][]string
}

func (f *fakeForwardGraph) SpeciesIDForCanonicalName(speciesName string) (string, bool) {
	if f.canonical == nil {
		return "", false
	}
	value, ok := f.canonical[speciesName]
	return value, ok
}

func (f *fakeForwardGraph) SpeciesIDForNormalizedName(normalized string) (string, bool) {
	if f.normalized == nil {
		return "", false
	}
	value, ok := f.normalized[normalized]
	return value, ok
}

func (f *fakeForwardGraph) ForwardFamily(speciesID string) []string {
	if f.families == nil {
		return nil
	}
	family := f.families[speciesID]
	if len(family) == 0 {
		return nil
	}
	out := make([]string, len(family))
	copy(out, family)
	return out
}
