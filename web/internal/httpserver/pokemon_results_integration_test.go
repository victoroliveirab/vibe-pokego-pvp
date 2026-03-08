package httpserver

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestPokemonResultsIntegrationReturnsSessionScopedResultsInDeterministicOrder(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)

	sessionA := createSessionViaHTTP(t, env.client, env.server.URL)
	sessionB := createSessionViaHTTP(t, env.client, env.server.URL)

	a1 := createUploadAndJobViaHTTP(t, env, sessionA)
	a2 := createUploadAndJobViaHTTP(t, env, sessionA)
	a3 := createUploadAndJobViaHTTP(t, env, sessionA)
	b1 := createUploadAndJobViaHTTP(t, env, sessionB)

	baseCreatedAt := time.Date(2026, time.March, 5, 18, 0, 0, 0, time.UTC)
	laterCreatedAt := baseCreatedAt.Add(time.Second)

	levelEstimate := 23.5
	levelConfidence := 0.72
	startMS := int64(12000)
	endMS := int64(15500)
	frameTimestampMS := int64(13200)
	extractionConfidence := 0.86

	insertIntegrationAppraisalResultRow(t, env.dbPath, integrationAppraisalResultRow{
		ID:                  "result-b",
		JobID:               a2.JobID,
		UploadID:            a2.UploadID,
		SessionID:           sessionA,
		SpeciesName:         "Pikachu",
		CP:                  410,
		HP:                  64,
		PowerUpStardustCost: 3000,
		IVAttack:            10,
		IVDefense:           12,
		IVStamina:           11,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           baseCreatedAt,
	})
	insertIntegrationAppraisalResultRow(t, env.dbPath, integrationAppraisalResultRow{
		ID:                  "result-z",
		JobID:               b1.JobID,
		UploadID:            b1.UploadID,
		SessionID:           sessionB,
		SpeciesName:         "Charmander",
		CP:                  500,
		HP:                  70,
		PowerUpStardustCost: 3000,
		IVAttack:            11,
		IVDefense:           12,
		IVStamina:           13,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           baseCreatedAt,
	})
	insertIntegrationAppraisalResultRow(t, env.dbPath, integrationAppraisalResultRow{
		ID:                   "result-a",
		JobID:                a1.JobID,
		UploadID:             a1.UploadID,
		SessionID:            sessionA,
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
		CreatedAt:            baseCreatedAt,
	})
	insertIntegrationAppraisalResultRow(t, env.dbPath, integrationAppraisalResultRow{
		ID:                  "result-c",
		JobID:               a3.JobID,
		UploadID:            a3.UploadID,
		SessionID:           sessionA,
		SpeciesName:         "Bulbasaur",
		CP:                  300,
		HP:                  50,
		PowerUpStardustCost: 2500,
		IVAttack:            9,
		IVDefense:           8,
		IVStamina:           10,
		LevelMethod:         "UNKNOWN",
		SourceType:          "IMAGE",
		CreatedAt:           laterCreatedAt,
	})

	resp := sendPokemonResultsRequest(t, env.client, http.MethodGet, env.server.URL+"/pokemon", sessionA)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload pokemonResultsEnvelopeResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected pokemon results payload, got: %v", err)
	}

	if len(payload.Results) != 3 {
		t.Fatalf("expected 3 results for session %q, got %d", sessionA, len(payload.Results))
	}

	expectedIDs := []string{"result-a", "result-b", "result-c"}
	for idx, expectedID := range expectedIDs {
		if payload.Results[idx].ID != expectedID {
			t.Fatalf("expected result index %d id %q, got %q", idx, expectedID, payload.Results[idx].ID)
		}
	}

	first := payload.Results[0]
	if first.SpeciesName != "Machop" || first.CP != 512 || first.HP != 64 || first.PowerUpStardustCost != 2500 {
		t.Fatalf("unexpected first result scalar fields: %#v", first)
	}
	if first.Level.Method != "ARC_POSITION" {
		t.Fatalf("expected first level.method ARC_POSITION, got %q", first.Level.Method)
	}
	if first.Level.Estimate == nil || *first.Level.Estimate != levelEstimate {
		t.Fatalf("expected first level.estimate %v, got %#v", levelEstimate, first.Level.Estimate)
	}
	if first.Level.Confidence == nil || *first.Level.Confidence != levelConfidence {
		t.Fatalf("expected first level.confidence %v, got %#v", levelConfidence, first.Level.Confidence)
	}
	if first.Source.Type != "VIDEO" {
		t.Fatalf("expected first source.type VIDEO, got %q", first.Source.Type)
	}
	if first.Source.TimeRangeMS.Start == nil || *first.Source.TimeRangeMS.Start != startMS {
		t.Fatalf("expected first source.timeRangeMs.start %d, got %#v", startMS, first.Source.TimeRangeMS.Start)
	}
	if first.Source.TimeRangeMS.End == nil || *first.Source.TimeRangeMS.End != endMS {
		t.Fatalf("expected first source.timeRangeMs.end %d, got %#v", endMS, first.Source.TimeRangeMS.End)
	}
	if first.Source.FrameTimestampMS == nil || *first.Source.FrameTimestampMS != frameTimestampMS {
		t.Fatalf("expected first source.frameTimestampMs %d, got %#v", frameTimestampMS, first.Source.FrameTimestampMS)
	}
	if first.Confidence == nil || *first.Confidence != extractionConfidence {
		t.Fatalf("expected first confidence %v, got %#v", extractionConfidence, first.Confidence)
	}

	second := payload.Results[1]
	if second.ID != "result-b" {
		t.Fatalf("expected second id result-b, got %q", second.ID)
	}
	if second.Level.Estimate != nil || second.Level.Confidence != nil {
		t.Fatalf("expected null level estimate/confidence for second result, got %#v", second.Level)
	}
	if second.Source.TimeRangeMS.Start != nil || second.Source.TimeRangeMS.End != nil {
		t.Fatalf("expected null timeRangeMs for second result, got %#v", second.Source.TimeRangeMS)
	}
	if second.Source.FrameTimestampMS != nil {
		t.Fatalf("expected null frameTimestampMs for second result, got %#v", second.Source.FrameTimestampMS)
	}
	if second.Confidence != nil {
		t.Fatalf("expected null confidence for second result, got %#v", second.Confidence)
	}

	for _, result := range payload.Results {
		if result.ID == "result-z" {
			t.Fatal("expected session B result to be excluded from session A response")
		}
	}

	repeatResp := sendPokemonResultsRequest(t, env.client, http.MethodGet, env.server.URL+"/pokemon", sessionA)
	defer repeatResp.Body.Close()

	if repeatResp.StatusCode != http.StatusOK {
		t.Fatalf("expected repeated status %d, got %d", http.StatusOK, repeatResp.StatusCode)
	}

	var repeatedPayload pokemonResultsEnvelopeResponse
	if err := json.NewDecoder(repeatResp.Body).Decode(&repeatedPayload); err != nil {
		t.Fatalf("expected repeated pokemon results payload, got: %v", err)
	}

	if len(repeatedPayload.Results) != len(expectedIDs) {
		t.Fatalf("expected %d repeated results, got %d", len(expectedIDs), len(repeatedPayload.Results))
	}
	for idx, expectedID := range expectedIDs {
		if repeatedPayload.Results[idx].ID != expectedID {
			t.Fatalf("expected repeated result index %d id %q, got %q", idx, expectedID, repeatedPayload.Results[idx].ID)
		}
	}
}

