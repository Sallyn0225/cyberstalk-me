// Package report sends sanitized device state to the server and retries on
// failure.
//
// It is the last stage of the agent pipeline: collect -> mapping -> assemble a
// shared.ReportPayload -> report. The package only ever sees the already-
// sanitized payload; the raw window title never reaches it (the mapping
// package is the privacy boundary). report imports only the shared contract
// and the standard library, so it builds and unit-tests on any platform — the
// Win32 collectors live in a separate windows-only package.
package report

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"cyberstalk.me/shared"
)

// ErrPermanent marks a report failure caused by the client's own
// configuration — a bad token, a wrong device id, or a malformed body. Retrying
// immediately will not help, so the loop backs off straight to the maximum
// instead of hammering the server. Send wraps it (errors.Is) into the returned
// error.
var ErrPermanent = errors.New("permanent report error: fix the config and restart")

// Client posts report payloads to a single endpoint. The *http.Client is
// reused across sends so connections are kept alive.
type Client struct {
	HTTP  *http.Client
	URL   string
	Token string
}

// NewClient returns a Client posting to url with the given bearer token. The
// HTTP client has a 10s timeout and the default transport (connection reuse).
func NewClient(url, token string) *Client {
	return &Client{
		HTTP:  &http.Client{Timeout: 10 * time.Second},
		URL:   url,
		Token: token,
	}
}

// Send posts one payload. It returns nil on 204. A 400/401 is a permanent
// configuration error (wrapped with ErrPermanent); a 5xx, a network error, or
// a timeout is a retryable error. The token never appears in the returned
// error: it lives only in the Authorization header, and the request URL carries
// no secret.
func (c *Client) Send(ctx context.Context, p shared.ReportPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		// Network failure, DNS, timeout, or ctx cancellation — all retryable.
		return fmt.Errorf("report: %w", err)
	}
	defer resp.Body.Close()
	// Drain so the underlying connection can be reused; the body is empty on
	// 204 and ignored otherwise (the server's errors carry only a short text).
	_, _ = io.Copy(io.Discard, resp.Body)

	switch {
	case resp.StatusCode == http.StatusNoContent:
		return nil
	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized:
		// 400/401: the client is misconfigured (bad token, wrong device id,
		// malformed body). Retrying fast won't fix it.
		return fmt.Errorf("%w: server status %d", ErrPermanent, resp.StatusCode)
	case resp.StatusCode >= 500:
		// Server-side or transient — worth retrying.
		return fmt.Errorf("report: server status %d", resp.StatusCode)
	default:
		// A status this endpoint never returns means the URL points somewhere
		// unexpected; treat it as permanent so the loop backs off hard.
		return fmt.Errorf("%w: unexpected server status %d", ErrPermanent, resp.StatusCode)
	}
}
