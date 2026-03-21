package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestPokemonPendingSpeciesHandlerMethodNotAllowed(t *testing.T) {
	handler := newPokemonPendingSpeciesHandler(&fakePokemonPendingSpeciesStore{})
	req := newPokemonPendingSpeciesHandlerRequest(http.MethodPost, "/pokemon/pending-species", "session-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
}

func TestPokemonPendingSpeciesHandlerReturnsMappedPayload(t *testing.T) {
	createdAt := time.Date(2026, time.March, 6, 13, 0, 0, 0, time.UTC)
	levelEstimate := 23.5
	levelConfidence := 0.72
	frameTimestamp := int64(300)
	confidence := 0.86
	handler := newPokemonPendingSpeciesHandler(&fakePokemonPendingSpeciesStore{
		listPendingFn: func(_ context.Context, gotOwnerKey string) ([]upload.PendingSpeciesReadingRecord, error) {
			if gotOwnerKey != "session-1" {
				t.Fatalf("expected owner key %q, got %q", "session-1", gotOwnerKey)
			}

			return []upload.PendingSpeciesReadingRecord{
				{
					ID:                   "reading-1",
					JobID:                "job-1",
					UploadID:             "upload-1",
					SessionID:            "session-1",
					CP:                   712,
					HP:                   120,
					IVAttack:             10,
					IVDefense:            11,
					IVStamina:            12,
					LevelEstimate:        &levelEstimate,
					LevelConfidence:      &levelConfidence,
					LevelMethod:          "ARC_POSITION",
					SourceType:           "VIDEO",
					FrameTimestampMS:     &frameTimestamp,
					ExtractionConfidence: &confidence,
					Status:               upload.JobStatusPendingUserDedup,
					CreatedAt:            createdAt,
					Options: []upload.PendingSpeciesOptionRecord{
						{ID: "option-1", SpeciesName: "Darumaka", MatchMode: "exact", MatchDistance: 0, OptionRank: 1},
						{ID: "option-2", SpeciesName: "Darumaka (Galarian)", MatchMode: "fuzzy", MatchDistance: 1, OptionRank: 2},
					},
				},
			}, nil
		},
	})

	req := newPokemonPendingSpeciesHandlerRequest(http.MethodGet, "/pokemon/pending-species", "session-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload pokemonPendingSpeciesEnvelopeResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected payload decode to succeed, got %v", err)
	}

	if len(payload.Readings) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(payload.Readings))
	}
	reading := payload.Readings[0]
	if reading.ID != "reading-1" || reading.JobID != "job-1" || reading.UploadID != "upload-1" {
		t.Fatalf("unexpected reading identity payload: %#v", reading)
	}
	if reading.CP != 712 || reading.HP != 120 {
		t.Fatalf("unexpected reading stats payload: %#v", reading)
	}
	if reading.IVs.Attack != 10 || reading.IVs.Defense != 11 || reading.IVs.Stamina != 12 {
		t.Fatalf("unexpected reading iv payload: %#v", reading.IVs)
	}
	if reading.Level.Method != "ARC_POSITION" {
		t.Fatalf("expected level method ARC_POSITION, got %q", reading.Level.Method)
	}
	if reading.Level.Estimate == nil || *reading.Level.Estimate != levelEstimate {
		t.Fatalf("expected level estimate %v, got %#v", levelEstimate, reading.Level.Estimate)
	}
	if reading.Level.Confidence == nil || *reading.Level.Confidence != levelConfidence {
		t.Fatalf("expected level confidence %v, got %#v", levelConfidence, reading.Level.Confidence)
	}
	if reading.Source.Type != "VIDEO" {
		t.Fatalf("expected source type VIDEO, got %q", reading.Source.Type)
	}
	if reading.Source.FrameTimestampMS == nil || *reading.Source.FrameTimestampMS != frameTimestamp {
		t.Fatalf("expected frame timestamp %d, got %#v", frameTimestamp, reading.Source.FrameTimestampMS)
	}
	if reading.Confidence == nil || *reading.Confidence != confidence {
		t.Fatalf("expected confidence %v, got %#v", confidence, reading.Confidence)
	}
	if reading.Status != upload.JobStatusPendingUserDedup {
		t.Fatalf("expected status %q, got %q", upload.JobStatusPendingUserDedup, reading.Status)
	}
	if reading.CreatedAt != createdAt.Format(time.RFC3339Nano) {
		t.Fatalf("expected createdAt %q, got %q", createdAt.Format(time.RFC3339Nano), reading.CreatedAt)
	}
	if len(reading.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(reading.Options))
	}
	if reading.Options[0].ID != "option-1" || reading.Options[0].OptionRank != 1 {
		t.Fatalf("unexpected option payload: %#v", reading.Options[0])
	}
}

