package setup

import "regexp"

// SuggestPattern turns a window title into a starting title_pattern.
//
// Everything is escaped and the match is made case-insensitive, so the
// suggestion matches exactly the sample it came from and nothing else. That is
// deliberately too narrow to be useful as-is: the UI shows how many samples the
// pattern currently matches, and the user widens it by deleting the parts that
// vary. Guessing which part of a title is the "interesting" one would be wrong
// often enough to be worse than starting from something predictable.
func SuggestPattern(title string) string {
	return "(?i)" + regexp.QuoteMeta(title)
}

// PatternTest is the result of trying one title_pattern against a set of
// samples: what the UI needs to say "matches 3 of 7" and highlight which ones.
type PatternTest struct {
	// Valid reports whether the pattern compiled.
	Valid bool `json:"valid"`
	// Error is the compile failure message, empty when Valid.
	Error string `json:"error,omitempty"`
	// Matched is parallel to the titles that were tested.
	Matched []bool `json:"matched"`
	// MatchCount is how many entries in Matched are true.
	MatchCount int `json:"match_count"`
}

// TestPattern compiles pat and reports which titles it matches.
//
// It uses the same regexp package, and therefore the same RE2 syntax, that
// mapping.New validates rules with — a pattern accepted here is a pattern the
// agent will accept at startup.
func TestPattern(pat string, titles []string) PatternTest {
	result := PatternTest{Matched: make([]bool, len(titles))}
	re, err := regexp.Compile(pat)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Valid = true
	for i, t := range titles {
		if re.MatchString(t) {
			result.Matched[i] = true
			result.MatchCount++
		}
	}
	return result
}
