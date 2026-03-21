package httpserver

import (
	"context"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type IdentityMode string

const (
	IdentityModeGuest IdentityMode = "guest"
	IdentityModeClerk IdentityMode = "clerk"
)

type requestIdentityContextKey struct{}

// RequestIdentity captures the authenticated owner associated with a request.
type RequestIdentity struct {
	Mode        IdentityMode
	SessionID   string
	ClerkUserID string
	ownerKey    string
}

// OwnerKey returns the opaque owner identifier used to scope persisted data.
func (i RequestIdentity) OwnerKey() string {
	return i.ownerKey
}

func newGuestIdentity(sess session.Session) RequestIdentity {
	return RequestIdentity{
		Mode:      IdentityModeGuest,
		SessionID: sess.ID,
		ownerKey:  upload.OwnerKeyForGuest(sess.ID),
	}
}

func newClerkIdentity(claims *clerk.SessionClaims) RequestIdentity {
	userID := strings.TrimSpace(claims.Subject)
	ownerKey := upload.OwnerKeyForClerkUser(userID)
	return RequestIdentity{
		Mode:        IdentityModeClerk,
		SessionID:   ownerKey,
		ClerkUserID: userID,
		ownerKey:    ownerKey,
	}
}

func contextWithIdentity(ctx context.Context, identity RequestIdentity) context.Context {
	return context.WithValue(ctx, requestIdentityContextKey{}, identity)
}

// IdentityFromContext returns the validated request identity injected by middleware.
func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	identity, ok := ctx.Value(requestIdentityContextKey{}).(RequestIdentity)
	return identity, ok
}
