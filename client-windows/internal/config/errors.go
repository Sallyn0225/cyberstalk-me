package config

import (
	"fmt"
	"strings"
)

// ValidationError is what Validate returns when a configuration is not usable.
//
// Its rendered message is byte-identical to what Load returned before this type
// existed; the added value is Fields, the YAML paths of the keys at fault, so a
// configuration UI can mark them without parsing the message back apart.
type ValidationError struct {
	// Source describes where the configuration came from — a file path for
	// Load, "draft" for an in-memory configuration being edited.
	Source string
	// Fields are the YAML paths at fault, e.g. ["server_url"] or
	// ["rules[1].title_patterns[0].match"]. It may hold several entries (a set
	// of required keys are all missing) or none (the problem is not
	// field-scoped).
	Fields []string
	// Message states the problem without the "config <source>: " prefix.
	Message string
	// Err is the underlying cause, if any.
	Err error
}

func (e *ValidationError) Error() string {
	if e.Source == "" {
		return e.Message
	}
	return fmt.Sprintf("config %s: %s", e.Source, e.Message)
}

func (e *ValidationError) Unwrap() error { return e.Err }

// invalid builds a ValidationError for source with a formatted message.
func invalid(source string, fields []string, format string, args ...any) *ValidationError {
	return &ValidationError{Source: source, Fields: fields, Message: fmt.Sprintf(format, args...)}
}

// ruleField is the YAML path of a key inside rules[i].
func ruleField(i int, key string) string { return fmt.Sprintf("rules[%d].%s", i, key) }

// joinFields renders a field list into a message. Kept next to the type so the
// separator cannot drift from what the messages already use.
func joinFields(paths []string) string { return strings.Join(paths, ", ") }