func TestPokemonPendingSpeciesHandlerReturnsInternalErrorWhenSessionMissing(t *testing.T) {
	handler := newPokemonPendingSpeciesHandler(&fakePokemonPendingSpeciesStore{
		listPendingFn: func(context.Context, string) ([]upload.PendingSpeciesReadingRecord, error) {
			t.Fatal("expected store not to be called")
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/pokemon/pending-species", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestPokemonPendingSpeciesResolveHandlerMethodNotAllowed(t *testing.T) {
	handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{}, time.Now)
	req := newPokemonPendingSpeciesResolveHandlerRequest(http.MethodGet, "/pokemon/pending-species/reading-1", "reading-1", "session-1", "{}")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPatch {
		t.Fatalf("expected Allow %q, got %q", http.MethodPatch, allow)
	}
}

func TestPokemonPendingSpeciesResolveHandlerReturnsBadRequestForInvalidPayload(t *testing.T) {
	handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{}, time.Now)
	req := newPokemonPendingSpeciesResolveHandlerRequest(
		http.MethodPatch,
		"/pokemon/pending-species/reading-1",
		"reading-1",
		"session-1",
		"{",
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPokemonPendingSpeciesResolveHandlerReturnsBadRequestForMissingOptionID(t *testing.T) {
	handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{}, time.Now)
	req := newPokemonPendingSpeciesResolveHandlerRequest(
		http.MethodPatch,
		"/pokemon/pending-species/reading-1",
		"reading-1",
		"session-1",
		`{"optionId":" "}`,
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPokemonPendingSpeciesResolveHandlerMapsDomainErrors(t *testing.T) {
	now := time.Date(2026, time.March, 6, 13, 30, 0, 0, time.UTC)
	testCases := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{name: "reading not found", err: upload.ErrPendingReadingNotFound, expectedStatus: http.StatusNotFound, expectedCode: "READING_NOT_FOUND"},
		{name: "reading locked", err: upload.ErrPendingReadingLocked, expectedStatus: http.StatusConflict, expectedCode: "READING_LOCKED"},
		{name: "option not found", err: upload.ErrPendingOptionNotFound, expectedStatus: http.StatusNotFound, expectedCode: "OPTION_NOT_FOUND"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{
				resolvePendingFn: func(_ context.Context, params upload.ResolvePendingReadingParams) (upload.PokemonResultRecord, error) {
					if params.ReadingID != "reading-1" || params.OptionID != "option-1" || params.OwnerKey != "session-1" {
						t.Fatalf("unexpected resolve params: %#v", params)
					}
					if !params.Now.Equal(now) {
						t.Fatalf("expected now %s, got %s", now, params.Now)
					}
					return upload.PokemonResultRecord{}, tc.err
				},
			}, func() time.Time { return now })

			req := newPokemonPendingSpeciesResolveHandlerRequest(
				http.MethodPatch,
				"/pokemon/pending-species/reading-1",
				"reading-1",
				"session-1",
				`{"optionId":"option-1"}`,
			)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Fatalf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}
			var payload APIError
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatalf("expected API error payload, got: %v", err)
			}
			if payload.Error.Code != tc.expectedCode {
				t.Fatalf("expected error code %q, got %q", tc.expectedCode, payload.Error.Code)
			}
		})
	}
}

