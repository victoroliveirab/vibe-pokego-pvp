package httpserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/go-jose/go-jose/v3"
	golangjwt "github.com/go-jose/go-jose/v3/jwt"
)

type clerkTestTokenConfig struct {
	authorizedParty string
	issuer          string
	subject         string
}

func newClerkTestAuthenticator(t *testing.T, cfg clerkTestTokenConfig) (*clerkAuthenticator, string) {
	t.Helper()

	cfg = normalizeClerkTestTokenConfig(cfg)
	privateKey, publicKeyPEM := generateClerkTestKeyPair(t)
	token := signClerkTestToken(t, privateKey, cfg)

	authenticator := newTestClerkAuthenticator(
		clerkhttp.AuthorizedPartyMatches(cfg.authorizedParty),
		clerkhttp.JSONWebKey(publicKeyPEM),
		clerkhttp.ProxyURL(cfg.issuer),
	)

	return authenticator, token
}

func generateClerkTestKeyPair(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("expected rsa key generation to succeed, got: %v", err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("expected public key marshal to succeed, got: %v", err)
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	return privateKey, string(publicKeyPEM)
}

func signClerkTestToken(t *testing.T, privateKey *rsa.PrivateKey, cfg clerkTestTokenConfig) string {
	t.Helper()

	cfg = normalizeClerkTestTokenConfig(cfg)

	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key:       privateKey,
	}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test-kid"))
	if err != nil {
		t.Fatalf("expected signer creation to succeed, got: %v", err)
	}

	now := time.Now().UTC()
	token, err := golangjwt.Signed(signer).Claims(map[string]any{
		"sub": cfg.subject,
		"iss": cfg.issuer,
		"azp": cfg.authorizedParty,
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
		"nbf": now.Add(-1 * time.Minute).Unix(),
		"sid": "sess_test_123",
		"v":   2,
	}).CompactSerialize()
	if err != nil {
		t.Fatalf("expected jwt signing to succeed, got: %v", err)
	}

	return token
}

func normalizeClerkTestTokenConfig(cfg clerkTestTokenConfig) clerkTestTokenConfig {
	if cfg.authorizedParty == "" {
		cfg.authorizedParty = "http://localhost:4173"
	}
	if cfg.issuer == "" {
		cfg.issuer = "https://issuer.test"
	}
	if cfg.subject == "" {
		cfg.subject = "user_test_123"
	}

	return cfg
}
