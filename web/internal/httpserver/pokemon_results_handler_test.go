package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestPokemonResultsHandlerMethodNotAllowed(t *testing.T) {
	handler := newPokemonResultsHandler(&fakePokemonResultsHandlerStore{})
	req := newPokemonResultsHandlerRequest(http.MethodPost, "/pokemon", "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
}

func TestPokemonResultsHandlerReturnsMappedPayloadForVideoAndImageResults(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"

	levelEstimate := 23.5
	levelConfidence := 0.72
	startMS := int64(12000)
	endMS := int64(15500)
	frameTimestampMS := int64(13200)
	extractionConfidence := 0.86
	bestLevelGreat := 23.5
	bestLevelUltra := 39.0

	createdAtVideo := time.Date(2026, time.March, 5, 10, 12, 5, 0, time.UTC)
	createdAtImage := createdAtVideo.Add(time.Second)

	store := &fakePokemonResultsHandlerStore{
		listFn: func(_ context.Context, gotSessionID string) ([]upload.PokemonResultRecord, error) {
			if gotSessionID != sessionID {
				t.Fatalf("expected session id %q, got %q", sessionID, gotSessionID)
			}

			return []upload.PokemonResultRecord{
				{
					ID:                   "result-video",
					JobID:                "job-video",
					UploadID:             "upload-video",
					SessionID:            sessionID,
					SpeciesName:          "Machop",
					CP:                   512,
					HP:                   64,
					PowerUpStardustCost:  2500,
					IVAttack:             12,
					IVDefense:            15,
					IVStamina:            13,
					LevelEstimate:        &levelEstimate,
					LevelConfidence:      &levelConfidence,
					LevelMethod:          "ARC_POSITION",
					SourceType:           "VIDEO",
					StartMS:              &startMS,
					EndMS:                &endMS,
					FrameTimestampMS:     &frameTimestampMS,
					ExtractionConfidence: &extractionConfidence,
					MaxCPEvaluations: []upload.PokemonResultMaxCPEvaluationRecord{
						{
							MaxCP:              1500,
							EvaluatedSpeciesID: "machoke",
							BestLevel:          bestLevelGreat,
							BestCP:             1499,
							StatProduct:        1567890.12,
							Rank:               143,
							Percentage:         93.32,
						},
						{
							MaxCP:              2500,
							EvaluatedSpeciesID: "machamp",
							BestLevel:          bestLevelUltra,
							BestCP:             2498,
							StatProduct:        2789012.34,
							Rank:               98,
							Percentage:         96.11,
						},
					},
					CreatedAt: createdAtVideo,
				},
				{
					ID:                  "result-image",
					JobID:               "job-image",
					UploadID:            "upload-image",
					SessionID:           sessionID,
					SpeciesName:         "Pikachu",
					CP:                  410,
					HP:                  64,
					PowerUpStardustCost: 3000,
					IVAttack:            10,
					IVDefense:           12,
					IVStamina:           11,
					LevelMethod:         "UNKNOWN",
					SourceType:          "IMAGE",
					CreatedAt:           createdAtImage,
				},
			}, nil
		},
	}

	handler := newPokemonResultsHandler(store)
	req := newPokemonResultsHandlerRequest(http.MethodGet, "/pokemon", sessionID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload pokemonResultsEnvelopeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}

	if len(payload.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(payload.Results))
	}

	first := payload.Results[0]
	if first.ID != "result-video" {
		t.Fatalf("expected first result id %q, got %q", "result-video", first.ID)
	}
	if first.SpeciesName != "Machop" {
		t.Fatalf("expected species %q, got %q", "Machop", first.SpeciesName)
	}
	if first.CP != 512 || first.HP != 64 || first.PowerUpStardustCost != 2500 {
		t.Fatalf("unexpected scalar payload values: %#v", first)
	}
	if first.IVs.Attack != 12 || first.IVs.Defense != 15 || first.IVs.Stamina != 13 {
		t.Fatalf("unexpected IV payload values: %#v", first.IVs)
	}
	if first.Level.Method != "ARC_POSITION" {
		t.Fatalf("expected level.method ARC_POSITION, got %q", first.Level.Method)
	}
	if first.Level.Estimate == nil || *first.Level.Estimate != levelEstimate {
		t.Fatalf("expected level.estimate %v, got %#v", levelEstimate, first.Level.Estimate)
	}
	if first.Level.Confidence == nil || *first.Level.Confidence != levelConfidence {
		t.Fatalf("expected level.confidence %v, got %#v", levelConfidence, first.Level.Confidence)
	}
	if first.Source.Type != "VIDEO" || first.Source.UploadID != "upload-video" || first.Source.JobID != "job-video" {
		t.Fatalf("unexpected source metadata: %#v", first.Source)
	}
	if first.Source.TimeRangeMS.Start == nil || *first.Source.TimeRangeMS.Start != startMS {
		t.Fatalf("expected source.timeRangeMs.start %d, got %#v", startMS, first.Source.TimeRangeMS.Start)
	}
	if first.Source.TimeRangeMS.End == nil || *first.Source.TimeRangeMS.End != endMS {
		t.Fatalf("expected source.timeRangeMs.end %d, got %#v", endMS, first.Source.TimeRangeMS.End)
	}
	if first.Source.FrameTimestampMS == nil || *first.Source.FrameTimestampMS != frameTimestampMS {
		t.Fatalf("expected source.frameTimestampMs %d, got %#v", frameTimestampMS, first.Source.FrameTimestampMS)
	}
	if first.Confidence == nil || *first.Confidence != extractionConfidence {
		t.Fatalf("expected confidence %v, got %#v", extractionConfidence, first.Confidence)
	}
	if first.CreatedAt != createdAtVideo.Format(time.RFC3339Nano) {
		t.Fatalf("expected createdAt %q, got %q", createdAtVideo.Format(time.RFC3339Nano), first.CreatedAt)
	}
	if len(first.MaxCPEvaluations) != 2 {
		t.Fatalf("expected 2 max cp evaluations, got %d", len(first.MaxCPEvaluations))
	}
	if first.MaxCPEvaluations[0].MaxCP != 1500 || first.MaxCPEvaluations[0].EvaluatedSpeciesID != "machoke" {
		t.Fatalf("unexpected first max cp evaluation payload: %#v", first.MaxCPEvaluations[0])
	}
	if first.MaxCPEvaluations[0].BestLevel != bestLevelGreat || first.MaxCPEvaluations[0].BestCP != 1499 {
		t.Fatalf("unexpected first max cp level/cp payload: %#v", first.MaxCPEvaluations[0])
	}
	if first.MaxCPEvaluations[0].Rank != 143 || first.MaxCPEvaluations[0].Percentage != 93.32 {
		t.Fatalf("unexpected first max cp rank/percentage payload: %#v", first.MaxCPEvaluations[0])
	}
	if first.MaxCPEvaluations[1].MaxCP != 2500 || first.MaxCPEvaluations[1].EvaluatedSpeciesID != "machamp" {
		t.Fatalf("unexpected second max cp evaluation payload: %#v", first.MaxCPEvaluations[1])
	}
	if first.MaxCPEvaluations[1].BestLevel != bestLevelUltra || first.MaxCPEvaluations[1].BestCP != 2498 {
		t.Fatalf("unexpected second max cp level/cp payload: %#v", first.MaxCPEvaluations[1])
	}
	if first.MaxCPEvaluations[1].Rank != 98 || first.MaxCPEvaluations[1].Percentage != 96.11 {
		t.Fatalf("unexpected second max cp rank/percentage payload: %#v", first.MaxCPEvaluations[1])
	}

	second := payload.Results[1]
	if second.ID != "result-image" {
		t.Fatalf("expected second result id %q, got %q", "result-image", second.ID)
	}
	if second.Source.Type != "IMAGE" {
		t.Fatalf("expected source.type IMAGE, got %q", second.Source.Type)
	}
	if second.Level.Estimate != nil || second.Level.Confidence != nil {
		t.Fatalf("expected null level estimate/confidence, got %#v", second.Level)
	}
	if second.Source.TimeRangeMS.Start != nil || second.Source.TimeRangeMS.End != nil {
		t.Fatalf("expected null source.timeRangeMs, got %#v", second.Source.TimeRangeMS)
	}
	if second.Source.FrameTimestampMS != nil {
		t.Fatalf("expected null frameTimestampMs, got %#v", second.Source.FrameTimestampMS)
	}
	if second.Confidence != nil {
		t.Fatalf("expected null confidence, got %#v", second.Confidence)
	}
	if second.CreatedAt != createdAtImage.Format(time.RFC3339Nano) {
		t.Fatalf("expected createdAt %q, got %q", createdAtImage.Format(time.RFC3339Nano), second.CreatedAt)
	}
	if len(second.MaxCPEvaluations) != 0 {
		t.Fatalf("expected empty max cp evaluations for second result, got %#v", second.MaxCPEvaluations)
	}
}

