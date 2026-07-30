package config

import (
	"os"
	"strings"
	"testing"
	"time"

	// The tz database is embedded here for the same reason main.go embeds it:
	// DISPLAY_TIMEZONE is resolved with time.LoadLocation, and a machine
	// without a system tz database (an alpine container, a bare CI image)
	// would otherwise fail these tests for an unrelated reason.
	_ "time/tzdata"
)

// loadWith runs Load with exactly the given env vars set and everything else
// cleared, so a developer's own environment cannot change the result.
func loadWith(t *testing.T, env map[string]string) (Config, error) {
	t.Helper()
	for _, key := range []string{
		"ADDR", "SQLITE_PATH", "OFFLINE_THRESHOLD", "SCAN_INTERVAL",
		"USAGE_RETENTION_DAYS", "USAGE_PRUNE_INTERVAL", "USAGE_MAX_GAP", "DISPLAY_TIMEZONE",
	} {
		// t.Setenv registers the restore-on-cleanup; Unsetenv then makes the
		// var genuinely absent, which is not the same as set-but-blank for
		// ADDR and SQLITE_PATH.
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
	for key, value := range env {
		t.Setenv(key, value)
	}
	return Load()
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := loadWith(t, nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr = %q, want :8080", cfg.Addr)
	}
	if cfg.SQLitePath != "cyberstalk.db" {
		t.Errorf("SQLitePath = %q, want cyberstalk.db", cfg.SQLitePath)
	}
	if cfg.OfflineThreshold != 60*time.Second {
		t.Errorf("OfflineThreshold = %s, want 1m", cfg.OfflineThreshold)
	}
	if cfg.ScanInterval != 5*time.Second {
		t.Errorf("ScanInterval = %s, want 5s", cfg.ScanInterval)
	}
	if cfg.UsageRetentionDays != 365 {
		t.Errorf("UsageRetentionDays = %d, want 365", cfg.UsageRetentionDays)
	}
	if cfg.UsagePruneInterval != time.Hour {
		t.Errorf("UsagePruneInterval = %s, want 1h", cfg.UsagePruneInterval)
	}
	if cfg.DisplayTimezone != "Asia/Shanghai" {
		t.Errorf("DisplayTimezone = %q, want Asia/Shanghai", cfg.DisplayTimezone)
	}
	if cfg.Location == nil || cfg.Location.String() != "Asia/Shanghai" {
		t.Errorf("Location = %v, want Asia/Shanghai", cfg.Location)
	}
}

func TestUsageMaxGapDefaultsToOfflineThreshold(t *testing.T) {
	cfg, err := loadWith(t, map[string]string{"OFFLINE_THRESHOLD": "90s"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.UsageMaxGap != 90*time.Second {
		t.Fatalf("UsageMaxGap = %s, want it to follow OFFLINE_THRESHOLD (90s)", cfg.UsageMaxGap)
	}

	// An explicit value wins: someone can widen the offline tolerance without
	// widening how long a silence is still counted as usage.
	cfg, err = loadWith(t, map[string]string{"OFFLINE_THRESHOLD": "10m", "USAGE_MAX_GAP": "2m"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.UsageMaxGap != 2*time.Minute {
		t.Fatalf("UsageMaxGap = %s, want 2m", cfg.UsageMaxGap)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantMsg string
	}{
		{"empty addr", map[string]string{"ADDR": " "}, "ADDR"},
		{"bad offline threshold", map[string]string{"OFFLINE_THRESHOLD": "soon"}, "OFFLINE_THRESHOLD"},
		{"negative offline threshold", map[string]string{"OFFLINE_THRESHOLD": "-5s"}, "OFFLINE_THRESHOLD"},
		{"bad retention days", map[string]string{"USAGE_RETENTION_DAYS": "a year"}, "USAGE_RETENTION_DAYS"},
		{"zero retention days", map[string]string{"USAGE_RETENTION_DAYS": "0"}, "USAGE_RETENTION_DAYS"},
		{"negative retention days", map[string]string{"USAGE_RETENTION_DAYS": "-1"}, "USAGE_RETENTION_DAYS"},
		{"bad prune interval", map[string]string{"USAGE_PRUNE_INTERVAL": "hourly"}, "USAGE_PRUNE_INTERVAL"},
		{"negative prune interval", map[string]string{"USAGE_PRUNE_INTERVAL": "-1h"}, "USAGE_PRUNE_INTERVAL"},
		{"negative max gap", map[string]string{"USAGE_MAX_GAP": "-30s"}, "USAGE_MAX_GAP"},
		{"unknown timezone", map[string]string{"DISPLAY_TIMEZONE": "Not/AZone"}, "DISPLAY_TIMEZONE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadWith(t, tt.env)
			if err == nil {
				t.Fatalf("want an error naming %s, got nil", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error %q does not name %s", err, tt.wantMsg)
			}
		})
	}
}

func TestBareSecondsAccepted(t *testing.T) {
	cfg, err := loadWith(t, map[string]string{"USAGE_MAX_GAP": "120"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.UsageMaxGap != 2*time.Minute {
		t.Fatalf("UsageMaxGap = %s, want 2m", cfg.UsageMaxGap)
	}
}
