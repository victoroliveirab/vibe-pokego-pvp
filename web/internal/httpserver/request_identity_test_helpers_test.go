package httpserver

import (
	"net/http"
)

func withTestGuestIdentity(req *http.Request, sessionID string) *http.Request {
	ctx := contextWithIdentity(req.Context(), RequestIdentity{
		Mode:      IdentityModeGuest,
		SessionID: sessionID,
		ownerKey:  sessionID,
	})
	return req.WithContext(ctx)
}
