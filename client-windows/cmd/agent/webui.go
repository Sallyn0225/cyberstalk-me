package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

// The built configuration UI, compiled into the exe so `go build` produces one
// file that carries its own interface.
//
// The bundle is committed to the repository because client-windows ships no
// release artifact: users build the agent themselves, and a clone without the
// bundle would produce an exe that serves a blank page. CI checks the committed
// output against a fresh build so it cannot silently go stale.
//
//go:embed all:webui
var webuiFS embed.FS

// tokenPlaceholder is what webui/index.html carries in place of the session
// token, quotes included, so the replacement stays inside the string literal.
const tokenPlaceholder = `"__SETUP_TOKEN_PLACEHOLDER__"`

// setupAssets is the UI's static bundle.
func setupAssets() fs.FS {
	assets, err := fs.Sub(webuiFS, "webui")
	if err != nil {
		// Only reachable if the embed directive above stops matching, which is
		// a build-time mistake rather than a runtime condition.
		panic(fmt.Sprintf("embedded webui is not a directory: %v", err))
	}
	return assets
}

// setupIndex renders the entry page with this session's token embedded.
//
// The token goes into the page rather than into the URL: a URL ends up in
// browser history and in any Referer the page later sends, and this token
// guards an API that can read raw window titles.
func setupIndex(token string) ([]byte, error) {
	page, err := webuiFS.ReadFile("webui/index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded index.html: %w", err)
	}

	quoted, err := json.Marshal(token)
	if err != nil {
		return nil, fmt.Errorf("encode session token: %w", err)
	}

	injected := bytes.Replace(page, []byte(tokenPlaceholder), quoted, 1)
	if bytes.Equal(injected, page) {
		// The page would load and then fail every request with a 401, which is
		// a confusing way to discover that the bundle is stale or edited.
		return nil, fmt.Errorf("embedded index.html has no %s to replace; rebuild webui", tokenPlaceholder)
	}
	return injected, nil
}
