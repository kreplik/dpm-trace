package visualizer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// nestedStepper loads the three-level settlement fixture. trace-b is one level
// deep, which cannot tell a transitive descendant count from a child count.
func nestedStepper(t *testing.T) (*Stepper, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	trace := load(t, "tests/fixtures/nested-tree.trace.json")
	return New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf), &buf
}

func TestTreeShowsEveryEventWhenNothingIsCollapsed(t *testing.T) {
	s, buf := nestedStepper(t)
	s.ShowTree("")

	out := buf.String()
	for _, id := range []string{"#5:0", "#5:1", "#5:2", "#5:3", "#5:4", "#5:5", "#5:6"} {
		if !strings.Contains(out, id) {
			t.Errorf("%s missing from the full tree:\n%s", id, out)
		}
	}
}

// Collapsing hides the whole subtree, not just the direct children -- the
// count has to mean "lines you cannot see".
func TestCollapseHidesDescendantsTransitively(t *testing.T) {
	s, buf := nestedStepper(t)
	s.Collapse("#5:1")

	out := buf.String()
	// #5:1 has one child (#5:3) with a child of its own, plus #5:2: three in all.
	if !strings.Contains(out, "3 events hidden") {
		t.Errorf("want a transitive count of 3:\n%s", out)
	}
	for _, hidden := range []string{"#5:2", "#5:3", "#5:4"} {
		if strings.Contains(out, hidden) {
			t.Errorf("%s should be hidden:\n%s", hidden, out)
		}
	}
	// Siblings stay visible: collapse is not "show only this".
	if !strings.Contains(out, "#5:5") {
		t.Errorf("sibling #5:5 disappeared:\n%s", out)
	}
}

func TestExpandRestoresTheSubtree(t *testing.T) {
	s, buf := nestedStepper(t)
	s.Collapse("#5:1")
	buf.Reset()
	s.Expand("#5:1")

	if !strings.Contains(buf.String(), "#5:4") {
		t.Errorf("expand did not restore the subtree:\n%s", buf.String())
	}
}

// Collapse is keyed by event, so it survives navigation: a reader who collapses
// a noisy subtree, steps away and comes back expects it still collapsed.
func TestCollapseSurvivesNavigation(t *testing.T) {
	s, buf := nestedStepper(t)
	s.Collapse("#5:1")
	s.Dispatch("n")
	s.Dispatch("p")

	buf.Reset()
	s.ShowTree("")
	if !strings.Contains(buf.String(), "hidden") {
		t.Errorf("collapse was lost across navigation:\n%s", buf.String())
	}
}

func TestCollapseAllAndExpandAll(t *testing.T) {
	s, buf := nestedStepper(t)

	s.Collapse("all")
	out := buf.String()
	// Only the root survives, hiding the other six.
	if !strings.Contains(out, "6 events hidden") {
		t.Errorf("collapse all = \n%s", out)
	}
	if strings.Contains(out, "#5:1") {
		t.Errorf("collapse all left a child visible:\n%s", out)
	}

	buf.Reset()
	s.Expand("all")
	if !strings.Contains(buf.String(), "#5:4") {
		t.Errorf("expand all did not restore the tree:\n%s", buf.String())
	}
	if len(s.Collapsed) != 0 {
		t.Errorf("expand all left %d collapsed", len(s.Collapsed))
	}
}

// `tree <depth>` is the overview: keep the top levels, fold the rest.
func TestTreeDepthCollapsesBelowTheGivenLevel(t *testing.T) {
	s, buf := nestedStepper(t)
	s.ShowTree(" 1")

	out := buf.String()
	for _, visible := range []string{"#5:0", "#5:1", "#5:5"} {
		if !strings.Contains(out, visible) {
			t.Errorf("depth 1 hid %s, which is at or above it:\n%s", visible, out)
		}
	}
	for _, hidden := range []string{"#5:2", "#5:4", "#5:6"} {
		if strings.Contains(out, hidden) {
			t.Errorf("depth 1 left %s visible:\n%s", hidden, out)
		}
	}

	// Depth 0 folds the roots themselves.
	buf.Reset()
	s.ShowTree(" 0")
	if strings.Contains(buf.String(), "#5:1") {
		t.Errorf("depth 0 should fold the root:\n%s", buf.String())
	}
}

func TestTreeRejectsNonNumericDepth(t *testing.T) {
	s, buf := nestedStepper(t)
	s.ShowTree(" sideways")
	if !strings.Contains(buf.String(), "usage: tree") {
		t.Errorf("bad depth = %q, want usage", buf.String())
	}
}

// A collapsed node announces itself with the hidden-count line, which carries
// the count and the command to reopen it. A separate "+" marker said the same
// thing less usefully, and cost a reserved column on every line of the tree.
func TestCollapsedNodeAnnouncesWhatIsHidden(t *testing.T) {
	s, buf := nestedStepper(t)
	s.ShowTree("")
	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("a fully expanded tree claims something is hidden:\n%s", buf.String())
	}

	buf.Reset()
	s.Collapse("#5:1")
	out := buf.String()
	if !strings.Contains(out, "3 events hidden (expand #5:1)") {
		t.Errorf("collapsed node does not say what it hides:\n%s", out)
	}
	// The sibling stays open.
	if !strings.Contains(out, "#5:5") {
		t.Errorf("sibling disappeared:\n%s", out)
	}
}

// Collapsing something with no children would be a silent no-op, which reads
// as a broken command.
func TestCollapseLeafAndUnknownEventExplainThemselves(t *testing.T) {
	s, buf := nestedStepper(t)

	s.Collapse("#5:2")
	if !strings.Contains(buf.String(), "no children") {
		t.Errorf("collapsing a leaf = %q", buf.String())
	}

	buf.Reset()
	s.Collapse("#nope")
	if !strings.Contains(buf.String(), "no event") {
		t.Errorf("collapsing an unknown id = %q", buf.String())
	}
}

