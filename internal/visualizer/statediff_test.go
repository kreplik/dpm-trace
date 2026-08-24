package visualizer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

func stepperFor(t *testing.T, rel string) (*Stepper, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	trace := load(t, rel)
	return New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf), &buf
}

// A transaction tree carries no archived event: the Ledger API reports the
// archive as `consuming: true` on the exercise. The archived side of the diff
// therefore has to be derived, and getting that wrong is the whole risk here.
func TestStateDiffDerivesArchivesFromConsumingExercises(t *testing.T) {
	s, _ := stepperFor(t, "examples/exercise-child-create.trace.json")
	created, archived := StateDiff(s.Trace, s.Order)

	if len(created) != 2 {
		t.Errorf("Split created %d contracts, want 2", len(created))
	}
	// Split is consuming: the original is gone, though no archive event exists.
	if len(archived) != 1 {
		t.Fatalf("Split archived %d contracts, want 1", len(archived))
	}
	if archived[0].ContractID == "" {
		t.Error("the archived contract has no id")
	}
	for _, c := range created {
		if c.ContractID == archived[0].ContractID {
			t.Errorf("a created contract reuses the archived id %q", c.ContractID)
		}
	}
}

func TestStateDiffPanelShowsBothSides(t *testing.T) {
	s, buf := stepperFor(t, "examples/exercise-child-create.trace.json")
	s.ShowStateDiff()

	out := buf.String()
	for _, want := range []string{"2 contracts created", "1 contract archived", "+ created", "x archived"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q:\n%s", want, out)
		}
	}
	// The payload is what tells two contracts of one template apart.
	if !strings.Contains(out, "quantity: 60") || !strings.Contains(out, "quantity: 40") {
		t.Errorf("panel does not distinguish the two creates:\n%s", out)
	}
	// A reader asking what a transaction did to the ledger is exactly the one
	// who might read this as the global effect.
	if !strings.Contains(out, "visible to") {
		t.Errorf("panel omits the projection caveat:\n%s", out)
	}
}

func TestStateDiffOnEachShape(t *testing.T) {
	for _, tc := range []struct{ artifact, wantCreated, wantArchived string }{
		{"examples/create.trace.json", "1 contract created", "0 contracts archived"},
		{"examples/archive.trace.json", "0 contracts created", "1 contract archived"},
	} {
		s, buf := stepperFor(t, tc.artifact)
		s.ShowStateDiff()
		out := buf.String()
		if !strings.Contains(out, tc.wantCreated) || !strings.Contains(out, tc.wantArchived) {
			t.Errorf("%s:\n%s", tc.artifact, out)
		}
	}
}

// An update that touches no contract on this participant must say so, rather
// than printing an empty panel that looks broken.
func TestStateDiffWithNoChangesSaysSo(t *testing.T) {
	s, buf := stepperFor(t, "tests/fixtures/reassignment/real-unassign-artifact.json")
	s.ShowStateDiff()
	if !strings.Contains(buf.String(), "no contract was created or archived") {
		t.Errorf("empty diff = %q", buf.String())
	}
}
