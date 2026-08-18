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

	// Mixed or non-numeric ids fall back to lexical order. Note this applies
	// only where the port sorts deliberately (linkRangeChildren, which mirrors
	// Python's numeric sort). Root order is NOT sorted -- infer_roots keeps
	// document order; see TestInferRootsKeepsDocumentOrder.
	mixed := []string{"#2:1", "#1:0", "#10:0"}
	sortEventIDs(mixed)
	// Lexical, not natural: ':' sorts after '0', so "#10:0" precedes "#1:0".
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
// them all, in document order, rather than rendering an empty tree.
func TestInferRootsFallsBackWhenEverythingIsAChild(t *testing.T) {
	events := map[string]*Event{
		"a": {EventID: "a", ChildEventIDs: []string{"b"}},
		"b": {EventID: "b", ChildEventIDs: []string{"a"}},
	}
	roots := inferRoots(events, []string{"a", "b"})
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

// Depth 3 is where the two possible readings of lastDescendantNodeId diverge:
// a node covers its whole subtree, so every id in (self, last] is a descendant
// but only some are direct children. Claiming all of them attaches
// grandchildren to the root, and the nested event then renders twice.
func TestLinkRangeChildrenSkipsGrandchildren(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"ExercisedEvent":{"nodeId":0,"lastDescendantNodeId":2,
	         "contractId":"00a","templateId":"pkg:M:T","choice":"Outer","consuming":true}},
	    "1":{"ExercisedEvent":{"nodeId":1,"lastDescendantNodeId":2,
	         "contractId":"00b","templateId":"pkg:M:T","choice":"Inner","consuming":true}},
	    "2":{"CreatedEvent":{"nodeId":2,"contractId":"00c","templateId":"pkg:M:T"}}
	  }}}}`)

	if got := strings.Join(trace.EventsByID["0"].ChildEventIDs, ","); got != "1" {
		t.Errorf("root children = %q, want only the direct child \"1\"", got)
	}
	if got := strings.Join(trace.EventsByID["1"].ChildEventIDs, ","); got != "2" {
		t.Errorf("inner children = %q, want \"2\"", got)
	}

	// Every event must appear exactly once in the rendered tree.
	seen := map[string]int{}
	var walk func(string)
	walk = func(id string) {
		seen[id]++
		for _, child := range trace.EventsByID[id].ChildEventIDs {
			walk(child)
		}
	}
	for _, root := range trace.RootEventIDs {
		walk(root)
	}
	for id, count := range seen {
		if count != 1 {
			t.Errorf("event %s appears %d times in the tree, want once", id, count)
		}
	}
}

// Four levels, and a sibling after the nested subtree: the root takes the
// nested exercise and the later sibling, but nothing in between.
func TestLinkRangeChildrenDeepWithSibling(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "0":{"ExercisedEvent":{"nodeId":0,"lastDescendantNodeId":4,
	         "contractId":"00a","templateId":"pkg:M:T","choice":"A","consuming":true}},
	    "1":{"ExercisedEvent":{"nodeId":1,"lastDescendantNodeId":3,
	         "contractId":"00b","templateId":"pkg:M:T","choice":"B","consuming":true}},
	    "2":{"ExercisedEvent":{"nodeId":2,"lastDescendantNodeId":3,
	         "contractId":"00c","templateId":"pkg:M:T","choice":"C","consuming":true}},
	    "3":{"CreatedEvent":{"nodeId":3,"contractId":"00d","templateId":"pkg:M:T"}},
	    "4":{"CreatedEvent":{"nodeId":4,"contractId":"00e","templateId":"pkg:M:T"}}
	  }}}}`)

	for id, want := range map[string]string{"0": "1,4", "1": "2", "2": "3", "3": "", "4": ""} {
		if got := strings.Join(trace.EventsByID[id].ChildEventIDs, ","); got != want {
			t.Errorf("event %s children = %q, want %q", id, got, want)
		}
	}
}

// Root order is preserved from the document, not sorted: compare pairs root
// events positionally, so reordering them invents differences. A lexical sort
// would put "#10:0" before "#1:0".
func TestInferRootsKeepsDocumentOrder(t *testing.T) {
	trace := normalizeRaw(t, `{"update":{"TransactionTree":{
	  "updateId":"1220aa","offset":1,
	  "eventsById":{
	    "#2:1":{"CreatedEvent":{"contractId":"00b","templateId":"pkg:M:T"}},
	    "#10:0":{"CreatedEvent":{"contractId":"00c","templateId":"pkg:M:T"}},
	    "#1:0":{"CreatedEvent":{"contractId":"00a","templateId":"pkg:M:T"}}
	  }}}}`)

	got := strings.Join(trace.RootEventIDs, ",")
	if got != "#2:1,#10:0,#1:0" {
		t.Errorf("roots = %q, want them in document order", got)
	}
}