func TestPokemonResultsHandlerReturnsEmptyResultsList(t *testing.T) {
	handler := newPokemonResultsHandler(&fakePokemonResultsHandlerStore{
		listFn: func(context.Context, string) ([]upload.PokemonResultRecord, error) {
			return []upload.PokemonResultRecord{}, nil
		},
	})

	req := newPokemonResultsHandlerRequest(http.MethodGet, "/pokemon", "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload pokemonResultsEnvelopeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload.Results == nil {
		t.Fatal("expected results list to be non-nil")
	}
	if len(payload.Results) != 0 {
		t.Fatalf("expected empty results list, got %d", len(payload.Results))
	}
}

func TestPokemonResultsHandlerReturnsInternalErrorWhenSessionMissingFromContext(t *testing.T) {
	handler := newPokemonResultsHandler(&fakePokemonResultsHandlerStore{
		listFn: func(context.Context, string) ([]upload.PokemonResultRecord, error) {
			t.Fatal("expected store not to be called when session context is missing")
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/pokemon", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR code, got %q", payload.Error.Code)
	}
}

func TestPokemonResultsHandlerReturnsInternalErrorWhenStoreFails(t *testing.T) {
	handler := newPokemonResultsHandler(&fakePokemonResultsHandlerStore{
		listFn: func(context.Context, string) ([]upload.PokemonResultRecord, error) {
			return nil, errors.New("db unavailable")
		},
	})

	req := newPokemonResultsHandlerRequest(http.MethodGet, "/pokemon", "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR code, got %q", payload.Error.Code)
	}
}

