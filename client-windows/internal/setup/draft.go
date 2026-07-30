package setup

import (
	"strings"
	"time"

	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/mapping"
)

// draftSource is what a validation error calls an unsaved configuration, in the
// place a file path would otherwise appear.
const draftSource = "draft"

// Draft is a configuration as it travels between the UI and this process.
//
// The keys are spelled exactly like the YAML ones so that what a user sees in
// the form and what ends up in config.yaml line up. Durations are strings for
// the same reason: "10s", not a nanosecond count.
//
// It carries the device token in the clear. That is unavoidable — the form has
// to show it — and is why every route sits behind the token and origin guards.
type Draft struct {
	ServerURL string `json:"server_url"`
	DeviceID  string `json:"device_id"`
	Token     string `json:"token"`
	Interval  string `json:"interval"`

	DeviceName    string `json:"device_name"`
	IdleThreshold string `json:"idle_threshold"`

	DefaultApp         string `json:"default_app"`
	DefaultDescription string `json:"default_description"`
	LockedApp          string `json:"locked_app"`
	LockedDescription  string `json:"locked_description"`

	Rules       []mapping.Rule `json:"rules"`
	ExposeTitle []string       `json:"expose_title"`
}

// DraftOf renders a configuration for the UI.
func DraftOf(cfg *config.Config) Draft {
	d := Draft{
		ServerURL:          cfg.ServerURL,
		DeviceID:           cfg.DeviceID,
		Token:              cfg.Token,
		Interval:           config.FormatDuration(cfg.Interval),
		DeviceName:         cfg.DeviceName,
		IdleThreshold:      config.FormatDuration(cfg.IdleThreshold),
		DefaultApp:         cfg.DefaultApp,
		DefaultDescription: cfg.DefaultDescription,
		LockedApp:          cfg.LockedApp,
		LockedDescription:  cfg.LockedDescription,
		Rules:              cfg.Rules,
		ExposeTitle:        cfg.ExposeTitle,
	}
	// A form binds to arrays; null would make the UI guard every access.
	if d.Rules == nil {
		d.Rules = []mapping.Rule{}
	}
	if d.ExposeTitle == nil {
		d.ExposeTitle = []string{}
	}
	return d
}

// Config converts a draft back into a configuration.
//
// Only the durations can fail here, because only they have a syntax of their
// own. Everything else — trimming, defaults, whether the result is usable at
// all — is config.Normalize and config.Validate, deliberately not reimplemented:
// the UI must accept exactly what the agent will start with.
func (d Draft) Config() (*config.Config, error) {
	interval, err := parseDraftDuration(d.Interval, "interval")
	if err != nil {
		return nil, err
	}
	idle, err := parseDraftDuration(d.IdleThreshold, "idle_threshold")
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{
		ServerURL:          d.ServerURL,
		DeviceID:           d.DeviceID,
		Token:              d.Token,
		Interval:           interval,
		DeviceName:         d.DeviceName,
		IdleThreshold:      idle,
		DefaultApp:         d.DefaultApp,
		DefaultDescription: d.DefaultDescription,
		LockedApp:          d.LockedApp,
		LockedDescription:  d.LockedDescription,
		Rules:              d.Rules,
		ExposeTitle:        d.ExposeTitle,
	}
	cfg.Normalize()
	return cfg, nil
}

// parseDraftDuration treats an empty field as "not set" and leaves it zero, so
// Normalize fills in the same default a missing YAML key would get. A field the
// user cleared must not become an error.
func parseDraftDuration(s, field string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	d, err := config.ParseDuration(s)
	if err != nil {
		return 0, &config.ValidationError{
			Source: draftSource, Fields: []string{field}, Message: err.Error(), Err: err,
		}
	}
	return d, nil
}
