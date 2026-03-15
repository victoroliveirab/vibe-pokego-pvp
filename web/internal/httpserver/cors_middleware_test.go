package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithCORSAddsAllowOriginForAllowedOrigin(t *testing.T) {
	nextCalled := false
	handler := withCORS([]string{"http://127.0.0.1:4173"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4173")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:4173" {
		t.Fatalf("expected Access-Control-Allow-Origin header, got %q", got)
	}
}

func TestWithCORSRespondsToPreflightForAllowedOrigin(t *testing.T) {
	handler := withCORS([]string{"http://127.0.0.1:4173"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/uploads", nil)
	req.Header.Set("Origin", "http://127.0.0.1:4173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,x-session-id")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, resp.Code)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:4173" {
		t.Fatalf("expected Access-Control-Allow-Origin header, got %q", got)
	}
	if got := resp.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("expected Access-Control-Allow-Methods header")
	}
	if got := resp.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPatch) {
		t.Fatalf("expected Access-Control-Allow-Methods to include %s, got %q", http.MethodPatch, got)
	}
	if got := resp.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodDelete) {
		t.Fatalf("expected Access-Control-Allow-Methods to include %s, got %q", http.MethodDelete, got)
	}
	if got := resp.Header().Get("Access-Control-Allow-Headers"); got != "content-type,x-session-id" {
		t.Fatalf("expected Access-Control-Allow-Headers to echo request headers, got %q", got)
	}
}

func TestWithCORSRejectsPreflightForDisallowedOrigin(t *testing.T) {
	handler := withCORS([]string{"http://127.0.0.1:4173"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called for preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/uploads", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, resp.Code)
	}
}

func TestWithCORSAllowsWildcardOrigin(t *testing.T) {
	handler := withCORS([]string{"*"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	req.Header.Set("Origin", "https://app.example.com")

	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.Code)
	}
	if got := resp.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin=*, got %q", got)
	}
}
