//go:build windows

package winsetup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/setup"
)

// AC8: setup mode must not be able to reach the network, and the way to know
// that is not to read the code but to ask the compiler what it links in.
//
// The report package is the only thing in the agent that talks to the server.
// If it ever appears in this package's dependency graph, someone has wired a
// path from the configuration UI to the outside world.
func TestSetupModeCannotReport(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "cyberstalk.me/client-windows/internal/winsetup").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasSuffix(dep, "client-windows/internal/report") {
			t.Errorf("setup mode depends on %s: it must not be able to report anything", dep)
		}
	}
	// A sanity check on the test itself: the graph must be non-trivial, or a
	// broken go list would make this pass by accident.
	if !strings.Contains(string(out), "client-windows/internal/setup") {
		t.Fatalf("dependency list looks wrong, got:\n%s", out)
	}
}

// R1.3: a missing config file is the normal starting point, not an error.
func TestLoadOrDefaultsWithNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg, notice := loadOrDefaults(path)
	if cfg == nil {
		t.Fatal("no configuration returned")
	}
	if notice == "" {
		t.Error("a missing file should be explained in the UI")
	}
	if !strings.Contains(notice, path) {
		t.Errorf("notice = %q, want it to name the file", notice)
	}
	// Defaults are applied, but the required identity fields stay empty so the
	// form marks them.
	if cfg.DefaultApp != config.DefaultApp || cfg.Interval != config.DefaultInterval {
		t.Errorf("defaults were not applied: %+v", cfg)
	}
	if cfg.ServerURL != "" || cfg.DeviceID != "" || cfg.Token != "" {
		t.Errorf("identity fields were invented: %+v", cfg)
	}
	// It must be usable as a starting draft even though it will not validate.
	if err := cfg.Validate("draft"); err == nil {
		t.Error("an empty configuration validated, want the required keys to be flagged")
	}
}

// R1.3: the tool that fixes a broken config must not be blocked by it.
func TestLoadOrDefaultsWithABrokenFile(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"unparseable yaml", "server_url: [unclosed\n"},
		{"unknown key", "sever_url: typo\n"},
		{"invalid regexp", "server_url: http://x:1\ndevice_id: a\ntoken: b\nrules:\n  - process: c.exe\n    app: C\n    title_patterns:\n      - match: \"([\"\n"},
		{"empty required keys", "device_name: 只写了个名字\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}

			cfg, notice := loadOrDefaults(path)
			if cfg == nil {
				t.Fatal("no configuration returned for a broken file")
			}
			if notice == "" {
				t.Error("a broken file must be explained in the UI")
			}
			// The user needs to know their file is not gone.
			if !strings.Contains(notice, config.BackupPath(path)) {
				t.Errorf("notice = %q, want it to mention the backup path", notice)
			}
			// It has to be good enough to build a server around.
			if _, err := setup.New(setup.Options{
				Initial: cfg,
				Catalog: setup.NewCatalog(setup.CatalogOptions{}),
				Source:  func() setup.Foreground { return setup.Foreground{} },
			}); err != nil {
				t.Errorf("the fallback configuration cannot start a session: %v", err)
			}
		})
	}
}

// A file that loads cleanly is used as-is, with nothing to explain.
func TestLoadOrDefaultsWithAGoodFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "server_url: http://localhost:8080\ndevice_id: win-desktop\ntoken: abc\ndevice_name: 台式机\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, notice := loadOrDefaults(path)
	if notice != "" {
		t.Errorf("notice = %q, want none for a good file", notice)
	}
	if cfg.DeviceName != "台式机" {
		t.Errorf("config was not loaded: %+v", cfg)
	}
}

// AC4: the preview source must carry every field mapping.Resolve consumes.
// Dropping IdleSeconds here would make the preview disagree with the report in
// a way no other test would catch.
func TestForegroundCarriesTheWholeSnapshot(t *testing.T) {
	fg := foreground()
	if fg.Title == nil {
		t.Error("Title getter is nil: the preview could never match a title pattern")
	}
	if fg.IdleSeconds < 0 {
		t.Errorf("IdleSeconds = %d", fg.IdleSeconds)
	}
	// Process is whatever is in the foreground on this machine, including "" on
	// a locked screen, so there is nothing to assert about its value — only
	// that reading it does not panic and that the getter is wired.
	_ = fg.Process
}

// The observer feeds the catalog on its own schedule and stops with its
// context.
func TestObserveFeedsTheCatalogAndStops(t *testing.T) {
	catalog := setup.NewCatalog(setup.CatalogOptions{})
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		observe(ctx, catalog, 5*time.Millisecond)
		close(done)
	}()

	// The first sample is taken without waiting for a tick.
	deadline := time.After(2 * time.Second)
	for {
		if catalog.Snapshot().Observations > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the observer never sampled anything")
		case <-time.After(2 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the observer did not stop with its context")
	}

	before := catalog.Snapshot().Observations
	time.Sleep(30 * time.Millisecond)
	if after := catalog.Snapshot().Observations; after != before {
		t.Errorf("observations went from %d to %d after cancellation", before, after)
	}
}
