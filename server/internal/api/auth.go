// Package api holds the HTTP handlers, chi router, and auth helpers for the
// backend. It maps sentinel errors from store/state to stable JSON error
// responses via a single writeError helper. It never logs or returns raw
// tokens, Authorization headers, or visitor IPs (see logging guidelines
// "What NOT to log").
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"cyberstalk.me/server/internal/store"
)

// ErrBadToken is the sentinel for an invalid or missing device token, or a
// token that does not match the device_id in the body. Handlers map it to 401.
var ErrBadToken = errors.New("invalid device token")

// authFromBearer extracts and hashes a Bearer token from the Authorization
// header, then looks up the device it belongs to. It returns the matched
// device or ErrBadToken. It does NOT log the token, the header, or the
// visitor IP.
func authFromBearer(ctx context.Context, s *store.Store, r *http.Request) (store.Device, error) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return store.Device{}, ErrBadToken
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return store.Device{}, ErrBadToken
	}
	hash := store.HashToken(token)
	dev, err := s.LookupByTokenHash(ctx, hash)
	if err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			return store.Device{}, ErrBadToken
		}
		return store.Device{}, err
	}
	return dev, nil
}

// writeError writes the single stable error response shape: {"error": msg}.
// Never use it for 500 with the real internal error text — call
// writeInternalError instead.
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeInternalError writes the generic 500 response. The real error is
// logged by the caller (log once, at the top); the body is always the
// constant "internal error" so paths/SQL never leak.
func writeInternalError(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, "internal error")
}
