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
	s, buf := stepperFor(t, "examples/unassign.trace.json")
	s.ShowStateDiff()
	if !strings.Contains(buf.String(), "no contract was created or archived") {
		t.Errorf("empty diff = %q", buf.String())
	}
}

// A contract created and consumed by the same transaction leaves nothing
// behind, but appears on both sides of the diff. Unmarked, it reads as two
// unrelated contracts that a reader has to pair by eye.
func TestStateDiffMarksTransientContracts(t *testing.T) {
	created := []StateChange{{EventID: "#5:3", ContractID: "00aa", Template: "pkg:Asset:Asset"}}
	archived := []StateChange{
		{EventID: "#5:5", ContractID: "00aa", Template: "pkg:Asset:Asset"},
		{EventID: "#5:1", ContractID: "00bb", Template: "pkg:Asset:Asset"},
	}

	transient := transientContracts(created, archived)
	if len(transient) != 1 || !transient["00aa"] {
		t.Fatalf("transient = %v, want just 00aa", transient)
	}
	if transient["00bb"] {
		t.Error("a contract archived but not created was called transient")
	}
}

// The id column is built to be scanned, so it has to fit the ids it holds:
// real Canton ids are "#<txid>:<n>", not four characters.
func TestStateDiffIDColumnFitsTheWidestID(t *testing.T) {
	width := eventIDWidth(
		[]StateChange{{EventID: "#5:3"}},
		[]StateChange{{EventID: "#123:10"}},
	)
	if width != len("#123:10") {
		t.Errorf("width = %d, want %d", width, len("#123:10"))
	}
}

// An archived contract normally has no payload -- a consuming exercise names
// what it destroyed, not its fields. A transient is the exception: this same
// transaction created it, so the fields are already in the trace.
func TestArchivedTransientKeepsItsPayload(t *testing.T) {
	created := []StateChange{{EventID: "#5:3", ContractID: "00aa", Payload: "fields"}}
	archived := []StateChange{
		{EventID: "#5:5", ContractID: "00aa"},
		{EventID: "#5:1", ContractID: "00bb"},
	}

	filled := withKnownPayloads(archived, created)
	if filled[0].Payload != "fields" {
		t.Errorf("transient payload = %v, want it filled from the create", filled[0].Payload)
	}
	if filled[1].Payload != nil {
		t.Errorf("a contract this transaction did not create gained a payload: %v", filled[1].Payload)
	}
	if archived[0].Payload != nil {
		t.Error("the caller's slice was mutated")
	}
}
