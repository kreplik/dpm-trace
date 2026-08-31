package model

import (
	"strings"
	"testing"
)

// A cyclic artifact must be refused at load. Every renderer recurses on
// ChildEventIDs assuming a tree, so accepting one costs unbounded output or a
// stack overflow rather than a wrong answer.
func TestTraceFromJSONRejectsCycles(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"two-event loop", `{
			"updateId": "u1",
			"rootEventIds": ["#5:0"],
			"eventsById": {
				"#5:0": {"eventId": "#5:0", "kind": "exercise", "childEventIds": ["#5:1"]},
				"#5:1": {"eventId": "#5:1", "kind": "exercise", "childEventIds": ["#5:0"]}
			}
		}`},
		{"self loop", `{
			"updateId": "u1",
			"rootEventIds": ["#5:0"],
			"eventsById": {
				"#5:0": {"eventId": "#5:0", "kind": "exercise", "childEventIds": ["#5:0"]}
			}
		}`},
		{"cycle among orphans", `{
			"updateId": "u1",
			"rootEventIds": ["#5:9"],
			"eventsById": {
				"#5:9": {"eventId": "#5:9", "kind": "create", "childEventIds": []},
				"#5:0": {"eventId": "#5:0", "kind": "exercise", "childEventIds": ["#5:1"]},
				"#5:1": {"eventId": "#5:1", "kind": "exercise", "childEventIds": ["#5:0"]}
			}
		}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Decode([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			_, err = TraceFromJSON(data)
			if err == nil {
				t.Fatal("cyclic trace was accepted")
			}
			if !strings.Contains(err.Error(), "descendant") {
				t.Errorf("error = %q, want it to name the cycle", err)
			}
		})
	}
}

// Two parents naming the same child is not a cycle, and a trace whose events
// are only partially linked still renders: neither may be refused.
func TestTraceFromJSONAcceptsNonCycles(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"shared child", `{
			"updateId": "u1",
			"rootEventIds": ["#5:0", "#5:1"],
			"eventsById": {
				"#5:0": {"eventId": "#5:0", "kind": "exercise", "childEventIds": ["#5:2"]},
				"#5:1": {"eventId": "#5:1", "kind": "exercise", "childEventIds": ["#5:2"]},
				"#5:2": {"eventId": "#5:2", "kind": "create", "childEventIds": []}
			}
		}`},
		{"orphan", `{
			"updateId": "u1",
			"rootEventIds": ["#5:0"],
			"eventsById": {
				"#5:0": {"eventId": "#5:0", "kind": "create", "childEventIds": []},
				"#5:7": {"eventId": "#5:7", "kind": "create", "childEventIds": []}
			}
		}`},
		{"child id that does not exist", `{
			"updateId": "u1",
			"rootEventIds": ["#5:0"],
			"eventsById": {
				"#5:0": {"eventId": "#5:0", "kind": "exercise", "childEventIds": ["#5:9"]}
			}
		}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Decode([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := TraceFromJSON(data); err != nil {
				t.Fatalf("rejected a valid trace: %v", err)
			}
		})
	}
}
