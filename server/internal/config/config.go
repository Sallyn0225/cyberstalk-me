// Package config loads server configuration from the environment.
//
// All knobs are env-based (optionally a .env file loaded by main) so the
// server stays a single binary with no config files of its own. Values are
// validated in Load and a missing required value is a fatal startup error
// (returned from Load, fatal in main).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the validated server configuration.
type Config struct {
	// Addr is the listen address for the HTTP server. Default ":8080".
	Addr string
	// SQLitePath is the filesystem path to the SQLite database, or ":memory:"
	// for an in-memory database (used by tests).
	SQLitePath string
	// OfflineThreshold is how long since last_seen_at a device may be silent
	// before it is judged offline.
	OfflineThreshold time.Duration
	// ScanInterval is how often the state tracker scans for newly-offline
	// devices.
	ScanInterval time.Duration
	// UsageRetentionDays is how many days of hourly usage buckets are kept.
	UsageRetentionDays int
	// UsagePruneInterval is how often buckets older than the retention window
	// are deleted.
	UsagePruneInterval time.Duration
	// UsageMaxGap is the longest silence still attributed to the last observed
	// activity. A longer gap is a blind spot: nothing is attributed at all, so
	// a powered-off night does not become eight hours of usage.
	UsageMaxGap time.Duration
	// DisplayTimezone is the IANA name the site's local days and hours are
	// computed in, kept alongside Location so it can be echoed on the wire.
	DisplayTimezone string
	// Location is DisplayTimezone resolved. Loaded once at startup so no
	// request has to.
	Location *time.Location
}

// Load reads configuration from the environment and applies defaults.
// Missing optional values get sensible defaults; invalid values return an
// error. A required value that is missing also returns an error.
func Load() (Config, error) {
	offlineThreshold, err := getEnvDuration("OFFLINE_THRESHOLD", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	scanInterval, err := getEnvDuration("SCAN_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	// USAGE_MAX_GAP defaults to OFFLINE_THRESHOLD: "silent long enough to show
	// as offline" and "silent long enough that we no longer know what the
	// device was doing" are the same line by default. It has to be read after
	// offlineThreshold, or the default would always be zero and every interval
	// would be discarded as a gap.
	usageMaxGap, err := getEnvDuration("USAGE_MAX_GAP", offlineThreshold)
	if err != nil {
		return Config{}, err
	}
	usagePruneInterval, err := getEnvDuration("USAGE_PRUNE_INTERVAL", time.Hour)
	if err != nil {
		return Config{}, err
	}
	usageRetentionDays, err := getEnvInt("USAGE_RETENTION_DAYS", 365)
	if err != nil {
		return Config{}, err
	}
	displayTimezone := getEnv("DISPLAY_TIMEZONE", "Asia/Shanghai")
	location, err := time.LoadLocation(displayTimezone)
	if err != nil {
		return Config{}, fmt.Errorf("DISPLAY_TIMEZONE: invalid IANA timezone %q: %w", displayTimezone, err)
	}

	cfg := Config{
		Addr:               getEnv("ADDR", ":8080"),
		SQLitePath:         getEnv("SQLITE_PATH", "cyberstalk.db"),
		OfflineThreshold:   offlineThreshold,
		ScanInterval:       scanInterval,
		UsageRetentionDays: usageRetentionDays,
		UsagePruneInterval: usagePruneInterval,
		UsageMaxGap:        usageMaxGap,
		DisplayTimezone:    displayTimezone,
		Location:           location,
	}

	if strings.TrimSpace(cfg.Addr) == "" {
		return Config{}, errors.New("ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.SQLitePath) == "" {
		return Config{}, errors.New("SQLITE_PATH must not be empty")
	}
	if cfg.OfflineThreshold <= 0 {
		return Config{}, fmt.Errorf("OFFLINE_THRESHOLD must be positive, got %s", cfg.OfflineThreshold)
	}
	if cfg.ScanInterval <= 0 {
		return Config{}, fmt.Errorf("SCAN_INTERVAL must be positive, got %s", cfg.ScanInterval)
	}
	if cfg.UsageRetentionDays <= 0 {
		return Config{}, fmt.Errorf("USAGE_RETENTION_DAYS must be positive, got %d", cfg.UsageRetentionDays)
	}
	if cfg.UsagePruneInterval <= 0 {
		return Config{}, fmt.Errorf("USAGE_PRUNE_INTERVAL must be positive, got %s", cfg.UsagePruneInterval)
	}
	if cfg.UsageMaxGap <= 0 {
		return Config{}, fmt.Errorf("USAGE_MAX_GAP must be positive, got %s", cfg.UsageMaxGap)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}

// getEnvDuration reads a duration-valued env var. If the var is unset or
// blank it returns def. A bare integer is accepted as seconds for
// ergonomics (e.g. OFFLINE_THRESHOLD=90). Any other unparseable value is an
// error rather than a silent fallback, so a typo'd config fails loud.
func getEnvDuration(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def, nil
	}
	trimmed := strings.TrimSpace(v)
	if d, err := time.ParseDuration(trimmed); err == nil {
		return d, nil
	}
	if secs, err := strconv.Atoi(trimmed); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	return 0, fmt.Errorf("%s: invalid duration %q (examples: 90s, 1m30s, or bare seconds 90)", key, v)
}

// getEnvInt reads an integer-valued env var. Unset or blank returns def; an
// unparseable value is an error rather than a silent fallback, for the same
// reason as getEnvDuration — a typo'd config must fail loud.
func getEnvInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", key, v)
	}
	return n, nil
}
