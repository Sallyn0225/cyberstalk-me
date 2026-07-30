package config

import (
	"errors"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/client-windows/internal/mapping"
)

// Load's messages are the first thing a user sees when the agent refuses to
// start. TestLoadErrors checks them loosely; these assert them verbatim, so
// routing them through ValidationError cannot quietly reword them.
func TestLoadErrorMessagesAreVerbatim(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantMsg    string
		wantFields []string
	}{
		{
			name:       "missing everything",
			body:       "",
			wantMsg:    "server_url, device_id, token must not be empty (run `server register-device` and paste its snippet)",
			wantFields: []string{"server_url", "device_id", "token"},
		},
		{
			name:       "server_url without a scheme",
			body:       "server_url: localhost:8080\ndevice_id: a\ntoken: b\n",
			wantMsg:    `server_url "localhost:8080" must be http(s)://host[:port] with no path`,
			wantFields: []string{"server_url"},
		},
		{
			name:       "negative interval",
			body:       minimalConfig + "interval: -5s\n",
			wantMsg:    "interval must be positive, got -5s",
			wantFields: []string{"interval"},
		},
		{
			name:       "negative idle_threshold",
			body:       minimalConfig + "idle_threshold: -1s\n",
			wantMsg:    "idle_threshold must be positive, got -1s",
			wantFields: []string{"idle_threshold"},
		},
		{
			name:       "rule without process",
			body:       minimalConfig + "rules:\n  - app: VS Code\n",
			wantMsg:    "rules[0].process must not be empty",
			wantFields: []string{"rules[0].process"},
		},
		{
			name:       "rule without app",
			body:       minimalConfig + "rules:\n  - process: code.exe\n",
			wantMsg:    "rules[0] (code.exe): app must not be empty",
			wantFields: []string{"rules[0].app"},
		},
		{
			name:       "duplicate rule",
			body:       minimalConfig + "rules:\n  - process: code.exe\n    app: A\n  - process: Code.EXE\n    app: B\n",
			wantMsg:    `rule 1: duplicate process "Code.EXE"`,
			wantFields: []string{"rules[1].process"},
		},
		{
			name:       "expose_title without a rule",
			body:       minimalConfig + "expose_title:\n  - chrome.exe\n",
			wantMsg:    `expose_title 0: process "chrome.exe" has no rule; add a rule for it first`,
			wantFields: []string{"expose_title[0]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.body)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("Load succeeded, want error %q", tt.wantMsg)
			}
			// The file path is part of the message: an agent started with
			// -config must say which file it was unhappy about.
			if want, got := "config "+path+": "+tt.wantMsg, err.Error(); got != want {
				t.Errorf("error =\n  %q\nwant\n  %q", got, want)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error is %T, want it to unwrap to *ValidationError", err)
			}
			if strings.Join(ve.Fields, ",") != strings.Join(tt.wantFields, ",") {
				t.Errorf("Fields = %v, want %v", ve.Fields, tt.wantFields)
			}
		})
	}
}

// A read or parse failure is not a ValidationError: nothing was validated, and
// there is no field to point at.
func TestLoadFileErrorsAreNotValidationErrors(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		wantSubstr string
	}{
		{"unknown key", minimalConfig + "sever_url: typo\n", "field sever_url not found"},
		{"unparseable duration", minimalConfig + "interval: soon\n", "invalid duration"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.body))
			if err == nil {
				t.Fatal("Load succeeded, want a parse error")
			}
			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantSubstr)
			}
			var ve *ValidationError
			if errors.As(err, &ve) {
				t.Errorf("parse error was reported as a ValidationError: %v", err)
			}
		})
	}
}

// Validate is the same check Load runs, callable on a configuration that never
// touched disk — an editor holding unsaved edits validates exactly what the
// agent would refuse to start with.
func TestValidateOnAnUnsavedConfig(t *testing.T) {
	cfg := &Config{
		ServerURL:     "http://localhost:8080",
		DeviceID:      "win-desktop",
		Token:         "abc123",
		Interval:      DefaultInterval,
		IdleThreshold: DefaultIdleThreshold,
		Rules: []mapping.Rule{
			{Process: "code.exe", App: "VS Code", Description: "在写代码"},
		},
	}
	if err := cfg.Validate("draft"); err != nil {
		t.Fatalf("Validate on a good config: %v", err)
	}

	cfg.Rules = append(cfg.Rules, mapping.Rule{Process: "code.exe", App: "重复的"})
	err := cfg.Validate("draft")
	if err == nil {
		t.Fatal("Validate accepted a duplicate process")
	}
	if want, got := `config draft: rule 1: duplicate process "code.exe"`, err.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error is %T, want *ValidationError", err)
	}
	if ve.Source != "draft" {
		t.Errorf("Source = %q, want %q", ve.Source, "draft")
	}
	if len(ve.Fields) != 1 || ve.Fields[0] != "rules[1].process" {
		t.Errorf("Fields = %v, want [rules[1].process]", ve.Fields)
	}
	// The mapping-level cause stays reachable for callers that want the exact
	// rule index without re-parsing the message.
	var re *mapping.RuleError
	if !errors.As(err, &re) {
		t.Fatalf("error does not unwrap to *mapping.RuleError")
	}
	if re.Index != 1 {
		t.Errorf("RuleError.Index = %d, want 1", re.Index)
	}
}

// Zero durations are a validation failure, not a silent default: defaults are
// applied by Load, and a configuration assembled in memory must say what it
// means.
func TestValidateRejectsZeroDurations(t *testing.T) {
	base := Config{ServerURL: "http://localhost:8080", DeviceID: "a", Token: "b"}

	cfg := base
	cfg.IdleThreshold = time.Minute
	if err := cfg.Validate("draft"); err == nil || !strings.Contains(err.Error(), "interval must be positive") {
		t.Errorf("zero interval: err = %v, want an interval complaint", err)
	}

	cfg = base
	cfg.Interval = time.Second
	if err := cfg.Validate("draft"); err == nil || !strings.Contains(err.Error(), "idle_threshold must be positive") {
		t.Errorf("zero idle_threshold: err = %v, want an idle_threshold complaint", err)
	}
}
