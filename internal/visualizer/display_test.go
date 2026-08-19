package visualizer

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// Without source metadata the visualizer must say so rather than print nothing:
// a silent `s` looks like a broken command.
func TestShowSourceWithoutMetadataExplainsItself(t *testing.T) {
	s, buf := newStepper(t)
	s.ShowSource()
	if !strings.Contains(buf.String(), "no source location available") {
		t.Errorf("ShowSource() = %q, want an explanation", buf.String())
	}
}

// With debug info the same command renders the snippet around the definition.
func TestShowSourceRendersSnippet(t *testing.T) {
	var buf bytes.Buffer
	index := source.NewIndex()
	index.LoadDebugInfo(filepath.Join("..", "..", "tests/fixtures/debug-info/token-debug-info.json"))
	trace := load(t, "tests/fixtures/compare/trace-b.json")

	s := New(trace, render.Color{Enabled: false}, index, &buf)
	s.ShowSource()

	out := buf.String()
	if !strings.Contains(out, "FailureDemo.daml") {
		t.Errorf("snippet does not name the file:\n%s", out)
	}
	// The snippet marks the definition line with ">".
	if !strings.Contains(out, ">") {
		t.Errorf("snippet has no current-line marker:\n%s", out)
	}
}

// `vars` is what a user reads to see the event's fields; the keys must appear
// in insertion order, not Go's randomized map order.
func TestShowVariablesListsEventFields(t *testing.T) {
	s, buf := newStepper(t)
	s.ShowVariables()

	out := buf.String()
	if !strings.Contains(out, "variables") {
		t.Fatalf("no header:\n%s", out)
	}
	for _, key := range []string{"eventId", "kind", "template"} {
		if !strings.Contains(out, key+":") {
			t.Errorf("missing %q in:\n%s", key, out)
		}
	}
	if strings.Index(out, "eventId:") > strings.Index(out, "kind:") {
		t.Errorf("keys out of insertion order:\n%s", out)
	}
}

func TestListBreakpoints(t *testing.T) {
	s, buf := newStepper(t)

	s.ListBreakpoints()
	if !strings.Contains(buf.String(), "no breakpoints") {
		t.Errorf("empty list = %q, want a notice", buf.String())
	}

	buf.Reset()
	s.AddBreakpoint("b Transfer")
	s.AddBreakpoint("b 2")
	buf.Reset()
	s.ListBreakpoints()

	out := buf.String()
	if !strings.Contains(out, "1:") || !strings.Contains(out, "Transfer") {
		t.Errorf("listing = %q, want numbered specs", out)
	}
	if !strings.Contains(out, "2:") {
		t.Errorf("second breakpoint missing from %q", out)
	}
}

// findContractPayload walks an ACS snapshot for the exercised contract's
// fields. Nothing writes acsSnapshot today, so this is the only place the
// lookup is exercised -- see the note on inputContractPayload.
func TestFindContractPayload(t *testing.T) {
	acs, err := model.Decode([]byte(`{
	  "response": [
	    {"createdEvent": {"contractId": "cid-other", "createArgument": {"owner": "Alice"}}},
	    {"createdEvent": {"contractId": "cid-1", "createArgument": {"owner": "Bob", "balance": "100"}}}
	  ]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	response, _ := acs.Get("response")

	payload := findContractPayload(response, "cid-1")
	object, ok := payload.(*model.Object)
	if !ok {
		t.Fatalf("payload = %#v, want an object", payload)
	}
	if owner, _ := object.Get("owner"); owner != "Bob" {
		t.Errorf("owner = %v, want Bob", owner)
	}

	if got := findContractPayload(response, "cid-absent"); got != nil {
		t.Errorf("absent contract returned %v, want nil", got)
	}
}

func TestCutLast(t *testing.T) {
	// A file:line spec splits on the LAST colon, so Windows-style paths and
	// module-qualified names do not split in the wrong place.
	before, after, found := cutLast("C:/pkg/Asset.daml:54", ":")
	if !found || before != "C:/pkg/Asset.daml" || after != "54" {
		t.Errorf("cutLast = (%q, %q, %v)", before, after, found)
	}
	if before, after, found := cutLast("noseparator", ":"); found || after != "noseparator" || before != "" {
		t.Errorf("cutLast(no sep) = (%q, %q, %v)", before, after, found)
	}
}

func TestIsDigits(t *testing.T) {
	for _, yes := range []string{"0", "54", "1234567890"} {
		if !isDigits(yes) {
			t.Errorf("isDigits(%q) = false", yes)
		}
	}
	for _, no := range []string{"", "54a", "-1", "5.4", " 5"} {
		if isDigits(no) {
			t.Errorf("isDigits(%q) = true", no)
		}
	}
}

// Every REPL command must do something and none may crash on a trace without
// source metadata -- an unrecognised command is the only one that should say so.
func TestDispatchHandlesEveryCommand(t *testing.T) {
	for _, cmd := range []string{
		"", "n", "next", "p", "prev", "j 1", "j 99", "s", "src", "source",
		"vars", "locals", "b Transfer", "bp", "breakpoints", "clear", "clear 1",
		"c", "continue", "tree", "context", "json", "help",
		"filter", "filter kind create", "filter nothing-matches-this",
		"filter choice", "find Counter", "find nothing-matches-this", "matches",
	} {
		s, buf := newStepper(t)
		if quit := s.Dispatch(cmd); quit {
			t.Errorf("%q reported quit", cmd)
		}
		if buf.Len() == 0 {
			t.Errorf("%q produced no output", cmd)
		}
	}

	s, buf := newStepper(t)
	if quit := s.Dispatch("not-a-command"); quit {
		t.Error("an unknown command reported quit")
	}
	if !strings.Contains(buf.String(), "unknown command") {
		t.Errorf("unknown command = %q", buf.String())
	}

	for _, cmd := range []string{"q", "quit", "exit"} {
		s, _ := newStepper(t)
		if !s.Dispatch(cmd) {
			t.Errorf("%q did not quit", cmd)
		}
	}
}

// Jump takes a 1-based index, and an out-of-range one must be reported rather
// than clamped silently -- a silent clamp looks like the tree is shorter.
func TestJumpReportsOutOfRange(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("j 99")
	if !strings.Contains(buf.String(), "step must be between") {
		t.Errorf("out-of-range jump = %q, want the valid range named", buf.String())
	}

	s, _ = newStepper(t)
	s.Dispatch("j 2")
	if s.Index != 1 {
		t.Errorf("index = %d, want the second step", s.Index)
	}
}

// `json` emits the current event, so it can be piped into other tools.
func TestShowJSONEmitsTheCurrentEvent(t *testing.T) {
	s, buf := newStepper(t)
	s.ShowJSON()
	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("json is not valid: %v\n%s", err, buf.String())
	}
	if event["eventId"] == nil {
		t.Errorf("event has no id: %v", event)
	}
}
