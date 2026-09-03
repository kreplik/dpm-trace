package model

import "testing"

// The committed side of a prepared comparison has children -- a consuming
// exercise and the create it produced. Building those rows without the event
// map dropped the subtree, so the same event carried its children in an update
// comparison and none here.
func TestPreparedComparisonInlinesChildren(t *testing.T) {
	raw := `{
		"updateId": "u1",
		"rootEventIds": ["#2:0"],
		"eventsById": {
			"#2:0": {"eventId": "#2:0", "kind": "exercise", "template": "pkg:Token:Token",
			         "choice": "Transfer", "childEventIds": ["#2:1"]},
			"#2:1": {"eventId": "#2:1", "kind": "create", "template": "pkg:Token:Token",
			         "childEventIds": []}
		}
	}`
	data, err := Decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := TraceFromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := Decode([]byte(`{"request": {"commandId": "c1", "commands": []}}`))
	if err != nil {
		t.Fatal(err)
	}

	c := ComparePreparedToTrace(prepared, trace)
	if len(c.RootEvents) != 1 {
		t.Fatalf("root events = %d, want 1", len(c.RootEvents))
	}
	if len(c.RootEvents[0].Children) != 1 {
		t.Fatalf("children = %v, want the create inlined", c.RootEvents[0].Children)
	}
	if got := c.RootEvents[0].Children[0].EventID; got != "#2:1" {
		t.Errorf("child event id = %q, want #2:1", got)
	}
}
