package model

import "sort"

// Comparison kinds.
const (
	KindUpdateVsUpdate       = "update-vs-update"
	KindPreparedVsUpdate     = "prepared-vs-update"
	KindPreparedVsCompletion = "prepared-vs-completion"
)

// TraceSummary is the per-side header of a comparison.
// Ports trace_summary_for_compare.
type TraceSummary struct {
	UpdateID   string
	Source     string
	Offset     string
	RecordTime string
	ReadAs     []string
	Events     int
}

// CompareRow is one event in a comparison, with its children inlined.
// Ports event_compare_row.
type CompareRow struct {
	EventID             string
	Kind                string
	Template            string
	Choice              string
	ContractID          string // already truncated to 48, as Python stores it
	Value               any
	ValueLabel          string
	Result              any
	Signatories         []string
	Observers           []string
	Witnesses           []string
	ActingParties       []string
	SourceSynchronizer  string
	TargetSynchronizer  string
	ReassignmentCounter *int64
	Children            []CompareRow
}

// Comparison is the result of comparing two updates.
type Comparison struct {
	Kind                 string
	Left                 TraceSummary
	Right                TraceSummary
	LeftCounts           map[string]int
	RightCounts          map[string]int
	LeftRoots            []CompareRow
	RightRoots           []CompareRow
	TemplatesOnlyInLeft  []string
	TemplatesOnlyInRight []string
}

// CompareTraces builds an update-vs-update comparison. Ports compare_traces.
func CompareTraces(left, right *Trace) *Comparison {
	return &Comparison{
		Kind:                 KindUpdateVsUpdate,
		Left:                 SummarizeTrace(left),
		Right:                SummarizeTrace(right),
		LeftCounts:           StateDiffCounts(left),
		RightCounts:          StateDiffCounts(right),
		LeftRoots:            rootRows(left),
		RightRoots:           rootRows(right),
		TemplatesOnlyInLeft:  templateDifference(left, right),
		TemplatesOnlyInRight: templateDifference(right, left),
	}
}

// SummarizeTrace ports trace_summary_for_compare.
func SummarizeTrace(trace *Trace) TraceSummary {
	return TraceSummary{
		UpdateID:   trace.UpdateID,
		Source:     trace.Source,
		Offset:     trace.Offset,
		RecordTime: trace.RecordTime,
		ReadAs:     trace.Projection.ReadAs,
		Events:     len(trace.EventsByID),
	}
}

// StateDiffCounts counts what the transaction did, per event kind.
//
// A consuming exercise is counted as both an exercise and an archive, matching
// render.StateDiffSummary: a transaction tree reports an archive as
// `consuming: true` rather than as a separate archived event. The two tallies
// have to agree, or the compare view and the trace view disagree about the same
// transaction.
func StateDiffCounts(trace *Trace) map[string]int {
	counts := map[string]int{"create": 0, "exercise": 0, "archive": 0, "assign": 0, "unassign": 0, "other": 0}
	for _, ev := range trace.EventsByID {
		if _, known := counts[ev.Kind]; known {
			counts[ev.Kind]++
		} else {
			counts["other"]++
		}
		if ev.Kind == KindExercise && ev.Consuming != nil && *ev.Consuming {
			counts[KindArchive]++
		}
	}
	return counts
}

func rootRows(trace *Trace) []CompareRow {
	rows := []CompareRow{}
	for _, id := range trace.RootEventIDs {
		if ev, ok := trace.EventsByID[id]; ok {
			rows = append(rows, EventCompareRow(ev, trace.EventsByID))
		}
	}
	return rows
}

