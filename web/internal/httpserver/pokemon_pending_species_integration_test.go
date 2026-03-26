package httpserver

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestPokemonPendingSpeciesIntegrationReturnsSessionScopedReadingsWithOptions(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionA := createSessionViaHTTP(t, env.client, env.server.URL)
	sessionB := createSessionViaHTTP(t, env.client, env.server.URL)

	createdA := createUploadAndJobViaHTTP(t, env, sessionA)
	createdB := createUploadAndJobViaHTTP(t, env, sessionB)
	pendingAt := time.Date(2026, time.March, 6, 16, 0, 0, 0, time.UTC)
	finishedAt := pendingAt
	setJobLifecycleState(t, env.dbPath, createdA.JobID, upload.JobStatusPendingUserDedup, 100, nil, nil, pendingAt, &finishedAt, nil, nil)
	setJobLifecycleState(t, env.dbPath, createdB.JobID, upload.JobStatusPendingUserDedup, 100, nil, nil, pendingAt, &finishedAt, nil, nil)

	readingA := seededPendingReadingRow{
		ID:               "reading-a",
		JobID:            createdA.JobID,
		UploadID:         createdA.UploadID,
		SessionID:        sessionA,
		CP:               712,
		HP:               120,
		IVAttack:         10,
		IVDefense:        11,
		IVStamina:        12,
		LevelMethod:      "ARC_POSITION",
		SourceType:       "VIDEO",
		Status:           upload.JobStatusPendingUserDedup,
		Locked:           false,
		CreatedAt:        pendingAt,
		FrameTimestampMS: int64Ptr(300),
	}
	readingB := seededPendingReadingRow{
		ID:               "reading-b",
		JobID:            createdB.JobID,
		UploadID:         createdB.UploadID,
		SessionID:        sessionB,
		CP:               500,
		HP:               100,
		IVAttack:         11,
		IVDefense:        11,
		IVStamina:        11,
		LevelMethod:      "UNKNOWN",
		SourceType:       "IMAGE",
		Status:           upload.JobStatusPendingUserDedup,
		Locked:           false,
		CreatedAt:        pendingAt,
		FrameTimestampMS: nil,
	}
	insertIntegrationPendingReadingRow(t, env.dbPath, readingA)
	insertIntegrationPendingReadingRow(t, env.dbPath, readingB)
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-a1",
		PendingReadingID: readingA.ID,
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        pendingAt,
	})
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-a2",
		PendingReadingID: readingA.ID,
		SpeciesName:      "Darumaka (Galarian)",
		MatchMode:        "fuzzy",
		MatchDistance:    1,
		OptionRank:       2,
		CreatedAt:        pendingAt,
	})
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-b1",
		PendingReadingID: readingB.ID,
		SpeciesName:      "Pikachu",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        pendingAt,
	})

	resp := sendPokemonResultsRequest(t, env.client, http.MethodGet, env.server.URL+"/pokemon/pending-species", sessionA)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload pokemonPendingSpeciesEnvelopeResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected payload decode to succeed, got %v", err)
	}
	if len(payload.Readings) != 1 {
		t.Fatalf("expected 1 reading for session A, got %d", len(payload.Readings))
	}
	reading := payload.Readings[0]
	if reading.ID != readingA.ID {
		t.Fatalf("expected reading id %q, got %q", readingA.ID, reading.ID)
	}
	if len(reading.Options) != 2 {
		t.Fatalf("expected 2 options for reading A, got %d", len(reading.Options))
	}
	if reading.Options[0].OptionRank != 1 || reading.Options[1].OptionRank != 2 {
		t.Fatalf("expected ordered options, got %#v", reading.Options)
	}
}

