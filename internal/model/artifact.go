package model

import (
	"fmt"
	"os"
)

// TraceArtifactSchema is the schema string exported artifacts carry.
const TraceArtifactSchema = "dpm-trace/trace-artifact/v0"

// LoadTraceArtifact reads and validates an exported trace artifact.
// Ports load_trace_artifact.
func LoadTraceArtifact(path string) (*Object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	artifact, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("trace artifact must be a JSON object: %s", path)
	}
	if schema := pickString(artifact, "schema"); schema != TraceArtifactSchema {
		return nil, fmt.Errorf("unsupported trace artifact schema in %s: %s", path, quoteOrNone(schema))
	}
	if _, ok := pickObject(artifact, "trace"); !ok {
		return nil, fmt.Errorf("trace artifact is missing trace object: %s", path)
	}
	return artifact, nil
}

// quoteOrNone renders a missing schema the way Python's %r renders None.
func quoteOrNone(value string) string {
	if value == "" {
		return "None"
	}
	return "'" + value + "'"
}

// TraceFromArtifact extracts the trace from a loaded artifact.
// Ports trace_from_artifact.
func TraceFromArtifact(artifact *Object) (*Trace, error) {
	inner, ok := pickObject(artifact, "trace")
	if !ok {
		return nil, fmt.Errorf("trace artifact is missing trace object")
	}
	return TraceFromJSON(inner)
}

// TraceFromJSON reads the already-normalized artifact encoding of a trace.
// This is not normalization: the artifact stores the model's own field names,
// so nothing is re-derived from Ledger API shapes. Ports trace_from_json.
func TraceFromJSON(data *Object) (*Trace, error) {
	eventsJSON, ok := pickObject(data, "eventsById")
	if !ok {
		return nil, fmt.Errorf("trace.eventsById must be an object")
	}

	eventsByID := make(map[string]*Event, eventsJSON.Len())
	for _, id := range eventsJSON.Keys() {
		raw, _ := eventsJSON.Get(id)
		obj, isObject := asObject(raw)
		if !isObject {
			continue
		}
		eventsByID[id] = EventFromJSON(obj)
	}

	source := pickString(data, "source")
	if source == "" {
		source = "artifact"
	}

	trace := &Trace{
		UpdateID:       pickString(data, "updateId"),
		Source:         source,
		SourceURL:      pickString(data, "sourceUrl"),
		RootEventIDs:   listString(pick(data, "rootEventIds")),
		EventsByID:     eventsByID,
		RecordTime:     pickString(data, "recordTime"),
		Offset:         pickString(data, "offset"),
		SynchronizerID: pickString(data, "synchronizerId"),
	}
	if projection, ok := pickObject(data, "projection"); ok {
		trace.Projection = Projection{
			Source:           pickString(projection, "source"),
			ParticipantScope: pick(projection, "participantScoped") == true,
			ReadAs:           listString(pick(projection, "readAs")),
			NotGlobal:        pick(projection, "notGlobal") == true,
			Note:             pickString(projection, "note"),
		}
	}
	return trace, nil
}

// EventFromJSON reads one event from the artifact encoding. Ports event_from_json.
func EventFromJSON(data *Object) *Event {
	return &Event{
		EventID:             pickString(data, "eventId"),
		Kind:                orDefault(pickString(data, "kind"), KindEvent),
		Template:            pickString(data, "template"),
		ContractID:          pickString(data, "contractId"),
		Choice:              pickString(data, "choice"),
		Consuming:           asBool(pick(data, "consuming")),
		ActingParties:       listString(pick(data, "actingParties")),
		Witnesses:           listString(pick(data, "witnesses")),
		Signatories:         listString(pick(data, "signatories")),
		Observers:           listString(pick(data, "observers")),
		ChildEventIDs:       listString(pick(data, "childEventIds")),
		Payload:             pick(data, "payload"),
		Argument:            pick(data, "argument"),
		Result:              pick(data, "result"),
		SourceSynchronizer:  pickString(data, "sourceSynchronizer"),
		TargetSynchronizer:  pickString(data, "targetSynchronizer"),
		ReassignmentID:      pickString(data, "reassignmentId"),
		ReassignmentCounter: asInt(pick(data, "reassignmentCounter")),
		Submitter:           pickString(data, "submitter"),
	}
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
