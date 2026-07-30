package setup

import (
	"regexp"
	"strings"
	"testing"
)

// A suggestion must match the sample it came from, whatever that sample
// contains — regexp metacharacters in window titles are the norm, not the
// exception (brackets, parentheses, plus signs, dots).
func TestSuggestPatternMatchesItsOwnSample(t *testing.T) {
	titles := []string{
		"main.go - project - VS Code",
		"[Debug] app.exe (running) — 100% | v1.2.3+build",
		"C:\\path\\to\\file.txt - Notepad",
		"《文档》- 微信",
		"a|b*c?d",
		"^start$",
		"emoji 🐟 title",
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			pat := SuggestPattern(title)
			re, err := regexp.Compile(pat)
			if err != nil {
				t.Fatalf("SuggestPattern(%q) = %q, which does not compile: %v", title, pat, err)
			}
			if !re.MatchString(title) {
				t.Errorf("pattern %q does not match the sample %q it came from", pat, title)
			}
		})
	}
}

func TestSuggestPatternIsCaseInsensitive(t *testing.T) {
	pat := SuggestPattern("GitHub - Chrome")
	if !strings.HasPrefix(pat, "(?i)") {
		t.Fatalf("pattern = %q, want a (?i) prefix", pat)
	}
	re := regexp.MustCompile(pat)
	if !re.MatchString("github - chrome") {
		t.Errorf("pattern %q did not match a differently-cased title", pat)
	}
}

// The suggestion is intentionally narrow: it must not match a different sample,
// or the UI's "matches 1 of 7" hint would be lying about how specific it is.
func TestSuggestPatternDoesNotOvermatch(t *testing.T) {
	pat := SuggestPattern("inbox - Gmail")
	re := regexp.MustCompile(pat)
	if re.MatchString("drafts - Gmail") {
		t.Errorf("pattern %q matched a different title", pat)
	}
}

func TestTestPattern(t *testing.T) {
	titles := []string{
		"YouTube - 首页",
		"github.com/anthropics - Chrome",
		"bilibili 番剧",
	}
	tests := []struct {
		name        string
		pat         string
		wantValid   bool
		wantMatched []bool
		wantCount   int
	}{
		{
			name:        "case-insensitive alternation",
			pat:         "(?i)youtube|bilibili",
			wantValid:   true,
			wantMatched: []bool{true, false, true},
			wantCount:   2,
		},
		{
			name:        "no match",
			pat:         "(?i)twitter",
			wantValid:   true,
			wantMatched: []bool{false, false, false},
			wantCount:   0,
		},
		{
			name:        "matches everything",
			pat:         "",
			wantValid:   true,
			wantMatched: []bool{true, true, true},
			wantCount:   3,
		},
		{
			name:        "escaped dot is literal",
			pat:         `github\.com`,
			wantValid:   true,
			wantMatched: []bool{false, true, false},
			wantCount:   1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TestPattern(tt.pat, titles)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v (error %q), want %v", got.Valid, got.Error, tt.wantValid)
			}
			if got.MatchCount != tt.wantCount {
				t.Errorf("MatchCount = %d, want %d", got.MatchCount, tt.wantCount)
			}
			for i := range tt.wantMatched {
				if got.Matched[i] != tt.wantMatched[i] {
					t.Errorf("Matched[%d] = %v, want %v (title %q)", i, got.Matched[i], tt.wantMatched[i], titles[i])
				}
			}
		})
	}
}

// An invalid pattern is a normal thing to be holding while typing, not an
// error the caller has to handle: it comes back as "not valid yet, nothing
// matches" so the UI can show the compile message live.
func TestTestPatternReportsCompileFailures(t *testing.T) {
	got := TestPattern("([", []string{"anything"})
	if got.Valid {
		t.Fatal("Valid = true for an unparseable pattern")
	}
	if got.Error == "" {
		t.Error("Error is empty, want the compile failure message")
	}
	if len(got.Matched) != 1 || got.Matched[0] {
		t.Errorf("Matched = %v, want one false entry", got.Matched)
	}
	if got.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", got.MatchCount)
	}
	// The message must be the regexp package's own, so it reads the same as the
	// one the agent prints when it refuses to start on a bad rule.
	if _, err := regexp.Compile("(["); err == nil || got.Error != err.Error() {
		t.Errorf("Error = %q, want the regexp package's message", got.Error)
	}
}

func TestTestPatternWithNoTitles(t *testing.T) {
	got := TestPattern("(?i)anything", nil)
	if !got.Valid || got.MatchCount != 0 || len(got.Matched) != 0 {
		t.Errorf("TestPattern with no titles = %+v, want a valid empty result", got)
	}
}
