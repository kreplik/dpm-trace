package main

import (
	"bytes"
	"strings"
	"testing"
)

// Help must not advertise flags this binary ignores: a user following the help
// would otherwise pass a flag that silently does nothing.
func TestHelpOnlyListsImplementedFlags(t *testing.T) {
	var buf bytes.Buffer
	rootHelp(&buf)
	out := buf.String()

	flagsSection := out[strings.Index(out, "Flags:"):strings.Index(out, "Examples:")]
	for _, unported := range []string{"--visualize", "--damlc", "--debug-info", "--explain-apis", "--log-file", "--source-root"} {
		if strings.Contains(flagsSection, unported) {
			t.Errorf("%s is listed as a flag but is not implemented", unported)
		}
		if !strings.Contains(out, unported) {
			t.Errorf("%s should still be named in the not-ported note", unported)
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
