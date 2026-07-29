package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig writes body to a temp file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const minimalConfig = `
server_url: http://localhost:8080
device_id: win-desktop
token: deadbeef
`

func TestLoadFull(t *testing.T) {
	path := writeConfig(t, `
server_url: https://cyberstalk.me/
device_id: win-desktop
token: abc123
interval: 30s
device_name: 我的台式机
idle_threshold: 2m
default_app: 某个应用
default_description: 使用中
locked_app: 已锁屏
locked_description: 人不在
rules:
  - process: Code.exe
    app: VS Code
    description: 在写代码
  - process: chrome.exe
    app: Chrome
    description: 在上网
    title_patterns:
      - match: "(?i)youtube"
        description: 在看视频
expose_title:
  - chrome.exe
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Trailing slash is stripped so the report URL joins cleanly.
	if cfg.ServerURL != "https://cyberstalk.me" {
		t.Errorf("ServerURL = %q, want %q", cfg.ServerURL, "https://cyberstalk.me")
	}
	if got, want := cfg.ReportURL(), "https://cyberstalk.me/api/v1/report"; got != want {
		t.Errorf("ReportURL() = %q, want %q", got, want)
	}
	if cfg.DeviceID != "win-desktop" || cfg.Token != "abc123" {
		t.Errorf("identity = %q/%q", cfg.DeviceID, cfg.Token)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("Interval = %s, want 30s", cfg.Interval)
	}
	if cfg.IdleThreshold != 2*time.Minute {
		t.Errorf("IdleThreshold = %s, want 2m", cfg.IdleThreshold)
	}
	if cfg.DeviceName != "我的台式机" {
		t.Errorf("DeviceName = %q", cfg.DeviceName)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("len(Rules) = %d, want 2", len(cfg.Rules))
	}
	if cfg.Rules[0].Process != "Code.exe" || cfg.Rules[0].App != "VS Code" {
		t.Errorf("Rules[0] = %+v", cfg.Rules[0])
	}
	if len(cfg.Rules[1].TitlePatterns) != 1 || cfg.Rules[1].TitlePatterns[0].Description != "在看视频" {
		t.Errorf("Rules[1].TitlePatterns = %+v", cfg.Rules[1].TitlePatterns)
	}
	if len(cfg.ExposeTitle) != 1 || cfg.ExposeTitle[0] != "chrome.exe" {
		t.Errorf("ExposeTitle = %v", cfg.ExposeTitle)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("Interval = %s, want %s", cfg.Interval, DefaultInterval)
	}
	if cfg.IdleThreshold != DefaultIdleThreshold {
		t.Errorf("IdleThreshold = %s, want %s", cfg.IdleThreshold, DefaultIdleThreshold)
	}
	if cfg.DefaultApp != DefaultApp || cfg.DefaultDescription != DefaultDescription {
		t.Errorf("defaults = %q/%q", cfg.DefaultApp, cfg.DefaultDescription)
	}
	if cfg.LockedApp != DefaultLockedApp || cfg.LockedDescription != DefaultLockedDesc {
		t.Errorf("locked defaults = %q/%q", cfg.LockedApp, cfg.LockedDescription)
	}
}

func TestLoadRuleDescriptionFallsBackToDefault(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalConfig+`
default_description: 在忙
rules:
  - process: code.exe
    app: VS Code
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Rules[0].Description != "在忙" {
		t.Errorf("Rules[0].Description = %q, want %q", cfg.Rules[0].Description, "在忙")
	}
}

func TestLoadDurationFormats(t *testing.T) {
	tests := []struct {
		name string
		body string
		want time.Duration
	}{
		{"go duration", "interval: 1m30s", 90 * time.Second},
		{"seconds string", "interval: 45s", 45 * time.Second},
		{"bare integer seconds", "interval: 20", 20 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Load(writeConfig(t, minimalConfig+tt.body+"\n"))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Interval != tt.want {
				t.Errorf("Interval = %s, want %s", cfg.Interval, tt.want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "missing server_url",
			body:    "device_id: a\ntoken: b\n",
			wantMsg: "server_url",
		},
		{
			name:    "missing device_id and token",
			body:    "server_url: http://localhost:8080\n",
			wantMsg: "device_id, token",
		},
		{
			name:    "empty file",
			body:    "",
			wantMsg: "server_url",
		},
		{
			name:    "server_url with no scheme",
			body:    "server_url: localhost:8080\ndevice_id: a\ntoken: b\n",
			wantMsg: "must be http(s)",
		},
		{
			name:    "negative interval",
			body:    minimalConfig + "interval: -5s\n",
			wantMsg: "interval must be positive",
		},
		{
			name:    "unparseable interval",
			body:    minimalConfig + "interval: soon\n",
			wantMsg: "invalid duration",
		},
		{
			name:    "zero idle_threshold",
			body:    minimalConfig + "idle_threshold: -1s\n",
			wantMsg: "idle_threshold must be positive",
		},
		{
			name:    "unknown key",
			body:    minimalConfig + "sever_url: typo\n",
			wantMsg: "field sever_url not found",
		},
		{
			name:    "rule without process",
			body:    minimalConfig + "rules:\n  - app: VS Code\n",
			wantMsg: "rules[0].process must not be empty",
		},
		{
			name:    "rule without app",
			body:    minimalConfig + "rules:\n  - process: code.exe\n",
			wantMsg: "app must not be empty",
		},
		{
			name:    "duplicate rule",
			body:    minimalConfig + "rules:\n  - process: code.exe\n    app: A\n  - process: Code.EXE\n    app: B\n",
			wantMsg: "duplicate process",
		},
		{
			name:    "bad title pattern regexp",
			body:    minimalConfig + "rules:\n  - process: code.exe\n    app: VS Code\n    title_patterns:\n      - match: \"([\"\n        description: 坏了\n",
			wantMsg: "compile",
		},
		{
			name:    "expose_title without a rule",
			body:    minimalConfig + "expose_title:\n  - chrome.exe\n",
			wantMsg: "has no rule",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeConfig(t, tt.body))
			if err == nil {
				t.Fatalf("Load succeeded, want error containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("Load succeeded on a missing file, want error")
	}
	if !strings.Contains(err.Error(), "read config") {
		t.Errorf("error = %v, want it to mention read config", err)
	}
}

// The token is a secret: printing a Config must never reveal it.
func TestStringRedactsToken(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, s := range []string{cfg.String(), (*cfg).String()} {
		if strings.Contains(s, "deadbeef") {
			t.Errorf("String() leaked the token: %s", s)
		}
		if !strings.Contains(s, "<redacted>") {
			t.Errorf("String() = %s, want a redacted token", s)
		}
	}
}

func TestDefaultPathIsNextToExecutable(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	if want := filepath.Join(filepath.Dir(exe), "config.yaml"); path != want {
		t.Errorf("DefaultPath() = %q, want %q", path, want)
	}
}