func TestPokemonResultsIntegrationReturnsInvalidSessionErrors(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	validSessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	_ = validSessionID

	testCases := []struct {
		name      string
		sessionID string
	}{
		{name: "missing header", sessionID: ""},
		{name: "malformed header", sessionID: "not-a-uuid"},
		{name: "unknown session", sessionID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendPokemonResultsRequest(t, env.client, http.MethodGet, env.server.URL+"/pokemon", tc.sessionID)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
			}

			var payload APIError
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("expected API error payload, got: %v", err)
			}
			if payload.Error.Code != "INVALID_SESSION" {
				t.Fatalf("expected INVALID_SESSION code, got %q", payload.Error.Code)
			}
		})
	}
}

type integrationAppraisalResultRow struct {
	ID                   string
	JobID                string
	UploadID             string
	SessionID            string
	SpeciesName          string
	CP                   int
	HP                   int
	PowerUpStardustCost  int
	IVAttack             int
	IVDefense            int
	IVStamina            int
	LevelEstimate        *float64
	LevelConfidence      *float64
	LevelMethod          string
	SourceType           string
	StartMS              *int64
	EndMS                *int64
	FrameTimestampMS     *int64
	ExtractionConfidence *float64
	CreatedAt            time.Time
}

func insertIntegrationAppraisalResultRow(t *testing.T, dbPath string, row integrationAppraisalResultRow) {
	t.Helper()

	db := openIntegrationDB(t, dbPath)

	const insertResult = `
INSERT INTO appraisal_results(
	id, job_id, upload_id, session_id, species_name, cp, hp, power_up_stardust_cost,
	iv_attack, iv_defense, iv_stamina, level_estimate, level_confidence, level_method,
	source_type, start_ms, end_ms, frame_timestamp_ms, extraction_confidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	var levelEstimate any
	if row.LevelEstimate != nil {
		levelEstimate = *row.LevelEstimate
	}

	var levelConfidence any
	if row.LevelConfidence != nil {
		levelConfidence = *row.LevelConfidence
	}

	var startMS any
	if row.StartMS != nil {
		startMS = *row.StartMS
	}

	var endMS any
	if row.EndMS != nil {
		endMS = *row.EndMS
	}

	var frameTimestampMS any
	if row.FrameTimestampMS != nil {
		frameTimestampMS = *row.FrameTimestampMS
	}

	var extractionConfidence any
	if row.ExtractionConfidence != nil {
		extractionConfidence = *row.ExtractionConfidence
	}

	if _, err := db.Exec(
		insertResult,
		row.ID,
		row.JobID,
		row.UploadID,
		row.SessionID,
		row.SpeciesName,
		row.CP,
		row.HP,
		row.PowerUpStardustCost,
		row.IVAttack,
		row.IVDefense,
		row.IVStamina,
		levelEstimate,
		levelConfidence,
		row.LevelMethod,
		row.SourceType,
		startMS,
		endMS,
		frameTimestampMS,
		extractionConfidence,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected appraisal result insert to succeed, got: %v", err)
	}
}

func sendPokemonResultsRequest(t *testing.T, client *http.Client, method string, url string, sessionID string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("expected request creation to succeed, got: %v", err)
	}
	if sessionID != "" {
		req.Header.Set(sessionHeaderName, sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected request to succeed, got: %v", err)
	}

	return resp
}
