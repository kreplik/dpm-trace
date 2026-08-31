package visualizer

import (
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// A reassignment carries none of the four party lists -- its acting party is
// Submitter -- and its identity is the reassignment id, not a contract id. The
// renderer treats all of these as first-class, so search has to as well.
func TestFilterMatchesReassignmentMetadata(t *testing.T) {
	ev := &model.Event{
		EventID:            "#5:0",
		Kind:               model.KindUnassign,
		Template:           "pkg123:Iou:Iou",
		Submitter:          "Alice::1220aa",
		ReassignmentID:     "0012200ce3d1",
		SourceSynchronizer: "sync-a::1220e1",
		TargetSynchronizer: "sync-b::1220b1",
	}

	for _, tc := range []struct {
		name   string
		filter Filter
		want   bool
	}{
		{"submitter is a party", Filter{Field: "party", Value: "Alice"}, true},
		{"other party is not", Filter{Field: "party", Value: "Mallory"}, false},
		{"reassignment id is an id", Filter{Field: "id", Value: "0012200c"}, true},
		{"event id still an id", Filter{Field: "id", Value: "5:0"}, true},
		{"unqualified submitter", Filter{Value: "Alice"}, true},
		{"unqualified reassignment id", Filter{Value: "0012200c"}, true},
		{"unqualified source synchronizer", Filter{Value: "sync-a"}, true},
		{"unqualified target synchronizer", Filter{Value: "sync-b"}, true},
		{"unqualified miss", Filter{Value: "sync-c"}, false},
		{"source synchronizer", Filter{Field: "synchronizer", Value: "sync-a"}, true},
		{"target synchronizer", Filter{Field: "synchronizer", Value: "sync-b"}, true},
		{"absent synchronizer", Filter{Field: "synchronizer", Value: "sync-c"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.filter.Matches(ev); got != tc.want {
				t.Errorf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}