func TestPokemonPendingSpeciesResolveIntegrationFinalizesReadingAndRejectsReresolve(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)
	pendingAt := time.Date(2026, time.March, 6, 17, 0, 0, 0, time.UTC)
	finishedAt := pendingAt
	setJobLifecycleState(
		t,
		env.dbPath,
		created.JobID,
		upload.JobStatusPendingUserDedup,
		100,
		nil,
		nil,
		pendingAt,
		&finishedAt,
		nil,
		nil,
	)

	reading := seededPendingReadingRow{
		ID:               "reading-resolve",
		JobID:            created.JobID,
		UploadID:         created.UploadID,
		SessionID:        sessionID,
		CP:               712,
		HP:               120,
		IVAttack:         10,
		IVDefense:        11,
		IVStamina:        12,
		LevelMethod:      "ARC_POSITION",
		SourceType:       "VIDEO",
		Status:           upload.JobStatusPendingUserDedup,
		Locked:           false,
		CreatedAt:        pendingAt,
		FrameTimestampMS: int64Ptr(600),
	}
	insertIntegrationPendingReadingRow(t, env.dbPath, reading)
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-resolve-1",
		PendingReadingID: reading.ID,
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        pendingAt,
	})
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-resolve-2",
		PendingReadingID: reading.ID,
		SpeciesName:      "Darumaka (Galarian)",
		MatchMode:        "fuzzy",
		MatchDistance:    1,
		OptionRank:       2,
		CreatedAt:        pendingAt,
	})

	resolveResp := sendPendingResolveRequest(
		t,
		env.client,
		http.MethodPatch,
		env.server.URL+"/pokemon/pending-species/"+reading.ID,
		sessionID,
		`{"optionId":"option-resolve-2"}`,
	)
	defer resolveResp.Body.Close()

	if resolveResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resolveResp.StatusCode)
	}

	var resolvePayload pokemonPendingSpeciesResolveResponse
	if err := json.NewDecoder(resolveResp.Body).Decode(&resolvePayload); err != nil {
		t.Fatalf("expected resolve payload decode to succeed, got %v", err)
	}
	if resolvePayload.Result.SpeciesName != "Darumaka (Galarian)" {
		t.Fatalf("expected resolved species %q, got %q", "Darumaka (Galarian)", resolvePayload.Result.SpeciesName)
	}
	if resolvePayload.Result.Source.JobID != created.JobID {
		t.Fatalf("expected resolved job id %q, got %q", created.JobID, resolvePayload.Result.Source.JobID)
	}

	status := readIntegrationPendingReadingStatus(t, env.dbPath, reading.ID)
	if status.Status != "RESOLVED" || status.Locked != 1 || !status.SelectedSpeciesName.Valid {
		t.Fatalf("expected resolved+locked pending reading, got %#v", status)
	}
	if status.SelectedSpeciesName.String != "Darumaka (Galarian)" {
		t.Fatalf("expected selected species %q, got %q", "Darumaka (Galarian)", status.SelectedSpeciesName.String)
	}

	jobSnapshot := readIntegrationJobSnapshot(t, env.dbPath, created.JobID)
	if jobSnapshot.Status != upload.JobStatusSucceeded {
		t.Fatalf("expected job status %q, got %q", upload.JobStatusSucceeded, jobSnapshot.Status)
	}
	if !jobSnapshot.FinishedAt.Valid {
		t.Fatalf("expected finished_at to be set after resolve")
	}

	db := openIntegrationDB(t, env.dbPath)
	var resultsCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM appraisal_results WHERE session_id = ? AND job_id = ? AND species_name = ?;`,
		sessionID,
		created.JobID,
		"Darumaka (Galarian)",
	).Scan(&resultsCount); err != nil {
		t.Fatalf("expected appraisal result count query to succeed, got %v", err)
	}
	if resultsCount != 1 {
		t.Fatalf("expected one resolved appraisal result row, got %d", resultsCount)
	}

	reResolveResp := sendPendingResolveRequest(
		t,
		env.client,
		http.MethodPatch,
		env.server.URL+"/pokemon/pending-species/"+reading.ID,
		sessionID,
		`{"optionId":"option-resolve-1"}`,
	)
	defer reResolveResp.Body.Close()
	if reResolveResp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d on re-resolve, got %d", http.StatusConflict, reResolveResp.StatusCode)
	}

	var errorPayload APIError
	if err := json.NewDecoder(reResolveResp.Body).Decode(&errorPayload); err != nil {
		t.Fatalf("expected error payload decode to succeed, got %v", err)
	}
	if errorPayload.Error.Code != "READING_LOCKED" {
		t.Fatalf("expected error code %q, got %q", "READING_LOCKED", errorPayload.Error.Code)
	}
}

func TestPokemonPendingSpeciesDismissIntegrationFinalizesReadingWithoutResult(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)
	pendingAt := time.Date(2026, time.March, 6, 17, 30, 0, 0, time.UTC)
	finishedAt := pendingAt
	setJobLifecycleState(
		t,
		env.dbPath,
		created.JobID,
		upload.JobStatusPendingUserDedup,
		100,
		nil,
		nil,
		pendingAt,
		&finishedAt,
		nil,
		nil,
	)

	reading := seededPendingReadingRow{
		ID:               "reading-dismiss",
		JobID:            created.JobID,
		UploadID:         created.UploadID,
		SessionID:        sessionID,
		CP:               712,
		HP:               120,
		IVAttack:         10,
		IVDefense:        11,
		IVStamina:        12,
		LevelMethod:      "ARC_POSITION",
		SourceType:       "VIDEO",
		Status:           upload.JobStatusPendingUserDedup,
		Locked:           false,
		CreatedAt:        pendingAt,
		FrameTimestampMS: int64Ptr(900),
	}
	insertIntegrationPendingReadingRow(t, env.dbPath, reading)
	insertIntegrationPendingOptionRow(t, env.dbPath, seededPendingOptionRow{
		ID:               "option-dismiss-1",
		PendingReadingID: reading.ID,
		SpeciesName:      "Darumaka",
		MatchMode:        "exact",
		MatchDistance:    0,
		OptionRank:       1,
		CreatedAt:        pendingAt,
	})

	dismissResp := sendPendingResolveRequest(
		t,
		env.client,
		http.MethodDelete,
		env.server.URL+"/pokemon/pending-species/"+reading.ID,
		sessionID,
		"",
	)
	defer dismissResp.Body.Close()

	if dismissResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, dismissResp.StatusCode)
	}

	status := readIntegrationPendingReadingStatus(t, env.dbPath, reading.ID)
	if status.Status != upload.PendingReadingStatusDismissed || status.Locked != 1 || status.SelectedSpeciesName.Valid {
		t.Fatalf("expected dismissed+locked pending reading, got %#v", status)
	}

	jobSnapshot := readIntegrationJobSnapshot(t, env.dbPath, created.JobID)
	if jobSnapshot.Status != upload.JobStatusSucceeded {
		t.Fatalf("expected job status %q, got %q", upload.JobStatusSucceeded, jobSnapshot.Status)
	}
	if !jobSnapshot.FinishedAt.Valid {
		t.Fatalf("expected finished_at to be set after dismiss")
	}

	db := openIntegrationDB(t, env.dbPath)
	var resultsCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM appraisal_results WHERE session_id = ? AND job_id = ?;`,
		sessionID,
		created.JobID,
	).Scan(&resultsCount); err != nil {
		t.Fatalf("expected appraisal result count query to succeed, got %v", err)
	}
	if resultsCount != 0 {
		t.Fatalf("expected no appraisal result rows after dismiss, got %d", resultsCount)
	}
}

