package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/client-windows/internal/mapping"
)

// fullConfig exercises every key, several title patterns, non-ASCII text and
// values that are hostile to YAML round-tripping.
func fullConfig() *Config {
	return &Config{
		ServerURL:          "https://cyberstalk.me",
		DeviceID:           "win-desktop",
		Token:              "0123456789", // all digits: must not come back as an int
		Interval:           30 * time.Second,
		DeviceName:         "我的台式机",
		IdleThreshold:      2 * time.Minute,
		DefaultApp:         "某个应用",
		DefaultDescription: "使用中",
		LockedApp:          "已锁屏",
		LockedDescription:  "人不在",
		Rules: []mapping.Rule{
			{Process: "code.exe", App: "VS Code", Description: "在写代码"},
			{
				Process:     "chrome.exe",
				App:         "Chrome",
				Description: "在上网: 大概吧", // a colon, which YAML would otherwise read as a mapping
				TitlePatterns: []mapping.TitlePattern{
					{Match: `(?i)youtube|bilibili`, Description: "在看视频"},
					{Match: `^\[Debug\]:\s+#\d+`, Description: "在调试"}, // regexp metacharacters and a #
					{Match: `(?i)^\s*github\.com/.*\z`, Description: "在看代码"},
				},
			},
			{Process: "term.exe", App: "终端", Description: "在敲命令"},
		},
		ExposeTitle: []string{"term.exe"},
	}
}

// AC5: a configuration written by Save must load back with the same meaning.
func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := fullConfig()
	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load what Save wrote: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the config:\n got %+v\nwant %+v", got, want)
	}
}

// Values that YAML would happily reinterpret as another type have to survive as
// the strings they are.
func TestSaveLoadRoundTripHostileValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"digits", "12345"},
		{"boolean-looking", "true"},
		{"null-looking", "null"},
		{"tilde", "~"},
		{"leading hash", "#1 最爱"},
		{"leading dash", "- 待办"},
		{"colon space", "标题: 副标题"},
		{"sexagesimal", "1:30"},
		{"trailing space", "在写代码 "},
		{"emoji", "在摸鱼 🐟"},
		{"quotes", `他说"好"，我说'行'`},
		{"backslash", `C:\path\to`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			want := fullConfig()
			want.DefaultDescription = tt.value
			want.Rules[0].Description = tt.value
			want.Rules[1].TitlePatterns[0].Description = tt.value
			if err := Save(path, want); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			// Load trims the top-level scalars, so the comparison is against
			// what Load normalizes to — the point here is that the value
			// survives YAML, not that Load stops trimming.
			if want := strings.TrimSpace(tt.value); got.DefaultDescription != want {
				t.Errorf("default_description = %q, want %q", got.DefaultDescription, want)
			}
			if got.Rules[0].Description != tt.value {
				t.Errorf("rules[0].description = %q, want %q", got.Rules[0].Description, tt.value)
			}
			if got.Rules[1].TitlePatterns[0].Description != tt.value {
				t.Errorf("title_patterns[0].description = %q, want %q", got.Rules[1].TitlePatterns[0].Description, tt.value)
			}
		})
	}
}

// An empty rule set is a legitimate starting point: the setup UI writes one
// before the user has configured anything.
func TestSaveLoadRoundTripEmptyCollections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := fullConfig()
	cfg.Rules = nil
	cfg.ExposeTitle = nil
	cfg.DeviceName = ""
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, want := range []string{"rules: []", "expose_title: []"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("output does not contain %q:\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "device_name") {
		t.Errorf("an empty device_name should be omitted, got:\n%s", data)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Rules) != 0 || len(got.ExposeTitle) != 0 || got.DeviceName != "" {
		t.Errorf("round trip = %+v, want empty rules/expose_title/device_name", got)
	}
}

// The first save has no previous file to preserve or back up.
func TestSaveCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := Save(path, fullConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config was not created: %v", err)
	}
	if _, err := os.Stat(BackupPath(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a backup was created on first save (err = %v)", err)
	}
	assertNoTempFile(t, dir)
}