// With no argument the commands act on the current step, which is the common case.
func TestCollapseDefaultsToTheCurrentEvent(t *testing.T) {
	s, buf := nestedStepper(t)
	s.Collapse("")
	if !s.Collapsed[s.Order[s.Index]] {
		t.Errorf("bare collapse did not act on the current event:\n%s", buf.String())
	}
}

// The tree is where a filter's shape becomes visible: matches are marked in
// place, so a reader sees which branch to open rather than losing the structure.
func TestTreeMarksFilterMatches(t *testing.T) {
	s, buf := nestedStepper(t)

	// Without a filter the marker column must not appear at all -- marking
	// every node says nothing.
	s.ShowTree("")
	if strings.Contains(buf.String(), "*") {
		t.Errorf("unfiltered tree has match markers:\n%s", buf.String())
	}

	buf.Reset()
	s.Dispatch("filter kind create")
	buf.Reset()
	s.ShowTree("")

	out := buf.String()
	if !strings.Contains(out, "*") {
		t.Fatalf("filtered tree has no match markers:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		// #5:2 is an archive, so it must not be marked; #5:6 is a create.
		if strings.Contains(line, "#5:2") && strings.Contains(line, "*") {
			t.Errorf("non-matching event marked: %q", line)
		}
		if strings.Contains(line, "#5:6") && !strings.Contains(line, "*") {
			t.Errorf("matching event not marked: %q", line)
		}
	}

	// Every marker must start at column 0. Placed after the indentation they
	// land in a different column at each depth, and the reader cannot scan
	// down the tree for them.
	for _, line := range strings.Split(out, "\n") {
		if i := strings.Index(line, "*"); i > 0 {
			t.Errorf("match marker at column %d, want a fixed gutter: %q", i, line)
		}
	}
}

func TestDescendantCountIgnoresUnknownIDs(t *testing.T) {
	s, _ := nestedStepper(t)
	if got := s.descendantCount("#nope"); got != 0 {
		t.Errorf("descendantCount(unknown) = %d", got)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "event"); got != "1 event" {
		t.Errorf("got %q", got)
	}
	if got := plural(3, "event"); got != "3 events" {
		t.Errorf("got %q", got)
	}
}

// A ledger's event ids are not one shape: Canton updates use bare integers,
// other sources use "#2:0". Breakpoints have always taken any of the three
// forms, and collapse/expand taking fewer meant one event had a different name
// depending on which command you were typing.
func TestResolveEventAcceptsIDHashAndStepNumber(t *testing.T) {
	s, _ := nestedStepper(t)

	for _, spec := range []string{"#5:1", "5:1"} {
		if got, ok := s.ResolveEvent(spec); !ok || got != "#5:1" {
			t.Errorf("ResolveEvent(%q) = (%q, %v), want #5:1", spec, got, ok)
		}
	}

	// Step numbers are 1-based, so step 1 is the first event in Order.
	if got, ok := s.ResolveEvent("1"); !ok || got != s.Order[0] {
		t.Errorf("ResolveEvent(\"1\") = (%q, %v), want %q", got, ok, s.Order[0])
	}

	for _, absent := range []string{"", "  ", "#nope", "999"} {
		if got, ok := s.ResolveEvent(absent); ok {
			t.Errorf("ResolveEvent(%q) resolved to %q, want no match", absent, got)
		}
	}
}

// When ids are bare integers -- which is what a Canton update gives -- a digit
// is both a plausible id and a plausible step number. The id wins, because it
// is what the tree prints next to the event.
func TestResolveEventPrefersAnExactIDOverAStepNumber(t *testing.T) {
	var buf bytes.Buffer
	trace := load(t, "tests/fixtures/compare/trace-b.json")
	s := New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf)

	// trace-b uses "#2:0"-style ids, so a digit can only be a step number.
	if got, ok := s.ResolveEvent("1"); !ok || got != s.Order[0] {
		t.Errorf("with no numeric ids, \"1\" should be step 1: got %q, %v", got, ok)
	}
}

// collapse and expand accept whatever ResolveEvent accepts.
func TestCollapseAcceptsEveryIDForm(t *testing.T) {
	for _, spec := range []string{"#5:1", "5:1"} {
		s, buf := nestedStepper(t)
		s.Collapse(spec)
		if !s.Collapsed["#5:1"] {
			t.Errorf("collapse %q did not collapse #5:1:\n%s", spec, buf.String())
		}
	}
}

// `tree 2` means a depth and `collapse 2` means a step, and against a Canton
// update -- where ids are bare integers -- a digit is a plausible id as well.
// Echoing what was resolved turns a silent wrong guess into a visible one.
func TestCollapseEchoesWhatItResolved(t *testing.T) {
	s, buf := nestedStepper(t)
	s.Collapse("2")
	out := buf.String()
	if !strings.Contains(out, "collapsed #5:1 (step 2)") {
		t.Errorf("collapse 2 did not say what it resolved:\n%s", out)
	}

	buf.Reset()
	s.Expand("#5:1")
	if !strings.Contains(buf.String(), "expanded #5:1 (step 2)") {
		t.Errorf("expand did not echo:\n%s", buf.String())
	}

	// The bare form acts on the current event and says which that was.
	buf.Reset()
	s.Collapse("")
	if !strings.Contains(buf.String(), "collapsed "+s.Order[s.Index]) {
		t.Errorf("bare collapse did not echo:\n%s", buf.String())
	}
}
