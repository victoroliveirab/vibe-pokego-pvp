package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
)

func TestClerkFrontendAPIProxyHandlerForwardsRequestWithRequiredHeaders(t *testing.T) {
	var receivedPath string
	var receivedQuery string
	var receivedProxyURL string
	var receivedSecretKey string
	var receivedForwardedFor string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedQuery = r.URL.RawQuery
		receivedProxyURL = r.Header.Get("Clerk-Proxy-Url")
		receivedSecretKey = r.Header.Get("Clerk-Secret-Key")
		receivedForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler, err := newClerkFrontendAPIProxyHandler(config.ClerkConfig{
		Enabled:        true,
		SecretKey:      "sk_test_proxy",
		ProxyURL:       "/api/__clerk",
		FrontendAPIURL: upstream.URL,
	})
	if err != nil {
		t.Fatalf("expected proxy handler to initialize, got: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/__clerk/v1/client?foo=bar", nil)
	req.Host = "vibepogo.victoroliveira.com.br"
	req.RemoteAddr = "203.0.113.8:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected status %d, got %d (body=%s)", http.StatusNoContent, rec.Code, string(body))
	}
	if receivedPath != "/v1/client" {
		t.Fatalf("expected upstream path %q, got %q", "/v1/client", receivedPath)
	}
	if receivedQuery != "foo=bar" {
		t.Fatalf("expected upstream query %q, got %q", "foo=bar", receivedQuery)
	}
	if receivedProxyURL != "https://vibepogo.victoroliveira.com.br/api/__clerk" {
		t.Fatalf("expected Clerk-Proxy-Url header %q, got %q", "https://vibepogo.victoroliveira.com.br/api/__clerk", receivedProxyURL)
	}
	if receivedSecretKey != "sk_test_proxy" {
		t.Fatalf("expected Clerk-Secret-Key header %q, got %q", "sk_test_proxy", receivedSecretKey)
	}
	if receivedForwardedFor != "203.0.113.8" {
		t.Fatalf("expected X-Forwarded-For header %q, got %q", "203.0.113.8", receivedForwardedFor)
	}
}

func TestClerkFrontendAPIProxyHandlerUsesAbsoluteConfiguredProxyURL(t *testing.T) {
	var receivedProxyURL string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedProxyURL = r.Header.Get("Clerk-Proxy-Url")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	handler, err := newClerkFrontendAPIProxyHandler(config.ClerkConfig{
		Enabled:        true,
		SecretKey:      "sk_test_proxy",
		ProxyURL:       "https://app.example.com/api/__clerk",
		FrontendAPIURL: upstream.URL,
	})
	if err != nil {
		t.Fatalf("expected proxy handler to initialize, got: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/__clerk", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if receivedProxyURL != "https://app.example.com/api/__clerk" {
		t.Fatalf("expected Clerk-Proxy-Url header %q, got %q", "https://app.example.com/api/__clerk", receivedProxyURL)
	}
}
