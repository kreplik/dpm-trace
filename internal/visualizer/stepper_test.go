package visualizer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

func load(t *testing.T, rel string) *model.Trace {
	t.Helper()
	artifact, err := model.LoadTraceArtifact(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := model.TraceFromArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

func newStepper(t *testing.T) (*Stepper, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	return New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf), &buf
}

// Children follow their parent, so the order matches the tree the user sees.
func TestPreorderFollowsTheTree(t *testing.T) {
	s, _ := newStepper(t)
	if len(s.Order) != 2 {
		t.Fatalf("order = %v, want 2 steps", s.Order)
	}
	parent := s.Trace.EventsByID[s.Order[0]]
	if len(parent.ChildEventIDs) == 0 || parent.ChildEventIDs[0] != s.Order[1] {
		t.Errorf("order = %v; the child should immediately follow its parent", s.Order)
	}
}

// An event unreachable from any root must still be visitable, or it would be
// invisible in the visualizer.
func TestPreorderIncludesOrphans(t *testing.T) {
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	trace.EventsByID["orphan"] = &model.Event{EventID: "orphan", Kind: model.KindCreate}
	s := New(trace, render.Color{Enabled: false}, source.NewIndex(), &bytes.Buffer{})

	found := false
	for _, id := range s.Order {
		if id == "orphan" {
			found = true
		}
	}
	if !found {
		t.Errorf("orphan missing from order %v", s.Order)
	}
}

func TestNavigationClampsAtBothEnds(t *testing.T) {
	s, _ := newStepper(t)
	s.Dispatch("p")
	if s.Index != 0 {
		t.Errorf("prev at the first step moved to %d", s.Index)
	}
	for i := 0; i < 5; i++ {
		s.Dispatch("n")
	}
	if s.Index != len(s.Order)-1 {
		t.Errorf("next past the end moved to %d, want %d", s.Index, len(s.Order)-1)
	}
}

func TestJumpRejectsOutOfRange(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("j 99")
	if s.Index != 0 {
		t.Errorf("index moved to %d on an out-of-range jump", s.Index)
	}
	if !strings.Contains(buf.String(), "step must be between") {
		t.Errorf("no range message:\n%s", buf.String())
	}
}

func TestQuitCommands(t *testing.T) {
	for _, cmd := range []string{"q", "quit", "exit"} {
		s, _ := newStepper(t)
		if !s.Dispatch(cmd) {
			t.Errorf("%q should quit", cmd)
		}
	}
	s, _ := newStepper(t)
	if s.Dispatch("tree") {
		t.Error("tree should not quit")
	}
}

func TestBreakpointMatching(t *testing.T) {
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	ev := trace.EventsByID[trace.RootEventIDs[0]]

	for _, tc := range []struct {
		spec string
		want bool
	}{
		{ev.EventID, true},
		{"#" + ev.EventID, true},
		{"1", true},           // step number, 1-based
		{"Transfer", true},    // choice
		{"Token:Token", true}, // template
		{"2", false},
		{"Nope", false},
		{"", false},
	} {
		if got := (Breakpoint{Spec: tc.spec}).Matches(0, ev.EventID, ev, nil); got != tc.want {
			t.Errorf("Matches(%q) = %v, want %v", tc.spec, got, tc.want)
		}
	}
}

// A source-based spec must not match when no source is loaded, rather than
// matching everything or panicking.
func TestBreakpointWithoutSource(t *testing.T) {
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	ev := trace.EventsByID[trace.RootEventIDs[0]]
	if (Breakpoint{Spec: "Asset.daml:42"}).Matches(0, ev.EventID, ev, nil) {
		t.Error("a file:line spec must not match without source")
	}
}

// Breakpoints match on step number, event id, template or choice -- not on
// event kind, so "b create" matches nothing.
func TestContinueStopsAtBreakpoint(t *testing.T) {
	s, _ := newStepper(t)
	s.Dispatch("b 2")
	s.Dispatch("c")
	if s.Index != 1 {
		t.Errorf("continue stopped at %d, want step 2 at index 1", s.Index)
	}
}

func TestBreakpointDoesNotMatchEventKind(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("b create")
	s.Dispatch("c")
	if s.Index != 0 {
		t.Errorf("index moved to %d; a kind is not a breakpoint spec", s.Index)
	}
	if !strings.Contains(buf.String(), "no later breakpoint hit") {
		t.Errorf("no message:\n%s", buf.String())
	}
}

func TestContinueWithoutBreakpointsStaysPut(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("c")
	if s.Index != 0 {
		t.Errorf("index moved to %d with no breakpoints set", s.Index)
	}
	if !strings.Contains(buf.String(), "no breakpoints set") {
		t.Errorf("no message:\n%s", buf.String())
	}
}

func TestClearBreakpoints(t *testing.T) {
	s, _ := newStepper(t)
	s.Dispatch("b a")
	s.Dispatch("b b")
	s.Dispatch("clear 1")
	if len(s.Breakpoints) != 1 || s.Breakpoints[0].Spec != "b" {
		t.Errorf("breakpoints = %v after clearing the first", s.Breakpoints)
	}
	s.Dispatch("clear")
	if len(s.Breakpoints) != 0 {
		t.Errorf("breakpoints = %v after clearing all", s.Breakpoints)
	}
}

// Variables are bound by presence, not by event kind, and parties are aliased.
func TestStepVariablesBindsPresentFields(t *testing.T) {
	s, _ := newStepper(t)
	ctx := render.NewContext(s.Trace)
	vars, order := s.StepVariables(s.Current(), ctx)

	for _, key := range []string{"eventId", "kind", "template", "contractId", "choice", "actors", "witnesses"} {
		if _, ok := vars[key]; !ok {
			t.Errorf("%q not bound; got %v", key, order)
		}
	}
	if order[0] != "eventId" || order[1] != "kind" {
		t.Errorf("order starts %v; the display depends on insertion order", order[:2])
	}
	actors, ok := vars["actors"].([]any)
	if !ok || len(actors) == 0 {
		t.Fatalf("actors = %#v", vars["actors"])
	}
	if actors[0] != "Alice" {
		t.Errorf("actors[0] = %v, want the alias", actors[0])
	}
}

func TestRunExitsOnEndOfInput(t *testing.T) {
	s, buf := newStepper(t)
	s.Run(strings.NewReader("tree\n"))
	if !strings.Contains(buf.String(), "Visualizer commands:") {
		t.Errorf("no banner:\n%s", buf.String())
	}
}

func TestShowJSONMatchesTheArtifactEncoding(t *testing.T) {
	s, buf := newStepper(t)
	s.Dispatch("json")
	if !strings.Contains(buf.String(), `"eventId"`) || !strings.Contains(buf.String(), `"kind"`) {
		t.Errorf("json output looks wrong:\n%s", buf.String())
	}
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }

// The projection is stated in the header, but the header scrolls away. A
// reader ten steps in is looking at payloads with no reminder that this is one
// participant's view, so the prompt names the parties for the whole session.
func TestPromptNamesTheProjectionParties(t *testing.T) {
	s, _ := newStepper(t)
	got := s.prompt()
	if !strings.Contains(got, "Issuer") && !strings.Contains(got, "Alice") {
		t.Errorf("prompt %q names no read-as party", got)
	}

	// An active filter is the other thing that changes what is on screen, so
	// it belongs in the same place.
	s.Dispatch("filter kind exercise")
	if got := s.prompt(); !strings.Contains(got, "filter:") {
		t.Errorf("prompt %q does not show the active filter", got)
	}
	s.Dispatch("filter")
	if got := s.prompt(); strings.Contains(got, "filter:") {
		t.Errorf("prompt %q still shows a cleared filter", got)
	}
}

// A trace with no read-as must still produce a usable prompt.
func TestPromptWithoutPartiesIsPlain(t *testing.T) {
	var buf bytes.Buffer
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	trace.Projection.ReadAs = nil
	s := New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf)
	if got := s.prompt(); got != "dpm-trace> " {
		t.Errorf("prompt = %q, want the plain form", got)
	}
}

// The visualizer must not print a header the caller already printed.
func TestVisualizerHeaderAppearsOnce(t *testing.T) {
	s, buf := newStepper(t)
	s.Run(strings.NewReader("q\n"))
	if got := strings.Count(buf.String(), "update:"); got != 1 {
		t.Errorf("header printed %d times, want 1:\n%s", got, buf.String())
	}
}
