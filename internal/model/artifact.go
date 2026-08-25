package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if schema := pickString(artifact, "schema"); schema != TraceArtifactSchema {
		if LooksLikeCompletion(artifact) {
			return nil, fmt.Errorf(
				"%s is completion data, not a trace artifact.\nA failed submission has no "+
					"transaction tree; read it with --completion-file instead of open", path)
		}
		if schema == "" {
			return nil, fmt.Errorf("%s has no schema field, so it is not a trace artifact", path)
		}
		return nil, fmt.Errorf("unsupported trace artifact schema in %s: '%s'", path, schema)
	}
	if _, ok := pickObject(artifact, "trace"); !ok {
		return nil, fmt.Errorf("trace artifact is missing trace object: %s", path)
	}
	return artifact, nil
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

// TraceToJSON renders a trace in the artifact encoding. Ports trace_to_json.
func TraceToJSON(t *Trace) map[string]any {
	events := map[string]any{}
	for id, ev := range t.EventsByID {
		events[id] = EventToJSON(ev)
	}
	return map[string]any{
		"updateId":       t.UpdateID,
		"source":         t.Source,
		"sourceUrl":      nilIfBlank(t.SourceURL),
		"projection":     t.Projection.json(),
		"recordTime":     nilIfBlank(t.RecordTime),
		"offset":         nilIfBlank(t.Offset),
		"synchronizerId": nilIfBlank(t.SynchronizerID),
		"rootEventIds":   stringsJSON(t.RootEventIDs),
		"eventsById":     events,
	}
}

// EventToJSON renders one event in the artifact encoding. Ports event_to_json.
func EventToJSON(ev *Event) map[string]any {
	return map[string]any{
		"eventId":             ev.EventID,
		"kind":                ev.Kind,
		"template":            nilIfBlank(ev.Template),
		"contractId":          nilIfBlank(ev.ContractID),
		"choice":              nilIfBlank(ev.Choice),
		"consuming":           boolOrNil(ev.Consuming),
		"actingParties":       stringsJSON(ev.ActingParties),
		"witnesses":           stringsJSON(ev.Witnesses),
		"signatories":         stringsJSON(ev.Signatories),
		"observers":           stringsJSON(ev.Observers),
		"childEventIds":       stringsJSON(ev.ChildEventIDs),
		"payload":             ev.Payload,
		"argument":            ev.Argument,
		"result":              ev.Result,
		"sourceSynchronizer":  nilIfBlank(ev.SourceSynchronizer),
		"targetSynchronizer":  nilIfBlank(ev.TargetSynchronizer),
		"reassignmentId":      nilIfBlank(ev.ReassignmentID),
		"reassignmentCounter": int64OrNil(ev.ReassignmentCounter),
		"submitter":           nilIfBlank(ev.Submitter),
	}
}

func (p Projection) json() map[string]any {
	return map[string]any{
		"source":            p.Source,
		"participantScoped": p.ParticipantScope,
		"readAs":            stringsJSON(p.ReadAs),
		"notGlobal":         p.NotGlobal,
		"note":              p.Note,
	}
}

func boolOrNil(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

// NewTraceArtifact wraps a trace for export. Ports create_trace_artifact.
//
// The privacy block is not decoration: an exported artifact travels, and it
// must carry what projection it came from and what is deliberately absent.
func NewTraceArtifact(t *Trace, ledgerURL, scanURL string, darPaths, debugInfoPaths []string) map[string]any {
	packageIDs := map[string]bool{}
	for _, ev := range t.EventsByID {
		if id := packageFromTemplate(ev.Template); id != "" {
			packageIDs[id] = true
		}
	}
	sorted := make([]string, 0, len(packageIDs))
	for id := range packageIDs {
		sorted = append(sorted, id)
	}
	sortStrings(sorted)

	return map[string]any{
		"schema":    TraceArtifactSchema,
		"createdAt": isoTimestamp(time.Now().UTC()),
		"kind":      "committed-update",
		"trace":     TraceToJSON(t),
		"participant": map[string]any{
			"ledgerUrl": nilIfBlank(ledgerURL),
			"scanUrl":   nilIfBlank(scanURL),
			"readAs":    stringsJSON(t.Projection.ReadAs),
		},
		"packages": packageMetadataContext(darPaths, debugInfoPaths, sorted),
		"privacy": map[string]any{
			"scope":                    nilIfBlank(t.Projection.Note),
			"readAs":                   stringsJSON(t.Projection.ReadAs),
			"missingPrivateDataPolicy": "private data outside this participant projection is not present in the artifact",
		},
	}
}

// packageMetadataContext records which local package metadata was available.
// Ports package_metadata_context.
func packageMetadataContext(darPaths, debugInfoPaths, packageIDs []string) map[string]any {
	found, missing := splitExisting(darPaths)
	foundDebug, missingDebug := splitExisting(debugInfoPaths)

	status := "package ids captured; package/source metadata must be supplied by local project or registry"
	available := len(found) > 0 || len(foundDebug) > 0
	if available {
		status = "local package/source metadata attached"
	}
	return map[string]any{
		"available":             available,
		"packageIds":            stringsJSON(packageIDs),
		"darPaths":              stringsJSON(found),
		"debugInfoPaths":        stringsJSON(foundDebug),
		"missingDarPaths":       stringsJSON(missing),
		"missingDebugInfoPaths": stringsJSON(missingDebug),
		"status":                status,
	}
}

func splitExisting(paths []string) (found, missing []string) {
	for _, path := range paths {
		resolved := resolvePath(path)
		if _, err := os.Stat(resolved); err == nil {
			found = append(found, resolved)
		} else {
			missing = append(missing, resolved)
		}
	}
	return found, missing
}

// resolvePath is str(Path(path).expanduser()): the tilde is expanded and the
// path is tidied, but ".." is left alone because pathlib does not resolve it
// without a filesystem lookup. filepath.Clean would collapse it, so the tidying
// is done by hand.
func resolvePath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	rooted := strings.HasPrefix(path, "/")
	segments := strings.Split(path, "/")
	kept := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment == "" || segment == "." {
			continue
		}
		kept = append(kept, segment)
	}
	out := strings.Join(kept, "/")
	if rooted {
		out = "/" + out
	}
	if out == "" {
		return "."
	}
	return out
}

// packageFromTemplate returns the package id of a package:module:entity template.
func packageFromTemplate(template string) string {
	// One colon is enough: package_from_template splits on the first one, so a
	// "pkg:Module" reference still yields its package id.
	if package_, _, found := strings.Cut(template, ":"); found {
		return package_
	}
	return ""
}

// isoTimestamp formats a UTC instant the way datetime.isoformat() does: the
// microseconds are omitted when they are zero rather than printed as .000000.
func isoTimestamp(t time.Time) string {
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05Z")
	}
	return t.Format("2006-01-02T15:04:05.000000Z")
}
