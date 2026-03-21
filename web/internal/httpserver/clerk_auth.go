package httpserver

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/clerk/clerk-sdk-go/v2/jwks"
	clerkjwt "github.com/clerk/clerk-sdk-go/v2/jwt"
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

	jwksClient, err := newClerkJWKSClient(cfg)
	if err != nil {
		return nil, err
	}

	options := []clerkhttp.AuthorizationOption{
		clerkhttp.AuthorizationFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logClerkAuthorizationFailure(r, cfg, jwksClient)
			writeInvalidAuthorization(w)
		})),
	}

	if len(cfg.AuthorizedParties) > 0 {
		options = append(options, clerkhttp.AuthorizedPartyMatches(cfg.AuthorizedParties...))
	}
	if strings.TrimSpace(cfg.ProxyURL) != "" {
		options = append(options, clerkhttp.ProxyURL(cfg.ProxyURL))
	}

	if jwksClient != nil {
		options = append(options, clerkhttp.JWKSClient(jwksClient))
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

func logClerkAuthorizationFailure(r *http.Request, cfg config.ClerkConfig, jwksClient *jwks.Client) {
	logger := slog.Default()
	if r == nil {
		logger.Warn("clerk authorization failed", "reason", "request missing")
		return
	}

	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.Header.Get(authorizationHeaderName)), "Bearer "))
	if token == "" {
		logger.Warn(
			"clerk authorization failed",
			"reason", "authorization token missing",
			"path", r.URL.Path,
			"proxy_url", cfg.ProxyURL,
			"authorized_parties", cfg.AuthorizedParties,
		)
		return
	}

	decoded, err := clerkjwt.Decode(r.Context(), &clerkjwt.DecodeParams{Token: token})
	if err != nil {
		logger.Warn(
			"clerk authorization failed",
			"reason", "token decode failed",
			"error", err.Error(),
			"path", r.URL.Path,
			"proxy_url", cfg.ProxyURL,
			"authorized_parties", cfg.AuthorizedParties,
		)
		return
	}

	verifyParams := clerkjwt.VerifyParams{
		Token:      token,
		JWKSClient: jwksClient,
		Clock:      clerk.NewClock(),
	}
	if strings.TrimSpace(cfg.ProxyURL) != "" {
		verifyParams.ProxyURL = clerk.String(cfg.ProxyURL)
	}
	if len(cfg.AuthorizedParties) > 0 {
		authorizedParties := make(map[string]struct{}, len(cfg.AuthorizedParties))
		for _, party := range cfg.AuthorizedParties {
			authorizedParties[party] = struct{}{}
		}
		verifyParams.AuthorizedPartyHandler = func(azp string) bool {
			if azp == "" || len(authorizedParties) == 0 {
				return true
			}
			_, ok := authorizedParties[azp]
			return ok
		}
	}

	claims, verifyErr := clerkjwt.Verify(r.Context(), &verifyParams)
	extraClaims := decoded.Extra
	logger.Warn(
		"clerk authorization failed",
		"path", r.URL.Path,
		"host", r.Host,
		"issuer", decoded.Issuer,
		"subject", decoded.Subject,
		"kid", decoded.KeyID,
		"sid", stringClaim(extraClaims, "sid"),
		"azp", stringClaim(extraClaims, "azp"),
		"proxy_url", cfg.ProxyURL,
		"authorized_parties", cfg.AuthorizedParties,
		"jwks_url", cfg.JWKSURL,
		"frontend_api_url", cfg.FrontendAPIURL,
		"verify_error", errorString(verifyErr),
		"manual_verify_subject", claimsSubject(claims),
	)
}

func stringClaim(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return value
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func claimsSubject(claims *clerk.SessionClaims) string {
	if claims == nil {
		return ""
	}
	return claims.Subject
}
