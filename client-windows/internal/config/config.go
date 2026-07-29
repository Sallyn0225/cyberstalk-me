// Package config loads the Windows agent configuration from a YAML file.
//
// The first four keys (server_url, device_id, token, interval) are worded to
// match the snippet printed by `server register-device`, so that output can be
// pasted into config.yaml verbatim.
//
// Validation is fail-loud: a missing required value, an unparseable duration,
// an unknown key or a bad regular expression is a startup error, never a
// silent fallback to a default.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"cyberstalk.me/client-windows/internal/mapping"
)

// Defaults applied when the corresponding key is absent.
const (
	DefaultInterval      = 10 * time.Second
	DefaultIdleThreshold = 5 * time.Minute

	// Privacy defaults are deliberately generic: an unmapped process must
	// never reveal what it actually is.
	DefaultApp         = "某个应用"
	DefaultDescription = "使用中"
	DefaultLockedApp   = "已锁屏"
	DefaultLockedDesc  = "人不在"
)

// Config is the validated agent configuration.
type Config struct {
	// ServerURL is the backend base URL without a path (e.g.
	// "http://localhost:8080"). Trailing slashes are stripped.
	ServerURL string
	// DeviceID must match the device the token was issued for.
	DeviceID string
	// Token is the device bearer token. It is a secret: never log it.
	Token string
	// Interval is how often a report is sent. The server judges a device
	// offline after its own threshold, so a report goes out every interval
	// even when nothing changed.
	Interval time.Duration

	// DeviceName is local-only: the server re-stamps the name it was
	// registered with, so this is just log readability.
	DeviceName string
	// IdleThreshold is how long without keyboard/mouse input counts as idle.
	IdleThreshold time.Duration

	DefaultApp         string
	DefaultDescription string
	LockedApp          string
	LockedDescription  string

	Rules       []mapping.Rule
	ExposeTitle []string
}

// String redacts the token so a Config can be printed without leaking it.
// The value receiver makes this apply to both Config and *Config.
func (c Config) String() string {
	return fmt.Sprintf("config{server_url:%s device_id:%s token:<redacted> interval:%s rules:%d expose_title:%d}",
		c.ServerURL, c.DeviceID, c.Interval, len(c.Rules), len(c.ExposeTitle))
}

// MapperOptions projects the config onto the mapping package's options.
func (c Config) MapperOptions() mapping.Options {
	return mapping.Options{
		Rules:              c.Rules,
		ExposeTitle:        c.ExposeTitle,
		DefaultApp:         c.DefaultApp,
		DefaultDescription: c.DefaultDescription,
		LockedApp:          c.LockedApp,
		LockedDescription:  c.LockedDescription,
		IdleThreshold:      c.IdleThreshold,
	}
}

// ReportURL is the endpoint a report is POSTed to.
func (c Config) ReportURL() string {
	return c.ServerURL + "/api/v1/report"
}

// fileConfig is the on-disk shape. It is separate from Config so durations can
// be parsed from strings ("10s") and defaults applied in one place.
type fileConfig struct {
	ServerURL string   `yaml:"server_url"`
	DeviceID  string   `yaml:"device_id"`
	Token     string   `yaml:"token"`
	Interval  duration `yaml:"interval"`

	DeviceName    string   `yaml:"device_name"`
	IdleThreshold duration `yaml:"idle_threshold"`

	DefaultApp         string `yaml:"default_app"`
	DefaultDescription string `yaml:"default_description"`
	LockedApp          string `yaml:"locked_app"`
	LockedDescription  string `yaml:"locked_description"`

	Rules       []mapping.Rule `yaml:"rules"`
	ExposeTitle []string       `yaml:"expose_title"`
}

// duration accepts either a Go duration string ("10s", "1m30s") or a bare
// integer read as seconds, matching the server's env parsing.
type duration time.Duration

func (d *duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("invalid duration %q (examples: 10s, 1m30s, or bare seconds 10)", node.Value)
	}
	s = strings.TrimSpace(s)
	if parsed, err := time.ParseDuration(s); err == nil {
		*d = duration(parsed)
		return nil
	}
	// A bare integer is read as seconds, matching the server's env parsing.
	if secs, err := strconv.Atoi(s); err == nil {
		*d = duration(time.Duration(secs) * time.Second)
		return nil
	}
	return fmt.Errorf("invalid duration %q (examples: 10s, 1m30s, or bare seconds 10)", s)
}

// DefaultPath returns the config.yaml next to the executable. The working
// directory is unreliable for a double-clicked exe or a registry Run entry, so
// the exe directory is the anchor.
func DefaultPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	return filepath.Join(filepath.Dir(exe), "config.yaml"), nil
}

// Load reads, parses and validates the config file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var f fileConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	// Unknown keys are typos; fail loud instead of silently ignoring them.
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg := &Config{
		ServerURL:          strings.TrimRight(strings.TrimSpace(f.ServerURL), "/"),
		DeviceID:           strings.TrimSpace(f.DeviceID),
		Token:              strings.TrimSpace(f.Token),
		Interval:           time.Duration(f.Interval),
		DeviceName:         strings.TrimSpace(f.DeviceName),
		IdleThreshold:      time.Duration(f.IdleThreshold),
		DefaultApp:         orDefault(f.DefaultApp, DefaultApp),
		DefaultDescription: orDefault(f.DefaultDescription, DefaultDescription),
		LockedApp:          orDefault(f.LockedApp, DefaultLockedApp),
		LockedDescription:  orDefault(f.LockedDescription, DefaultLockedDesc),
		Rules:              f.Rules,
		ExposeTitle:        f.ExposeTitle,
	}
	if cfg.Interval == 0 {
		cfg.Interval = DefaultInterval
	}
	if cfg.IdleThreshold == 0 {
		cfg.IdleThreshold = DefaultIdleThreshold
	}
	// A rule may omit its description; fall back to the generic one rather
	// than reporting an empty string.
	for i := range cfg.Rules {
		if strings.TrimSpace(cfg.Rules[i].Description) == "" {
			cfg.Rules[i].Description = cfg.DefaultDescription
		}
	}

	if err := cfg.validate(path); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate(path string) error {
	var missing []string
	if c.ServerURL == "" {
		missing = append(missing, "server_url")
	}
	if c.DeviceID == "" {
		missing = append(missing, "device_id")
	}
	if c.Token == "" {
		missing = append(missing, "token")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config %s: %s must not be empty (run `server register-device` and paste its snippet)", path, strings.Join(missing, ", "))
	}

	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return fmt.Errorf("config %s: server_url %q is not a valid URL: %w", path, c.ServerURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("config %s: server_url %q must be http(s)://host[:port] with no path", path, c.ServerURL)
	}

	if c.Interval <= 0 {
		return fmt.Errorf("config %s: interval must be positive, got %s", path, c.Interval)
	}
	if c.IdleThreshold <= 0 {
		return fmt.Errorf("config %s: idle_threshold must be positive, got %s", path, c.IdleThreshold)
	}

	for i, r := range c.Rules {
		if strings.TrimSpace(r.Process) == "" {
			return fmt.Errorf("config %s: rules[%d].process must not be empty", path, i)
		}
		if strings.TrimSpace(r.App) == "" {
			return fmt.Errorf("config %s: rules[%d] (%s): app must not be empty", path, i, r.Process)
		}
	}

	// Building the mapper validates rule keys, duplicates, regular expressions
	// and expose_title entries — one implementation, checked at startup.
	if _, err := mapping.New(c.MapperOptions()); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}
