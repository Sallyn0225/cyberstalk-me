//go:build windows

// Package winsetup wires `agent.exe -setup`: the Win32 collectors on one side,
// the local configuration UI on the other.
//
// It is a separate package from setup for one reason worth stating plainly: it
// does not import the report package, and neither does anything it depends on.
// -setup must not be able to send anything anywhere, and keeping report out of
// this package's dependency graph turns that from a promise into something a
// test can check. See TestSetupModeCannotReport.
package winsetup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"cyberstalk.me/client-windows/internal/collect"
	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/setup"
)

// DefaultObserveInterval is how often the foreground window is sampled. About a
// second is what makes "switch to the app you want to configure and it appears"
// feel immediate without spinning.
const DefaultObserveInterval = time.Second

// Options configures a setup session.
type Options struct {
	// ConfigPath is the file to load from and save to.
	ConfigPath string
	// ObserveInterval defaults to DefaultObserveInterval.
	ObserveInterval time.Duration
	// IdleTimeout is passed through to the server.
	IdleTimeout time.Duration
	// Index renders the UI page. Without it only the API is served.
	Index func(token string) ([]byte, error)
	// Assets is the UI's static bundle.
	Assets fs.FS
	// OpenBrowser defaults to true; tests turn it off.
	OpenBrowser *bool
	// OnListen, if set, is called with the UI's address once the port is bound
	// and before the browser is opened.
	OnListen func(url string)
}

// Run serves the configuration UI until the user is done, the context is
// cancelled, or the session goes idle.
func Run(ctx context.Context, opts Options) error {
	if opts.ObserveInterval <= 0 {
		opts.ObserveInterval = DefaultObserveInterval
	}

	initial, notice := loadOrDefaults(opts.ConfigPath)
	catalog := setup.NewCatalog(setup.CatalogOptions{})

	srv, err := setup.New(setup.Options{
		ConfigPath:  opts.ConfigPath,
		Initial:     initial,
		Notice:      notice,
		Catalog:     catalog,
		Source:      foreground,
		IdleTimeout: opts.IdleTimeout,
		Index:       opts.Index,
		Assets:      opts.Assets,
	})
	if err != nil {
		return err
	}
	if err := srv.Listen(); err != nil {
		return err
	}

	// The observer stops with the session, not with the request that started it.
	observeCtx, stopObserving := context.WithCancel(ctx)
	defer stopObserving()
	go observe(observeCtx, catalog, opts.ObserveInterval)

	url := srv.URL()
	if opts.OnListen != nil {
		opts.OnListen(url)
	}
	if opts.OpenBrowser == nil || *opts.OpenBrowser {
		if err := openBrowser(url); err != nil {
			slog.Warn("setup: could not open a browser, open this yourself", "url", url, "err", err)
		}
	}
	// Printed unconditionally: a user whose browser opened still benefits from
	// having the address, and one whose browser did not needs it. The token is
	// not in the URL, so this is safe to have on screen.
	fmt.Println("configuration UI:", url)
	slog.Info("setup: serving the configuration UI", "url", url, "config", opts.ConfigPath)

	reason := srv.Serve(ctx)
	if reason != nil {
		slog.Info("setup: session ended", "reason", reason)
	}
	return nil
}

// loadOrDefaults reads the configuration, falling back to defaults with an
// explanation rather than refusing to start.
//
// This is the opposite of the resident agent's rule, on purpose: a bad config
// file must not lock the user out of the tool that exists to fix it.
func loadOrDefaults(path string) (*config.Config, string) {
	cfg, err := config.Load(path)
	if err == nil {
		return cfg, ""
	}

	fresh := &config.Config{}
	fresh.Normalize()

	if fileMissing(path) {
		return fresh, fmt.Sprintf("还没有 %s，这里从空白开始配，点「保存」时会创建它。", path)
	}
	// The message includes the load error because it names the offending key,
	// which is the whole point of showing it.
	return fresh, fmt.Sprintf("%s 读不出来，先从默认值开始。保存会覆盖它，原文件留在 %s。原因：%v",
		path, config.BackupPath(path), err)
}

// fileMissing reports whether path does not exist, as opposed to existing but
// being unreadable — the two get different explanations in the UI.
func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// observe samples the foreground window into the catalog.
//
// PRIVACY: this is the only place in the agent that reads a raw window title
// into a structure that outlives one report cycle. The title goes into the
// in-memory catalog and nowhere else — not to a log, not to disk, and not to
// the server, which this package cannot reach.
func observe(ctx context.Context, catalog *setup.Catalog, interval time.Duration) {
	sample := func() {
		snap := collect.Collect()
		catalog.Observe(snap.Process, snap.Title())
	}
	sample() // the first reading should not wait for the first tick

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sample()
		}
	}
}

// foreground reads the current foreground window for the live preview.
//
// Every field mapping.Resolve consumes is carried across, including
// IdleSeconds: a preview that ignored idle time would disagree with what the
// agent actually reports.
func foreground() setup.Foreground {
	snap := collect.Collect()
	return setup.Foreground{
		Process:     snap.Process,
		Title:       snap.Title,
		IdleSeconds: snap.IdleSeconds,
	}
}

// openBrowser opens url in the user's default browser.
//
// rundll32 url.dll,FileProtocolHandler is used instead of `cmd /c start`
// because it takes the URL as a single argument rather than handing it to the
// shell to re-parse.
func openBrowser(url string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	// Not waited on: the handler outlives this call by design.
	go func() { _ = cmd.Wait() }()
	return nil
}