type fakePokemonResultsHandlerStore struct {
	listFn func(ctx context.Context, sessionID string) ([]upload.PokemonResultRecord, error)
}

func (s *fakePokemonResultsHandlerStore) CreateUploadAndQueuedJob(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
	return upload.Upload{}, upload.Job{}, errors.New("not implemented")
}

func (s *fakePokemonResultsHandlerStore) CreateRetryJob(context.Context, string, string, time.Time) (upload.RetryJob, error) {
	return upload.RetryJob{}, errors.New("not implemented")
}

func (s *fakePokemonResultsHandlerStore) GetJobStatus(context.Context, string, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, errors.New("not implemented")
}

func (s *fakePokemonResultsHandlerStore) ListPokemonResultsBySession(
	ctx context.Context,
	sessionID string,
) ([]upload.PokemonResultRecord, error) {
	if s.listFn != nil {
		return s.listFn(ctx, sessionID)
	}

	return nil, nil
}

func (s *fakePokemonResultsHandlerStore) ListPendingReadingsBySession(
	context.Context,
	string,
) ([]upload.PendingSpeciesReadingRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakePokemonResultsHandlerStore) ResolvePendingReading(
	context.Context,
	upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	return upload.PokemonResultRecord{}, errors.New("not implemented")
}

func newPokemonResultsHandlerRequest(method string, path string, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: sessionID})
	return req.WithContext(ctx)
}
