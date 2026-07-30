package mapping

import (
	"errors"
	"regexp"
	"regexp/syntax"
	"testing"
)

// New's messages are user-facing startup errors. They are asserted verbatim
// here so that giving them a structured type cannot quietly reword them.
func TestNewErrorMessagesAndFieldPaths(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Options)
		wantMsg   string
		wantField string
	}{
		{
			name:      "empty process",
			mutate:    func(o *Options) { o.Rules = []Rule{{Process: "  ", App: "X"}} },
			wantMsg:   "rule 0: process must not be empty",
			wantField: "rules[0].process",
		},
		{
			name: "duplicate process differing only in case",
			mutate: func(o *Options) {
				o.Rules = []Rule{{Process: "code.exe", App: "A"}, {Process: "CODE.EXE", App: "B"}}
			},
			wantMsg:   `rule 1: duplicate process "CODE.EXE"`,
			wantField: "rules[1].process",
		},
		{
			name: "bad regexp in the second pattern of the second rule",
			mutate: func(o *Options) {
				o.ExposeTitle = nil
				o.Rules = []Rule{
					{Process: "code.exe", App: "A"},
					{Process: "chrome.exe", App: "B", TitlePatterns: []TitlePattern{
						{Match: "ok"},
						{Match: "(["},
					}},
				}
			},
			wantMsg:   `rule 1 (chrome.exe) title_pattern 1: compile "([": ` + regexpCompileError(t, "(["),
			wantField: "rules[1].title_patterns[1].match",
		},
		{
			name:      "expose_title without a rule",
			mutate:    func(o *Options) { o.ExposeTitle = []string{"term.exe", "ghost.exe"} },
			wantMsg:   `expose_title 1: process "ghost.exe" has no rule; add a rule for it first`,
			wantField: "expose_title[1]",
		},
		{
			name:      "empty expose_title entry",
			mutate:    func(o *Options) { o.ExposeTitle = []string{""} },
			wantMsg:   "expose_title 0: process must not be empty",
			wantField: "expose_title[0]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := testOptions()
			tt.mutate(&opts)
			_, err := New(opts)
			if err == nil {
				t.Fatalf("New succeeded, want error %q", tt.wantMsg)
			}
			if got := err.Error(); got != tt.wantMsg {
				t.Errorf("error = %q, want %q", got, tt.wantMsg)
			}
			var re *RuleError
			if !errors.As(err, &re) {
				t.Fatalf("error is %T, want it to unwrap to *RuleError", err)
			}
			if got := re.FieldPath(); got != tt.wantField {
				t.Errorf("FieldPath() = %q, want %q", got, tt.wantField)
			}
		})
	}
}

// A compile failure must stay inspectable through the wrapped regexp error, not
// only through its rendered message.
func TestRuleErrorUnwrapsCompileFailure(t *testing.T) {
	_, err := New(Options{Rules: []Rule{
		{Process: "code.exe", App: "A", TitlePatterns: []TitlePattern{{Match: "(["}}},
	}})
	if err == nil {
		t.Fatal("New succeeded on a bad regexp, want error")
	}
	var se *syntax.Error
	if !errors.As(err, &se) {
		t.Errorf("error %v does not unwrap to *syntax.Error", err)
	}
}

// regexpCompileError returns the exact message regexp.Compile produces for pat,
// so the assertions above do not hardcode a Go-version-specific string.
func regexpCompileError(t *testing.T, pat string) string {
	t.Helper()
	_, err := regexp.Compile(pat)
	if err == nil {
		t.Fatalf("regexp.Compile(%q) unexpectedly succeeded", pat)
	}
	return err.Error()
}