type seededPendingReadingRow struct {
	ID                   string
	JobID                string
	UploadID             string
	SessionID            string
	CP                   int
	HP                   int
	IVAttack             int
	IVDefense            int
	IVStamina            int
	LevelEstimate        *float64
	LevelConfidence      *float64
	LevelMethod          string
	SourceType           string
	FrameTimestampMS     *int64
	ExtractionConfidence *float64
	Status               string
	Locked               bool
	SelectedSpeciesName  *string
	ResolvedAt           *time.Time
	CreatedAt            time.Time
}

type seededPendingOptionRow struct {
	ID               string
	PendingReadingID string
	SpeciesName      string
	MatchMode        string
	MatchDistance    int
	OptionRank       int
	CreatedAt        time.Time
}

func insertIntegrationPendingReadingRow(t *testing.T, dbPath string, row seededPendingReadingRow) {
	t.Helper()
	db := openIntegrationDB(t, dbPath)

	var levelEstimate any
	if row.LevelEstimate != nil {
		levelEstimate = *row.LevelEstimate
	}
	var levelConfidence any
	if row.LevelConfidence != nil {
		levelConfidence = *row.LevelConfidence
	}
	var frameTimestampMS any
	if row.FrameTimestampMS != nil {
		frameTimestampMS = *row.FrameTimestampMS
	}
	var extractionConfidence any
	if row.ExtractionConfidence != nil {
		extractionConfidence = *row.ExtractionConfidence
	}
	var selectedSpeciesName any
	if row.SelectedSpeciesName != nil {
		selectedSpeciesName = *row.SelectedSpeciesName
	}
	var resolvedAt any
	if row.ResolvedAt != nil {
		resolvedAt = row.ResolvedAt.UTC().Format(time.RFC3339Nano)
	}

	locked := 0
	if row.Locked {
		locked = 1
	}

	const insertPendingReading = `
INSERT INTO appraisal_pending_readings(
	id, job_id, upload_id, session_id, cp, hp, iv_attack, iv_defense, iv_stamina,
	level_estimate, level_confidence, level_method, source_type, frame_timestamp_ms,
	extraction_confidence, status, locked, selected_species_name, resolved_at, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.Exec(
		insertPendingReading,
		row.ID,
		row.JobID,
		row.UploadID,
		row.SessionID,
		row.CP,
		row.HP,
		row.IVAttack,
		row.IVDefense,
		row.IVStamina,
		levelEstimate,
		levelConfidence,
		row.LevelMethod,
		row.SourceType,
		frameTimestampMS,
		extractionConfidence,
		row.Status,
		locked,
		selectedSpeciesName,
		resolvedAt,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected pending reading insert to succeed, got: %v", err)
	}
}

func insertIntegrationPendingOptionRow(t *testing.T, dbPath string, row seededPendingOptionRow) {
	t.Helper()
	db := openIntegrationDB(t, dbPath)

	const insertPendingOption = `
INSERT INTO appraisal_pending_species_options(
	id, pending_reading_id, species_name, species_name_normalized,
	match_mode, match_distance, option_rank, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?);`

	if _, err := db.Exec(
		insertPendingOption,
		row.ID,
		row.PendingReadingID,
		row.SpeciesName,
		row.SpeciesName,
		row.MatchMode,
		row.MatchDistance,
		row.OptionRank,
		row.CreatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		t.Fatalf("expected pending option insert to succeed, got: %v", err)
	}
}

type pendingReadingStatusSnapshot struct {
	Status              string
	Locked              int
	SelectedSpeciesName sql.NullString
}

func readIntegrationPendingReadingStatus(t *testing.T, dbPath string, readingID string) pendingReadingStatusSnapshot {
	t.Helper()
	db := openIntegrationDB(t, dbPath)

	const query = `
SELECT status, locked, selected_species_name
FROM appraisal_pending_readings
WHERE id = ?;`

	var snapshot pendingReadingStatusSnapshot
	if err := db.QueryRow(query, readingID).Scan(
		&snapshot.Status,
		&snapshot.Locked,
		&snapshot.SelectedSpeciesName,
	); err != nil {
		t.Fatalf("expected pending reading snapshot query to succeed, got %v", err)
	}
	return snapshot
}

func sendPendingResolveRequest(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	sessionID string,
	body string,
) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("expected request creation to succeed, got %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set(sessionHeaderName, sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected request to succeed, got %v", err)
	}
	return resp
}

func int64Ptr(value int64) *int64 {
	return &value
}
