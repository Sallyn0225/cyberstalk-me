package mapping

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cyberstalk.me/shared"
)

// canaryTitle is the stand-in for a raw window title. No test output may ever
// contain it unless the test explicitly opted into expose_title.
const canaryTitle = "SECRET-TITLE-CANARY — 机密项目代号"

// titleStub returns a title getter plus a pointer to its call count, so tests
// can assert the title was never even read.
func titleStub(title string) (func() string, *int) {
	calls := 0
	return func() string {
		calls++
		return title
	}, &calls
}

func testOptions() Options {
	return Options{
		Rules: []Rule{
			{Process: "code.exe", App: "VS Code", Description: "在写代码"},
			{
				Process:     "chrome.exe",
				App:         "Chrome",
				Description: "在上网",
				TitlePatterns: []TitlePattern{
					{Match: "(?i)youtube", Description: "在看视频"},
					{Match: "(?i)github", Description: "在看代码"},
				},
			},
			{Process: "term.exe", App: "终端", Description: "在敲命令"},
		},
		ExposeTitle:        []string{"term.exe"},
		DefaultApp:         "某个应用",
		DefaultDescription: "使用中",
		LockedApp:          "已锁屏",
		LockedDescription:  "人不在",
		IdleThreshold:      5 * time.Minute,
	}
}

func newTestMapper(t *testing.T) *Mapper {
	t.Helper()
	m, err := New(testOptions())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name  string
		proc  string
		title string
		// wantTitleReads is how many times the title getter may be called.
		wantTitleReads int
		wantApp        string
		wantDesc       string
	}{
		{
			name: "rule match", proc: "code.exe", title: canaryTitle,
			wantTitleReads: 0, wantApp: "VS Code", wantDesc: "在写代码",
		},
		{
			name: "process name is case insensitive", proc: "Code.EXE", title: canaryTitle,
			wantTitleReads: 0, wantApp: "VS Code", wantDesc: "在写代码",
		},
		{
			name: "process name is trimmed", proc: "  code.exe  ", title: canaryTitle,
			wantTitleReads: 0, wantApp: "VS Code", wantDesc: "在写代码",
		},
		{
			// The privacy boundary: an unmapped process is generic and the raw
			// title is never even read.
			name: "unknown process falls back to generic and never reads the title",
			proc: "some-internal-tool.exe", title: canaryTitle,
			wantTitleReads: 0, wantApp: "某个应用", wantDesc: "使用中",
		},
		{
			name: "no foreground window is locked", proc: "", title: canaryTitle,
			wantTitleReads: 0, wantApp: "已锁屏", wantDesc: "人不在",
		},
		{
			name: "title pattern first match wins", proc: "chrome.exe", title: "YouTube — github.com",
			wantTitleReads: 1, wantApp: "Chrome", wantDesc: "在看视频",
		},
		{
			name: "title pattern later match", proc: "chrome.exe", title: "GitHub — pull requests",
			wantTitleReads: 1, wantApp: "Chrome", wantDesc: "在看代码",
		},
		{
			name: "no title pattern matches falls back to rule description",
			proc: "chrome.exe", title: canaryTitle,
			wantTitleReads: 1, wantApp: "Chrome", wantDesc: "在上网",
		},
		{
			name: "expose_title reports the raw title verbatim",
			proc: "term.exe", title: "bash — ~/code",
			wantTitleReads: 1, wantApp: "终端", wantDesc: "bash — ~/code",
		},
		{
			name: "expose_title with an empty title keeps the rule description",
			proc: "term.exe", title: "   ",
			wantTitleReads: 1, wantApp: "终端", wantDesc: "在敲命令",
		},
	}

	m := newTestMapper(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, calls := titleStub(tt.title)
			got := m.Resolve(tt.proc, title, 0)
			if got.App != tt.wantApp || got.Description != tt.wantDesc {
				t.Errorf("Resolve(%q) = {%q, %q}, want {%q, %q}", tt.proc, got.App, got.Description, tt.wantApp, tt.wantDesc)
			}
			if *calls != tt.wantTitleReads {
				t.Errorf("title getter called %d times, want %d", *calls, tt.wantTitleReads)
			}
		})
	}
}

