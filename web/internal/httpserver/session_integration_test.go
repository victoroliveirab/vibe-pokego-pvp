package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
)

func TestSessionLifecycleIntegration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "integration.db")
	srv, err := New(config.Config{
		AppEnv:       "test",
		Port:         0,
		DatabasePath: dbPath,
		Storage: config.StorageConfig{
			Mode:     config.UploadStorageModeLocal,
			LocalDir: t.TempDir(),
		},
	}, config.StorageConfig{
		Mode:     config.UploadStorageModeLocal,
		LocalDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("expected server to initialize, got: %v", err)
	}

	testServer := httptest.NewServer(srv.Handler)
	t.Cleanup(testServer.Close)

	client := testServer.Client()

	createResp, err := client.Post(testServer.URL+"/session", "application/json", nil)
	if err != nil {
		t.Fatalf("expected POST /session request to succeed, got: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createResp.StatusCode)
	}

	var createPayload struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createPayload); err != nil {
		t.Fatalf("expected valid session response JSON, got: %v", err)
	}
	if err := session.ValidateID(createPayload.SessionID); err != nil {
		t.Fatalf("expected UUIDv4 session id, got %q: %v", createPayload.SessionID, err)
	}

	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store for verification, got: %v", err)
	}

	beforeTouch, err := store.GetByID(context.Background(), createPayload.SessionID)
	if err != nil {
		t.Fatalf("expected session in persistence, got: %v", err)
	}
	if !beforeTouch.CreatedAt.Equal(beforeTouch.LastSeenAt) {
		t.Fatalf("expected initial timestamps to match, created_at=%v last_seen_at=%v", beforeTouch.CreatedAt, beforeTouch.LastSeenAt)
	}

	assertProtectedInvalidSession(t, client, testServer.URL+"/protected/ping", "")
	assertProtectedInvalidSession(t, client, testServer.URL+"/protected/ping", "not-a-uuid")
	assertProtectedInvalidSession(t, client, testServer.URL+"/protected/ping", "12f9f169-d9ca-4ea3-91e0-18356a1e1477")

	time.Sleep(20 * time.Millisecond)

	protectedReq, err := http.NewRequest(http.MethodGet, testServer.URL+"/protected/ping", nil)
	if err != nil {
		t.Fatalf("expected protected request creation to succeed, got: %v", err)
	}
	protectedReq.Header.Set(sessionHeaderName, createPayload.SessionID)

	protectedResp, err := client.Do(protectedReq)
	if err != nil {
		t.Fatalf("expected protected request to succeed, got: %v", err)
	}
	defer protectedResp.Body.Close()

	if protectedResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, protectedResp.StatusCode)
	}

	var protectedPayload map[string]string
	if err := json.NewDecoder(protectedResp.Body).Decode(&protectedPayload); err != nil {
		t.Fatalf("expected valid protected response JSON, got: %v", err)
	}
	if protectedPayload["status"] != "ok" {
		t.Fatalf("expected protected status ok, got %#v", protectedPayload)
	}
	if protectedPayload["sessionId"] != createPayload.SessionID {
		t.Fatalf("expected protected response sessionId %q, got %q", createPayload.SessionID, protectedPayload["sessionId"])
	}

	afterTouch, err := store.GetByID(context.Background(), createPayload.SessionID)
	if err != nil {
		t.Fatalf("expected session after protected call, got: %v", err)
	}
	if !afterTouch.LastSeenAt.After(beforeTouch.LastSeenAt) {
		t.Fatalf("expected last_seen_at to advance, before=%v after=%v", beforeTouch.LastSeenAt, afterTouch.LastSeenAt)
	}
	if !afterTouch.CreatedAt.Equal(beforeTouch.CreatedAt) {
		t.Fatalf("expected created_at to remain stable, before=%v after=%v", beforeTouch.CreatedAt, afterTouch.CreatedAt)
	}
}

func assertProtectedInvalidSession(t *testing.T, client *http.Client, url string, sessionID string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON error payload, got: %v", err)
	}
	if payload.Error.Code != "INVALID_SESSION" {
		t.Fatalf("expected INVALID_SESSION code, got %q", payload.Error.Code)
	}
}
