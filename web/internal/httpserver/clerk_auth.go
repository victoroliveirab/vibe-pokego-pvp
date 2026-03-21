package httpserver

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
)

const authorizationHeaderName = "Authorization"

type clerkAuthenticator struct {
	options []clerkhttp.AuthorizationOption
}

func newClerkAuthenticator(cfg config.ClerkConfig) (*clerkAuthenticator, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	clerk.SetKey(cfg.SecretKey)

	options := []clerkhttp.AuthorizationOption{
		clerkhttp.AuthorizationFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeInvalidAuthorization(w)
		})),
	}

	if len(cfg.AuthorizedParties) > 0 {
		options = append(options, clerkhttp.AuthorizedPartyMatches(cfg.AuthorizedParties...))
	}

	if client, err := newClerkJWKSClient(cfg); err != nil {
		return nil, err
	} else if client != nil {
		options = append(options, clerkhttp.JWKSClient(client))
	}

	return &clerkAuthenticator{options: options}, nil
}

func newTestClerkAuthenticator(options ...clerkhttp.AuthorizationOption) *clerkAuthenticator {
	baseOptions := []clerkhttp.AuthorizationOption{
		clerkhttp.AuthorizationFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeInvalidAuthorization(w)
		})),
	}
	baseOptions = append(baseOptions, options...)
	return &clerkAuthenticator{options: baseOptions}
}

func (a *clerkAuthenticator) Authenticate(next http.Handler) http.Handler {
	if a == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeInvalidAuthorization(w)
		})
	}

	return clerkhttp.WithHeaderAuthorization(a.options...)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := clerk.SessionClaimsFromContext(r.Context())
		if !ok || claims == nil || strings.TrimSpace(claims.Subject) == "" {
			writeInvalidAuthorization(w)
			return
		}

		next.ServeHTTP(w, r)
	}))
}

func hasAuthorizationHeader(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get(authorizationHeaderName)) != ""
}

func newClerkJWKSClient(cfg config.ClerkConfig) (*jwks.Client, error) {
	jwksURL := strings.TrimSpace(cfg.JWKSURL)
	if jwksURL == "" {
		return nil, nil
	}

	parsedURL, err := url.Parse(jwksURL)
	if err != nil {
		return nil, err
	}

	baseURL := parsedURL.Scheme + "://" + parsedURL.Host
	httpClient := &http.Client{
		Transport: &jwksRewriteTransport{
			target: parsedURL,
			base:   http.DefaultTransport,
		},
	}

	return jwks.NewClient(&clerk.ClientConfig{
		BackendConfig: clerk.BackendConfig{
			HTTPClient: httpClient,
			URL:        clerk.String(baseURL),
			Key:        clerk.String(cfg.SecretKey),
		},
	}), nil
}

type jwksRewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t *jwksRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL = cloneURL(t.target)
	cloned.Host = cloned.URL.Host
	return t.base.RoundTrip(cloned)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return &url.URL{}
	}
	cloned := *value
	return &cloned
}

func writeInvalidAuthorization(w http.ResponseWriter) {
	writeAPIError(w, http.StatusUnauthorized, "INVALID_AUTHORIZATION", "Missing or invalid Authorization header", nil)
}