func TestPokemonPendingSpeciesResolveHandlerReturnsMappedResolvedResult(t *testing.T) {
	now := time.Date(2026, time.March, 6, 14, 0, 0, 0, time.UTC)
	levelEstimate := 22.5
	levelConfidence := 0.67
	frameTimestamp := int64(600)
	confidence := 0.91
	handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{
		resolvePendingFn: func(_ context.Context, params upload.ResolvePendingReadingParams) (upload.PokemonResultRecord, error) {
			if params.ReadingID != "reading-1" || params.OptionID != "option-1" || params.OwnerKey != "session-1" {
				t.Fatalf("unexpected resolve params: %#v", params)
			}

			return upload.PokemonResultRecord{
				ID:                   "result-1",
				JobID:                "job-1",
				UploadID:             "upload-1",
				SessionID:            "session-1",
				SpeciesName:          "Darumaka",
				CP:                   712,
				HP:                   120,
				PowerUpStardustCost:  0,
				IVAttack:             10,
				IVDefense:            11,
				IVStamina:            12,
				LevelEstimate:        &levelEstimate,
				LevelConfidence:      &levelConfidence,
				LevelMethod:          "ARC_POSITION",
				SourceType:           "VIDEO",
				FrameTimestampMS:     &frameTimestamp,
				ExtractionConfidence: &confidence,
				CreatedAt:            now,
			}, nil
		},
	}, func() time.Time { return now })

	req := newPokemonPendingSpeciesResolveHandlerRequest(
		http.MethodPatch,
		"/pokemon/pending-species/reading-1",
		"reading-1",
		"session-1",
		`{"optionId":"option-1"}`,
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload pokemonPendingSpeciesResolveResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected response payload, got %v", err)
	}
	if payload.Result.ID != "result-1" || payload.Result.SpeciesName != "Darumaka" {
		t.Fatalf("unexpected result payload: %#v", payload.Result)
	}
	if payload.Result.IVs.Attack != 10 || payload.Result.IVs.Defense != 11 || payload.Result.IVs.Stamina != 12 {
		t.Fatalf("unexpected result iv payload: %#v", payload.Result.IVs)
	}
	if payload.Result.Level.Estimate == nil || *payload.Result.Level.Estimate != levelEstimate {
		t.Fatalf("unexpected level estimate payload: %#v", payload.Result.Level)
	}
	if payload.Result.Source.FrameTimestampMS == nil || *payload.Result.Source.FrameTimestampMS != frameTimestamp {
		t.Fatalf("unexpected frame timestamp payload: %#v", payload.Result.Source)
	}
	if payload.Result.Confidence == nil || *payload.Result.Confidence != confidence {
		t.Fatalf("unexpected confidence payload: %#v", payload.Result.Confidence)
	}
}

func TestPokemonPendingSpeciesResolveHandlerReturnsInternalErrorWhenStoreFails(t *testing.T) {
	handler := newPokemonPendingSpeciesResolveHandler(&fakePokemonPendingSpeciesStore{
		resolvePendingFn: func(context.Context, upload.ResolvePendingReadingParams) (upload.PokemonResultRecord, error) {
			return upload.PokemonResultRecord{}, errors.New("db unavailable")
		},
	}, time.Now)

	req := newPokemonPendingSpeciesResolveHandlerRequest(
		http.MethodPatch,
		"/pokemon/pending-species/reading-1",
		"reading-1",
		"session-1",
		`{"optionId":"option-1"}`,
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

type fakePokemonPendingSpeciesStore struct {
	listPendingFn    func(ctx context.Context, ownerKey string) ([]upload.PendingSpeciesReadingRecord, error)
	resolvePendingFn func(ctx context.Context, params upload.ResolvePendingReadingParams) (upload.PokemonResultRecord, error)
}

func (s *fakePokemonPendingSpeciesStore) CreateUploadAndQueuedJob(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
	return upload.Upload{}, upload.Job{}, errors.New("not implemented")
}

func (s *fakePokemonPendingSpeciesStore) CreateRetryJob(context.Context, string, string, time.Time) (upload.RetryJob, error) {
	return upload.RetryJob{}, errors.New("not implemented")
}

func (s *fakePokemonPendingSpeciesStore) GetJobStatus(context.Context, string, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, errors.New("not implemented")
}

func (s *fakePokemonPendingSpeciesStore) GetActiveJobStatus(context.Context, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, upload.ErrJobNotFound
}

func (s *fakePokemonPendingSpeciesStore) ListPokemonResults(context.Context, string) ([]upload.PokemonResultRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakePokemonPendingSpeciesStore) ListPendingReadings(
	ctx context.Context,
	ownerKey string,
) ([]upload.PendingSpeciesReadingRecord, error) {
	if s.listPendingFn != nil {
		return s.listPendingFn(ctx, ownerKey)
	}
	return nil, nil
}

func (s *fakePokemonPendingSpeciesStore) SoftDeletePokemonResult(context.Context, string, string, time.Time) error {
	return errors.New("not implemented")
}

func (s *fakePokemonPendingSpeciesStore) ResolvePendingReading(
	ctx context.Context,
	params upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	if s.resolvePendingFn != nil {
		return s.resolvePendingFn(ctx, params)
	}
	return upload.PokemonResultRecord{}, nil
}

func newPokemonPendingSpeciesHandlerRequest(method string, path string, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	return withTestGuestIdentity(req, sessionID)
}

func newPokemonPendingSpeciesResolveHandlerRequest(
	method string,
	path string,
	readingID string,
	sessionID string,
	body string,
) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.SetPathValue("readingId", readingID)
	return withTestGuestIdentity(req, sessionID)
}
