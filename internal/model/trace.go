package model

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
