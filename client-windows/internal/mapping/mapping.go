// Package mapping turns a raw process name into the sanitized {app,
// description} pair that is safe to report.
//
// This package is the privacy boundary of the agent. Two rules make it work:
//
//  1. The raw window title is never passed in as a string. Callers hand over a
//     lazy title func that is only invoked when a rule actually needs the
//     title, so a deployment without title rules never even reads one.
//  2. A process that matches no rule falls back to the generic default. The
//     executable name is never derived into an app name — an exe name (an
//     internal tool, a project codename) is itself a leak.
//
// The package is pure (no I/O, no Win32) and must stay unit-tested; see
// .trellis/spec/backend/quality-guidelines.md.
package mapping

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"cyberstalk.me/shared"
)

// Rule maps one process to its sanitized app name and description. The yaml
// tags let config.Load decode straight into this type without a parallel set
// of structs; struct tags add no dependency on the yaml package.
type Rule struct {
	// Process is the executable base name, matched case-insensitively
	// (e.g. "code.exe").
	Process string `yaml:"process"`
	// App is the sanitized app name shown on the site.
	App string `yaml:"app"`
	// Description is the sanitized default activity description.
	Description string `yaml:"description"`
	// TitlePatterns optionally refine Description by matching the raw window
	// title. The title is matched in memory only; it is never reported.
	TitlePatterns []TitlePattern `yaml:"title_patterns"`
}

// TitlePattern refines a rule's description when the raw window title matches
// Match (a Go regular expression).
type TitlePattern struct {
	Match       string `yaml:"match"`
	Description string `yaml:"description"`
}

// Options is the mapper configuration. Defaults are the caller's job (see
// package config); New only validates.
type Options struct {
	Rules []Rule
	// ExposeTitle lists processes whose raw window title is reported verbatim.
	// This is the one explicit opt-out of sanitization and defaults to empty.
	ExposeTitle []string
	// DefaultApp / DefaultDescription are used for any process without a rule.
	DefaultApp         string
	DefaultDescription string
	// LockedApp / LockedDescription are used when there is no foreground
	// window (lock screen, session switch).
	LockedApp         string
	LockedDescription string
	// IdleThreshold is how long without input counts as idle.
	IdleThreshold time.Duration
}

// Mapper resolves process names to sanitized activities.
type Mapper struct {
	rules              map[string]compiledRule
	expose             map[string]struct{}
	defaultApp         string
	defaultDescription string
	lockedApp          string
	lockedDescription  string
	idleThreshold      time.Duration
}

type compiledRule struct {
	app         string
	description string
	patterns    []compiledPattern
}

type compiledPattern struct {
	re          *regexp.Regexp
	description string
}

// New compiles the rules into a Mapper. A bad regular expression is a
// configuration error and fails here, at startup, rather than silently inside
// the report loop.
func New(opts Options) (*Mapper, error) {
	m := &Mapper{
		rules:              make(map[string]compiledRule, len(opts.Rules)),
		expose:             make(map[string]struct{}, len(opts.ExposeTitle)),
		defaultApp:         opts.DefaultApp,
		defaultDescription: opts.DefaultDescription,
		lockedApp:          opts.LockedApp,
		lockedDescription:  opts.LockedDescription,
		idleThreshold:      opts.IdleThreshold,
	}
	for i, r := range opts.Rules {
		key := normalize(r.Process)
		if key == "" {
			return nil, fmt.Errorf("rule %d: process must not be empty", i)
		}
		if _, dup := m.rules[key]; dup {
			return nil, fmt.Errorf("rule %d: duplicate process %q", i, r.Process)
		}
		cr := compiledRule{app: r.App, description: r.Description}
		for j, p := range r.TitlePatterns {
			re, err := regexp.Compile(p.Match)
			if err != nil {
				return nil, fmt.Errorf("rule %d (%s) title_pattern %d: compile %q: %w", i, r.Process, j, p.Match, err)
			}
			cr.patterns = append(cr.patterns, compiledPattern{re: re, description: p.Description})
		}
		m.rules[key] = cr
	}
	for i, p := range opts.ExposeTitle {
		key := normalize(p)
		if key == "" {
			return nil, fmt.Errorf("expose_title %d: process must not be empty", i)
		}
		if _, ok := m.rules[key]; !ok {
			// Without a matching rule the process would fall back to the
			// generic default and the title would never be read, which is not
			// what someone writing expose_title expects. Fail loud.
			return nil, fmt.Errorf("expose_title %d: process %q has no rule; add a rule for it first", i, p)
		}
		m.expose[key] = struct{}{}
	}
	return m, nil
}

// Resolve maps a foreground process to a sanitized activity.
//
// process is the executable base name, or "" when there is no foreground
// window. title is a lazy getter for the raw window title: it is called only
// when a rule needs the title, and its result never leaves this function
// except through an explicit expose_title opt-in.
func (m *Mapper) Resolve(process string, title func() string, idleSeconds int) shared.Activity {
	if idleSeconds < 0 {
		idleSeconds = 0
	}
	act := shared.Activity{
		Idle:        time.Duration(idleSeconds)*time.Second >= m.idleThreshold,
		IdleSeconds: idleSeconds,
	}

	key := normalize(process)
	if key == "" {
		// No foreground window: locked screen or a session switch. The flag is
		// what the server keys on — lockedApp is a user-configured string it
		// cannot recognize.
		act.App = m.lockedApp
		act.Description = m.lockedDescription
		act.Locked = true
		return act
	}

	rule, ok := m.rules[key]
	if !ok {
		// Unknown process: generic description. The title func is NOT called,
		// so the raw title never enters the process image at all.
		act.App = m.defaultApp
		act.Description = m.defaultDescription
		return act
	}

	act.App = rule.app
	act.Description = rule.description

	if _, exposed := m.expose[key]; exposed {
		// The one explicit opt-out: the user asked for the raw title.
		if raw := strings.TrimSpace(readTitle(title)); raw != "" {
			act.Description = raw
		}
		return act
	}

	if len(rule.patterns) > 0 {
		raw := readTitle(title)
		for _, p := range rule.patterns {
			if p.re.MatchString(raw) {
				act.Description = p.description
				break
			}
		}
	}
	return act
}

// readTitle calls the lazy title getter, tolerating a nil func.
func readTitle(title func() string) string {
	if title == nil {
		return ""
	}
	return title()
}

// normalize lowercases and trims a process name so matching is
// case-insensitive ("Code.exe" and "code.EXE" are the same rule).
func normalize(process string) string {
	return strings.ToLower(strings.TrimSpace(process))
}