// Nothing but an explicit expose_title opt-in may put the raw title into the
// reported activity. This walks every string field of the result so a future
// field addition cannot silently start leaking.
func TestResolveNeverLeaksRawTitle(t *testing.T) {
	m := newTestMapper(t)
	processes := []string{"code.exe", "Chrome.exe", "unknown.exe", "", "explorer.exe"}
	for _, proc := range processes {
		title, _ := titleStub(canaryTitle)
		act := m.Resolve(proc, title, 42)
		v := reflect.ValueOf(act)
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.Kind() != reflect.String {
				continue
			}
			if strings.Contains(f.String(), "SECRET-TITLE-CANARY") {
				t.Errorf("Resolve(%q).%s leaked the raw title: %q", proc, v.Type().Field(i).Name, f.String())
			}
		}
	}
}

func TestResolveIdle(t *testing.T) {
	m := newTestMapper(t) // idle threshold 5m
	tests := []struct {
		name        string
		idleSeconds int
		wantIdle    bool
		wantSeconds int
	}{
		{"active", 0, false, 0},
		{"just below threshold", 299, false, 299},
		{"exactly at threshold counts as idle", 300, true, 300},
		{"above threshold", 3600, true, 3600},
		{"negative seconds are clamped", -5, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, _ := titleStub(canaryTitle)
			got := m.Resolve("code.exe", title, tt.idleSeconds)
			if got.Idle != tt.wantIdle {
				t.Errorf("Idle = %v, want %v", got.Idle, tt.wantIdle)
			}
			if got.IdleSeconds != tt.wantSeconds {
				t.Errorf("IdleSeconds = %d, want %d", got.IdleSeconds, tt.wantSeconds)
			}
			// Idle does not hide what the app is; the site has its own badge.
			if got.App != "VS Code" || got.Description != "在写代码" {
				t.Errorf("idle changed the activity: %+v", got)
			}
		})
	}
}

func TestResolveNilTitleGetter(t *testing.T) {
	m := newTestMapper(t)
	// A collector that could not produce a title must not crash the mapper.
	if got := m.Resolve("chrome.exe", nil, 0); got.Description != "在上网" {
		t.Errorf("Resolve with nil title = %+v, want the rule default", got)
	}
	if got := m.Resolve("term.exe", nil, 0); got.Description != "在敲命令" {
		t.Errorf("expose_title with nil title = %+v, want the rule default", got)
	}
}

func TestResolveReturnsContractType(t *testing.T) {
	m := newTestMapper(t)
	var _ shared.Activity = m.Resolve("code.exe", nil, 0)
}

func TestNewErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantMsg string
	}{
		{
			name:    "empty process",
			mutate:  func(o *Options) { o.Rules = []Rule{{Process: "  ", App: "X"}} },
			wantMsg: "process must not be empty",
		},
		{
			name: "duplicate process differing only in case",
			mutate: func(o *Options) {
				o.Rules = []Rule{{Process: "code.exe", App: "A"}, {Process: "CODE.EXE", App: "B"}}
			},
			wantMsg: "duplicate process",
		},
		{
			name: "bad regexp",
			mutate: func(o *Options) {
				o.Rules = []Rule{{Process: "code.exe", App: "A", TitlePatterns: []TitlePattern{{Match: "(["}}}}
			},
			wantMsg: "compile",
		},
		{
			name:    "expose_title without a rule",
			mutate:  func(o *Options) { o.ExposeTitle = []string{"ghost.exe"} },
			wantMsg: "has no rule",
		},
		{
			name:    "empty expose_title entry",
			mutate:  func(o *Options) { o.ExposeTitle = []string{""} },
			wantMsg: "must not be empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := testOptions()
			tt.mutate(&opts)
			_, err := New(opts)
			if err == nil {
				t.Fatalf("New succeeded, want error containing %q", tt.wantMsg)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %v, want it to contain %q", err, tt.wantMsg)
			}
		})
	}
}
