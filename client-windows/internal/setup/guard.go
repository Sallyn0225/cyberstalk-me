package setup

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// newToken returns the one-time bearer token for this setup session.
//
// It exists for exactly as long as the process does. Anything reaching the API
// has to present it, which is what stops another program on this machine from
// reading the window titles the catalog holds.
func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate setup token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// guard rejects anything that is not this session's UI.
//
// Two independent checks, because they stop different attackers:
//
//   - The bearer token stops any other local program. It is not a cookie, so a
//     browser will never attach it to a cross-site request on its own.
//   - The Origin/Host check stops a page on another site that a browser has
//     been talked into loading. It is defence in depth behind the token, and
//     costs nothing.
//
// Nothing here logs a header or a body: the Authorization header is the token
// and the bodies contain raw window titles.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.originAllowed(r) {
			slog.Debug("setup: rejected cross-origin request", "path", r.URL.Path)
			writeError(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		if !s.tokenAllowed(r) {
			slog.Debug("setup: rejected request without a valid token", "path", r.URL.Path)
			writeError(w, http.StatusUnauthorized, "missing or invalid setup token")
			return
		}
		s.touch()
		next.ServeHTTP(w, r)
	})
}

// tokenAllowed reports whether the request carries this session's token.
func (s *Server) tokenAllowed(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	value, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	// Constant time so the token cannot be recovered a byte at a time.
	return subtle.ConstantTimeCompare([]byte(strings.TrimSpace(value)), []byte(s.token)) == 1
}

// originAllowed reports whether the request came from this server's own page.
//
// A missing Origin is allowed: browsers omit it on same-origin GETs, and a
// forged one is no easier to send than a forged token. Host is checked in both
// cases so a DNS-rebinding answer that resolves some attacker domain to
// 127.0.0.1 does not reach the API.
func (s *Server) originAllowed(r *http.Request) bool {
	if !s.hostAllowed(r.Host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" {
		return false
	}
	return s.hostAllowed(u.Host)
}

// hostAllowed reports whether host is this server's own address. The loopback
// address can be spelled a few ways and a browser will use whichever one the
// URL had.
func (s *Server) hostAllowed(host string) bool {
	for _, allowed := range s.allowedHosts {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
}

// loopbackHosts is every spelling of "this server" that a browser might send,
// given a URL pointing at port.
func loopbackHosts(port string) []string {
	return []string{
		"127.0.0.1:" + port,
		"localhost:" + port,
		"[::1]:" + port,
	}
}