// The previous file survives verbatim, which is the escape hatch for everything
// a regenerated file loses (hand-written comments, key order).
func TestSaveBacksUpPreviousFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := "# hand written, with its own comments\n" + minimalConfig
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if err := Save(path, fullConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}

	backup, err := os.ReadFile(BackupPath(path))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backup) != original {
		t.Errorf("backup =\n%s\nwant\n%s", backup, original)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	if got.DeviceName != "我的台式机" {
		t.Errorf("config was not replaced: %+v", got)
	}
	assertNoTempFile(t, dir)
}

// Saving twice must keep working: the second save overwrites the first backup.
func TestSaveTwiceOverwritesBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := fullConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	cfg.DeviceName = "第二次"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.DeviceName != "第二次" {
		t.Errorf("DeviceName = %q, want 第二次", got.DeviceName)
	}
	// The backup now holds the first generated file, not the hand-written one.
	backup, err := Load(BackupPath(path))
	if err != nil {
		t.Fatalf("Load backup: %v", err)
	}
	if backup.DeviceName != "我的台式机" {
		t.Errorf("backup DeviceName = %q, want 我的台式机", backup.DeviceName)
	}
	assertNoTempFile(t, dir)
}

// A configuration the agent would refuse to start with must never reach disk.
func TestSaveRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantMsg string
	}{
		{"empty token", func(c *Config) { c.Token = "" }, "token must not be empty"},
		{"duplicate process", func(c *Config) {
			c.Rules = append(c.Rules, mapping.Rule{Process: "CODE.EXE", App: "又一个"})
		}, "duplicate process"},
		{"bad regexp", func(c *Config) {
			c.Rules[0].TitlePatterns = []mapping.TitlePattern{{Match: "(["}}
		}, "compile"},
		{"expose_title without a rule", func(c *Config) {
			c.ExposeTitle = append(c.ExposeTitle, "ghost.exe")
		}, "has no rule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			cfg := fullConfig()
			tt.mutate(cfg)

			err := Save(path, cfg)
			if err == nil {
				t.Fatalf("Save accepted an invalid config, want %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantMsg)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Errorf("error is %T, want *ValidationError so a UI can mark the field", err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("an invalid config was written to disk (err = %v)", err)
			}
			assertNoTempFile(t, dir)
		})
	}
}

// The DANGER block is the only warning a user gets before their raw window
// titles go public. It is regenerated on every save and must not go missing.
func TestMarshalKeepsTheExposeTitleWarning(t *testing.T) {
	data, err := Marshal(fullConfig())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{
		"# DANGER: every process listed here reports its RAW WINDOW TITLE as the",
		"# sanitization. Keep it empty unless you really mean it.",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("output is missing %q:\n%s", want, data)
		}
	}
}

// Key order is fixed so two saves of the same configuration produce the same
// file, which is what makes a diff of config.yaml readable.
func TestMarshalKeyOrderIsStable(t *testing.T) {
	data, err := Marshal(fullConfig())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	want := []string{
		"server_url:", "device_id:", "token:", "interval:", "device_name:",
		"idle_threshold:", "default_app:", "default_description:",
		"locked_app:", "locked_description:", "rules:", "expose_title:",
	}
	rest := string(data)
	for _, key := range want {
		i := strings.Index(rest, "\n"+key)
		if i < 0 {
			t.Fatalf("key %q missing or out of order in:\n%s", key, data)
		}
		rest = rest[i+1:]
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{10 * time.Second, "10s"},
		{90 * time.Second, "90s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{time.Minute + 30*time.Second, "90s"},
		{1500 * time.Millisecond, "1.5s"},
		{0, "0s"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.in); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// A failed or successful save must not leave a half-written config.yaml.tmp
// next to the real file — the next reader would have to guess which is which.
func assertNoTempFile(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), tempSuffix) {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}
