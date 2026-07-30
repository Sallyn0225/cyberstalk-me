package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"

	"cyberstalk.me/client-windows/internal/mapping"
)

const (
	backupSuffix = ".bak"
	tempSuffix   = ".tmp"
)

// BackupPath is where Save moves the previous configuration before replacing
// it. A user who dislikes what a configuration UI wrote renames this back.
func BackupPath(path string) string { return path + backupSuffix }

// Save validates cfg and writes it to path as commented YAML.
//
// Validation runs first and uses the same rules as Load, so a file Save
// produced is by construction a file the agent will start with.
//
// The write is atomic and preserves the existing file's ACL: config.yaml holds
// a device token and the README tells users to tighten its permissions, so a
// save must never silently loosen them. The previous file is left at
// BackupPath(path).
func Save(path string, cfg *Config) error {
	if err := cfg.Validate(path); err != nil {
		return err
	}
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}

	// The temp file sits next to the target so the replacement stays within one
	// volume, and is written 0600 because it already contains the token.
	tmp := path + tempSuffix
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}

	if _, err := os.Stat(path); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			os.Remove(tmp)
			return fmt.Errorf("stat config %s: %w", path, err)
		}
		// First save: no ACL to preserve and nothing to back up.
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("create config %s: %w", path, err)
		}
		return nil
	}

	if err := replaceFile(path, tmp, BackupPath(path)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace config %s: %w", path, err)
	}
	return nil
}

// Marshal renders cfg as the commented YAML that Save writes.
//
// The comments are regenerated from the text below on every save, which is why
// Save keeps a backup: hand-written comments and key ordering do not survive a
// round trip through a configuration UI.
func Marshal(cfg *Config) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	put := func(key, comment string, value *yaml.Node) {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key, HeadComment: comment},
			value)
	}

	put("server_url", headerComment, scalar(cfg.ServerURL))
	put("device_id", "", scalar(cfg.DeviceID))
	put("token", "", scalar(cfg.Token))
	put("interval", "", scalar(FormatDuration(cfg.Interval)))
	if cfg.DeviceName != "" {
		put("device_name", deviceNameComment, scalar(cfg.DeviceName))
	}
	put("idle_threshold", idleThresholdComment, scalar(FormatDuration(cfg.IdleThreshold)))
	put("default_app", defaultsComment, scalar(cfg.DefaultApp))
	put("default_description", "", scalar(cfg.DefaultDescription))
	put("locked_app", lockedComment, scalar(cfg.LockedApp))
	put("locked_description", "", scalar(cfg.LockedDescription))
	put("rules", rulesComment, rulesNode(cfg.Rules))
	put("expose_title", exposeTitleComment, stringsNode(cfg.ExposeTitle))

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}
	return buf.Bytes(), nil
}

// scalar builds a string node. The !!str tag is what makes the encoder quote a
// value that would otherwise read back as another type — a token of "12345"
// must not come back as an int.
func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func entry(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content, scalar(key), value)
}

func rulesNode(rules []mapping.Rule) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	if len(rules) == 0 {
		// Flow style so an empty list reads as "rules: []" rather than a bare
		// key with nothing under it.
		n.Style = yaml.FlowStyle
		return n
	}
	for _, r := range rules {
		item := &yaml.Node{Kind: yaml.MappingNode}
		entry(item, "process", scalar(r.Process))
		entry(item, "app", scalar(r.App))
		// An omitted description falls back to default_description on load, so
		// writing an empty one would be noise.
		if r.Description != "" {
			entry(item, "description", scalar(r.Description))
		}
		if len(r.TitlePatterns) > 0 {
			patterns := &yaml.Node{Kind: yaml.SequenceNode}
			for _, p := range r.TitlePatterns {
				pattern := &yaml.Node{Kind: yaml.MappingNode}
				entry(pattern, "match", scalar(p.Match))
				if p.Description != "" {
					entry(pattern, "description", scalar(p.Description))
				}
				patterns.Content = append(patterns.Content, pattern)
			}
			entry(item, "title_patterns", patterns)
		}
		n.Content = append(n.Content, item)
	}
	return n
}

func stringsNode(values []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode}
	if len(values) == 0 {
		n.Style = yaml.FlowStyle
		return n
	}
	for _, v := range values {
		n.Content = append(n.Content, scalar(v))
	}
	return n
}

// FormatDuration prefers the shortest exact unit ("5m" over Go's "5m0s"), which
// is what a hand-written config looks like and what the docs show. It is the
// inverse of ParseDuration and is exported for the same reason: a UI shows the
// same spelling the file uses.
func FormatDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return d.String()
	case d%time.Hour == 0:
		return strconv.FormatInt(int64(d/time.Hour), 10) + "h"
	case d%time.Minute == 0:
		return strconv.FormatInt(int64(d/time.Minute), 10) + "m"
	case d%time.Second == 0:
		return strconv.FormatInt(int64(d/time.Second), 10) + "s"
	default:
		return d.String()
	}
}

// The comment blocks below are kept in sync with config.example.yaml: someone
// who read the example and someone who used the setup UI should end up with the
// same explanations in front of them.
const (
	headerComment = `cyberstalk-me Windows agent config.

Written by ` + "`agent.exe -setup`" + `. Comments and key order are regenerated on
every save; the file this replaced is kept as config.yaml.bak.

The first four keys are printed verbatim by:
  server register-device <id> <name> windows

config.yaml contains a device token — treat it as a secret and tighten its
ACL (see README.md).`

	deviceNameComment = `
Optional. The server re-stamps the name it was registered with, so this only
affects local log readability. Rename a device with register-device, not here.`

	idleThresholdComment = `
No keyboard/mouse input for this long counts as idle.`

	defaultsComment = `
Fallbacks for a process with no rule below. Deliberately generic: the exe
name is never used as an app name, because the name itself can leak
(internal tools, project codenames). Want it shown? Write a rule.`

	lockedComment = `
Shown when there is no foreground window (lock screen, session switch).`

	rulesComment = `
process is the exe base name, matched case-insensitively.
title_patterns is optional: the raw window title is read only for rules that
have patterns, is matched in memory, and is never reported — only the
matching pattern's description is.`

	exposeTitleComment = `
DANGER: every process listed here reports its RAW WINDOW TITLE as the
description, verbatim and publicly. That is the one and only opt-out of
sanitization. Keep it empty unless you really mean it.
A process listed here must also have a rule above.`
)
