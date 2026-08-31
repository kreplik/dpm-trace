package model

import "fmt"

// Event is one node of a participant-visible transaction, or one half of a
// reassignment. Ports TraceEvent from cli.py.
type Event struct {
	EventID       string
	Kind          string
	Template      string
	ContractID    string
	Choice        string
	Consuming     *bool
	ActingParties []string
	Witnesses     []string
	Signatories   []string
	Observers     []string
	ChildEventIDs []string
	Payload       any
	Argument      any
	Result        any

	// Reassignment (assign/unassign) metadata; zero for transaction events.
	SourceSynchronizer  string
	TargetSynchronizer  string
	ReassignmentID      string
	ReassignmentCounter *int64
	Submitter           string

	Raw *Object
}

// Trace is a normalized update: the events, their parent/child structure, and
// the projection context they were read under. Ports NormalizedTrace.
type Trace struct {
	UpdateID       string
	Source         string
	SourceURL      string
	Projection     Projection
	RootEventIDs   []string
	EventsByID     map[string]*Event
	RecordTime     string
	Offset         string
	SynchronizerID string
	Raw            *Object
}

// CheckAcyclic reports an error if the child links contain a cycle. Every
// renderer and walker recurses on ChildEventIDs assuming a tree, so a cycle
// costs unbounded output or a stack overflow rather than a wrong answer. A
// ledger cannot produce one -- child links are derived strictly forward -- but
// artifacts are arbitrary files read from disk, so the check belongs here,
// where both constructors pass through, rather than in each walker.
func (t *Trace) CheckAcyclic() error {
	const (
		unvisited = 0
		onPath    = 1
		done      = 2
	)
	state := make(map[string]int, len(t.EventsByID))

	var visit func(string) error
	visit = func(eventID string) error {
		switch state[eventID] {
		case onPath:
			return fmt.Errorf("event %s is its own descendant", eventID)
		case done:
			return nil
		}
		ev, ok := t.EventsByID[eventID]
		if !ok {
			return nil
		}
		state[eventID] = onPath
		for _, child := range ev.ChildEventIDs {
			if err := visit(child); err != nil {
				return err
			}
		}
		state[eventID] = done
		return nil
	}

	for _, root := range t.RootEventIDs {
		if err := visit(root); err != nil {
			return err
		}
	}
	// Orphans are legitimate -- a truncated or partially linked trace still
	// renders -- but they carry subtrees of their own.
	ids := make([]string, 0, len(t.EventsByID))
	for id := range t.EventsByID {
		ids = append(ids, id)
	}
	sortEventIDs(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// Projection records what the output is and is not: which party rights it was
// read under, and whether it is a participant projection or public Scan data.
// The tool must never present either as a global Canton transaction.
type Projection struct {
	Source           string
	ParticipantScope bool
	ReadAs           []string
	NotGlobal        bool
	Note             string
}

// Event kinds.
const (
	KindCreate   = "create"
	KindExercise = "exercise"
	KindArchive  = "archive"
	KindAssign   = "assign"
	KindUnassign = "unassign"
	KindEvent    = "event" // unrecognized variant
)

// Projection notes, reproduced verbatim from cli.py: they are user-visible and
// pinned by the golden harness.
const (
	scanNote        = "Public Scan projection. Event ids may be Scan-indexed and are not the same as a participant projection."
	participantNote = "Authorized participant projection. Private data outside these party rights is not available."
)

// Event returns the event with the given id, if present.
func (t *Trace) Event(id string) (*Event, bool) {
	ev, ok := t.EventsByID[id]
	return ev, ok
}
