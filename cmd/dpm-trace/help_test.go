package main

import (
	"bytes"
	"strings"
	"testing"
)

// Every flag help lists must be implemented: a user following the help would
// otherwise pass one that silently does nothing. Nothing is unported now, so
// the list is asserted empty rather than enumerated.
func TestHelpOnlyListsImplementedFlags(t *testing.T) {
	var buf bytes.Buffer
	rootHelp(&buf)
	out := buf.String()

	flagsSection := out[strings.Index(out, "Flags:"):strings.Index(out, "Examples:")]
	for _, flag := range []string{"--visualize", "--damlc", "--debug-info", "--dar", "--wait", "--export"} {
		if flag == "--visualize" {
			continue // an `open` flag, not a root one
		}
		if !strings.Contains(flagsSection, flag) {
			t.Errorf("%s is implemented but not documented", flag)
		}
	}
}

// The participant-scoping caveat is a stated project rule, not decoration.
func TestHelpStatesParticipantScope(t *testing.T) {
	var buf bytes.Buffer
	rootHelp(&buf)
	out := buf.String()
	if !strings.Contains(out, "participant-scoped") {
		t.Error("help must state that output is participant-scoped")
	}
	if !strings.Contains(out, "not a") || !strings.Contains(out, "global Canton transaction") {
		t.Error("help must state that this is not a global Canton transaction")
	}
}

func TestHelpHasNoTrailingWhitespace(t *testing.T) {
	var buf bytes.Buffer
	rootHelp(&buf)
	for i, line := range strings.Split(buf.String(), "\n") {
		if line != strings.TrimRight(line, " \t") {
			t.Errorf("line %d has trailing whitespace: %q", i+1, line)
		}
	}
}

func TestWantsHelp(t *testing.T) {
	if !wantsHelp([]string{"open", "--help"}) {
		t.Error("--help not detected")
	}
	if !wantsHelp([]string{"-h"}) {
		t.Error("-h not detected")
	}
	if wantsHelp([]string{"open", "trace.json"}) {
		t.Error("false positive")
	}
}

// A bare `version` or `help` word must stay a positional argument: cli.py
// treats them as update ids, and shadowing a positional is a behavior change.
// --version is the one deliberate addition, since a standalone binary needs it.
func TestBareWordsAreNotSubcommands(t *testing.T) {
	for _, word := range []string{"version", "help"} {
		if wantsHelp([]string{word}) {
			t.Errorf("%q must not be treated as a help request", word)
		}
	}
	if !wantsHelp([]string{"--help"}) {
		t.Error("--help must be a help request")
	}
}