// EventCompareRow ports event_compare_row.
func EventCompareRow(ev *Event, eventsByID map[string]*Event) CompareRow {
	row := CompareRow{
		EventID:             ev.EventID,
		Kind:                ev.Kind,
		Template:            ev.Template,
		Choice:              ev.Choice,
		ContractID:          shortValue(ev.ContractID, 48),
		Signatories:         ev.Signatories,
		Observers:           ev.Observers,
		Witnesses:           ev.Witnesses,
		ActingParties:       ev.ActingParties,
		SourceSynchronizer:  ev.SourceSynchronizer,
		TargetSynchronizer:  ev.TargetSynchronizer,
		ReassignmentCounter: ev.ReassignmentCounter,
		Children:            []CompareRow{},
	}
	switch ev.Kind {
	case KindCreate, KindAssign:
		row.Value = ev.Payload
		row.ValueLabel = "payload"
	case KindExercise:
		row.Value = ev.Argument
		row.ValueLabel = "argument"
		row.Result = ev.Result
	}
	if eventsByID != nil {
		for _, childID := range ev.ChildEventIDs {
			if child, ok := eventsByID[childID]; ok {
				row.Children = append(row.Children, EventCompareRow(child, eventsByID))
			}
		}
	}
	return row
}

func templateDifference(a, b *Trace) []string {
	inB := map[string]bool{}
	for _, ev := range b.EventsByID {
		if ev.Template != "" {
			inB[ev.Template] = true
		}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ev := range a.EventsByID {
		if ev.Template == "" || inB[ev.Template] || seen[ev.Template] {
			continue
		}
		seen[ev.Template] = true
		out = append(out, ev.Template)
	}
	sort.Strings(out)
	return out
}

// shortValue mirrors render's Short but returns "" for empty, because Python
// stores short(None, 48) as "-" only when rendering, not in the row itself.
func shortValue(value string, maxLen int) string {
	if value == "" {
		return "-"
	}
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}

// JSON renders the comparison the way --print-json does. Built from maps so
// encoding sorts keys, matching json.dumps(..., sort_keys=True).
func (c *Comparison) JSON() map[string]any {
	return map[string]any{
		"kind":  c.Kind,
		"left":  c.Left.json(),
		"right": c.Right.json(),
		"diff": map[string]any{
			"eventCounts": map[string]any{
				"left":  intMap(c.LeftCounts),
				"right": intMap(c.RightCounts),
			},
			"rootEvents": map[string]any{
				"left":  rowsJSON(c.LeftRoots),
				"right": rowsJSON(c.RightRoots),
			},
			"templatesOnlyInLeft":  stringsJSON(c.TemplatesOnlyInLeft),
			"templatesOnlyInRight": stringsJSON(c.TemplatesOnlyInRight),
		},
	}
}

func (s TraceSummary) json() map[string]any {
	return map[string]any{
		"updateId":   s.UpdateID,
		"source":     s.Source,
		"offset":     nilIfBlank(s.Offset),
		"recordTime": nilIfBlank(s.RecordTime),
		"readAs":     stringsJSON(s.ReadAs),
		"events":     s.Events,
	}
}

func (r CompareRow) json() map[string]any {
	return map[string]any{
		"eventId":             r.EventID,
		"kind":                r.Kind,
		"template":            nilIfBlank(r.Template),
		"choice":              nilIfBlank(r.Choice),
		"contractId":          r.ContractID,
		"value":               r.Value,
		"valueLabel":          nilIfBlank(r.ValueLabel),
		"result":              r.Result,
		"signatories":         stringsJSON(r.Signatories),
		"observers":           stringsJSON(r.Observers),
		"witnesses":           stringsJSON(r.Witnesses),
		"actingParties":       stringsJSON(r.ActingParties),
		"sourceSynchronizer":  nilIfBlank(r.SourceSynchronizer),
		"targetSynchronizer":  nilIfBlank(r.TargetSynchronizer),
		"reassignmentCounter": int64OrNil(r.ReassignmentCounter),
		"children":            rowsJSON(r.Children),
	}
}

func rowsJSON(rows []CompareRow) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.json())
	}
	return out
}

func stringsJSON(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func intMap(counts map[string]int) map[string]any {
	out := make(map[string]any, len(counts))
	for key, value := range counts {
		out[key] = value
	}
	return out
}

func nilIfBlank(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func int64OrNil(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
