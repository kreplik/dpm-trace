package model

import (
	"strings"
	"testing"
)

// The tree a user reads is inferred, not given: the Ledger API sends a flat
// event map plus node-id ranges. These heuristics decide what nests under what,
// so a mistake here silently reshapes every trace.

func normalizeRaw(t *testing.T, raw string) *Trace {
	t.Helper()
	obj, err := Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := NormalizeTrace(obj, "test", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

// Node ids are numbers encoded as strings. Sorting them lexically would put
// "10" before "2" and scramble the tree.
func TestSortEventIDsIsNumericWhenItCan(t *testing.T) {
	ids := []string{"10", "2", "1", "20", "3"}
	sortEventIDs(ids)
	if strings.Join(ids, ",") != "1,2,3,10,20" {
		t.Errorf("numeric ids sorted as %v", ids)
	}

	// Mixed or non-numeric ids fall back to lexical order, which is at least
	// stable -- map iteration order is not.
	mixed := []string{"#2:1", "#1:0", "#10:0"}
	sortEventIDs(mixed)
	// Lexical, not natural: ':' sorts after '0', so "#10:0" precedes "#1:0".
	// Python's sorted() agrees, which is what parity requires.
	if strings.Join(mixed, ",") != "#10:0,#1:0,#2:1" {
		t.Errorf("non-numeric ids sorted as %v", mixed)
	}

	sortEventIDs(nil)
	sortEventIDs([]string{"only"})
}

// lastDescendantNodeId says "this node covers ids up to N", which is how a
// consuming exercise claims the creates it produced.
func TestLinkRangeChildrenBuildsTheTree(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"ExercisedEvent":{"nodeId":0,"lastDescendantNodeId":2,
	         "contractId":"00a","templateId":"pkg:M:T","choice":"Split","consuming":true}},
	    "1":{"CreatedEvent":{"nodeId":1,"contractId":"00b","templateId":"pkg:M:T"}},
	    "2":{"CreatedEvent":{"nodeId":2,"contractId":"00c","templateId":"pkg:M:T"}}
	  }}}}`)

	root := trace.EventsByID["0"]
	if got := strings.Join(root.ChildEventIDs, ","); got != "1,2" {
		t.Errorf("children = %q, want both creates nested", got)
	}
	if len(trace.RootEventIDs) != 1 || trace.RootEventIDs[0] != "0" {
		t.Errorf("roots = %v, want only the exercise", trace.RootEventIDs)
	}
}

// A node covering only itself claims nothing, or every sibling would nest.
func TestLinkRangeChildrenIgnoresSelfOnlyRanges(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"CreatedEvent":{"nodeId":0,"lastDescendantNodeId":0,"contractId":"00a","templateId":"pkg:M:T"}},
	    "1":{"CreatedEvent":{"nodeId":1,"lastDescendantNodeId":1,"contractId":"00b","templateId":"pkg:M:T"}}
	  }}}}`)

	for _, id := range []string{"0", "1"} {
		if children := trace.EventsByID[id].ChildEventIDs; len(children) != 0 {
			t.Errorf("event %s claimed %v", id, children)
		}
	}
	if len(trace.RootEventIDs) != 2 {
		t.Errorf("roots = %v, want both events", trace.RootEventIDs)
	}
}

// Without any lastDescendantNodeId there is nothing to infer from, so events
// stay flat rather than being guessed into a tree.
func TestLinkRangeChildrenWithoutRanges(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"CreatedEvent":{"nodeId":0,"contractId":"00a","templateId":"pkg:M:T"}},
	    "1":{"CreatedEvent":{"nodeId":1,"contractId":"00b","templateId":"pkg:M:T"}}
	  }}}}`)
	if len(trace.RootEventIDs) != 2 {
		t.Errorf("roots = %v, want both flat", trace.RootEventIDs)
	}
}

// Explicit child links win: the range heuristic must not overwrite them.
func TestExplicitChildrenAreNotOverwritten(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"ExercisedEvent":{"nodeId":0,"lastDescendantNodeId":2,"childEventIds":["1"],
	         "contractId":"00a","templateId":"pkg:M:T","choice":"C"}},
	    "1":{"CreatedEvent":{"nodeId":1,"contractId":"00b","templateId":"pkg:M:T"}},
	    "2":{"CreatedEvent":{"nodeId":2,"contractId":"00c","templateId":"pkg:M:T"}}
	  }}}}`)

	if got := strings.Join(trace.EventsByID["0"].ChildEventIDs, ","); got != "1" {
		t.Errorf("children = %q, want the explicit link kept", got)
	}
}

// Every event being someone's child would leave no root; the fallback shows
// them all rather than rendering an empty tree.
func TestInferRootsFallsBackWhenEverythingIsAChild(t *testing.T) {
	events := map[string]*Event{
		"a": {EventID: "a", ChildEventIDs: []string{"b"}},
		"b": {EventID: "b", ChildEventIDs: []string{"a"}},
	}
	roots := inferRoots(events)
	if len(roots) != 2 {
		t.Errorf("roots = %v, want every event as a fallback", roots)
	}
}

// The kind drives the icon, the colour and the counts, and arrives as free text.
func TestKindFromExplicit(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"CreatedEvent", KindCreate},
		{"created", KindCreate},
		{"ExercisedEvent", KindExercise},
		{"archived", KindArchive},
		{"JsUnassignedEvent", KindUnassign},
		{"JsAssignmentEvent", KindAssign},
		{"something else", KindEvent},
		{"", KindEvent},
	} {
		if got := kindFromExplicit(tc.in); got != tc.want {
			t.Errorf("kindFromExplicit(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// "unassign" contains "assign": the order of the checks is what keeps an
// unassignment from being reported as an assignment.
func TestKindFromExplicitPrefersUnassign(t *testing.T) {
	if got := kindFromExplicit("unassigned"); got != KindUnassign {
		t.Errorf("got %q, want unassign -- the substring order matters", got)
	}
}
