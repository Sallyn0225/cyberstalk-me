package mapping

import "fmt"

// RuleError locates a rule-level configuration problem.
//
// New returns it for every rules[] and expose_title[] failure. The rendered
// message is byte-identical to what New returned before this type existed —
// the point of the type is the machine-readable location (FieldPath), so a
// caller such as a configuration UI can highlight the offending YAML key
// instead of parsing the message back apart.
type RuleError struct {
	// Index is the position in Options.Rules, or in Options.ExposeTitle when
	// ExposeTitle is true.
	Index int
	// Pattern is the position in the rule's TitlePatterns, or -1 when the
	// problem is not scoped to a single pattern.
	Pattern int
	// ExposeTitle reports whether Index refers to Options.ExposeTitle rather
	// than Options.Rules.
	ExposeTitle bool
	// Process is the process name as the user wrote it. It appears in the
	// message of pattern-scoped errors to make them readable on their own.
	Process string
	// Field is the leaf YAML key at fault ("process", "match"), or "" when the
	// whole entry is at fault.
	Field string
	// Message states the problem without any location prefix.
	Message string
	// Err is the underlying cause, if any (a regexp compile failure).
	Err error
}

func (e *RuleError) Error() string {
	switch {
	case e.ExposeTitle:
		return fmt.Sprintf("expose_title %d: %s", e.Index, e.Message)
	case e.Pattern >= 0:
		return fmt.Sprintf("rule %d (%s) title_pattern %d: %s", e.Index, e.Process, e.Pattern, e.Message)
	default:
		return fmt.Sprintf("rule %d: %s", e.Index, e.Message)
	}
}

func (e *RuleError) Unwrap() error { return e.Err }

// FieldPath is the YAML path of the offending key, e.g.
// "rules[2].title_patterns[0].match". The indices are positions in the
// configuration as written, which is what a form needs to mark a field.
func (e *RuleError) FieldPath() string {
	switch {
	case e.ExposeTitle:
		return fmt.Sprintf("expose_title[%d]", e.Index)
	case e.Pattern >= 0:
		return fmt.Sprintf("rules[%d].title_patterns[%d].%s", e.Index, e.Pattern, e.Field)
	case e.Field == "":
		return fmt.Sprintf("rules[%d]", e.Index)
	default:
		return fmt.Sprintf("rules[%d].%s", e.Index, e.Field)
	}
}
