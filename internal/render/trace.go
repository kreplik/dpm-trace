package render

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// PrettyTrace writes the participant-visible trace: header, party aliases and
// the event tree. Ports print_pretty_trace.
//
// index may be nil, in which case no source lines are emitted -- the same as
// Python without source metadata.
func PrettyTrace(w io.Writer, trace *model.Trace, color Color, index *source.Index) {
	ctx := NewContext(trace)

	fmt.Fprintln(w, color.Apply("Canton trace", "bold"))
	fmt.Fprintf(w, "  update:       %s\n", Short(trace.UpdateID, 80))
	fmt.Fprintf(w, "  source:       %s (%s)\n", trace.Source, orDash(trace.SourceURL))
	fmt.Fprintf(w, "  offset:       %s\n", orDash(trace.Offset))
	fmt.Fprintf(w, "  record time:  %s\n", orDash(trace.RecordTime))
	fmt.Fprintf(w, "  synchronizer: %s\n", Short(trace.SynchronizerID, 80))
	if len(trace.Projection.ReadAs) > 0 {
		rendered := make([]string, 0, len(trace.Projection.ReadAs))
		for _, party := range trace.Projection.ReadAs {
			rendered = append(rendered, ctx.PartyWithFull(party))
		}
		fmt.Fprintf(w, "  read-as:      %s\n", strings.Join(rendered, ", "))
	}
	fmt.Fprintf(w, "  visibility:   %s\n", trace.Projection.Note)
	fmt.Fprintf(w, "  events:       %s\n", StateDiffSummary(trace, color))
	if index != nil && index.HasSources() {
		fmt.Fprintf(w, "  source roots: %s\n", strings.Join(index.Roots, ", "))
	}

	if len(ctx.PartyAliases) > 0 {
		fmt.Fprintln(w, "  parties:")
		// Python sorts by alias, not by party id.
		type pair struct{ party, alias string }
		pairs := make([]pair, 0, len(ctx.PartyAliases))
		for party, alias := range ctx.PartyAliases {
			pairs = append(pairs, pair{party, alias})
		}
		sort.Slice(pairs, func(i, j int) bool {
			if pairs[i].alias != pairs[j].alias {
				return pairs[i].alias < pairs[j].alias
			}
			return pairs[i].party < pairs[j].party
		})
		for _, p := range pairs {
			fmt.Fprintf(w, "    %s = %s\n", p.alias, p.party)
		}
	}
	fmt.Fprintln(w)

	if len(trace.RootEventIDs) == 0 {
		fmt.Fprintln(w, color.Apply("No events found.", "yellow"))
		return
	}

	fmt.Fprintln(w, color.Apply("Trace", "bold"))
	for i, eventID := range trace.RootEventIDs {
		eventTree(w, trace, eventID, "", i == len(trace.RootEventIDs)-1, color, ctx, index)
	}
}

// eventTree writes one event and its descendants. Ports print_event_tree.
func eventTree(w io.Writer, trace *model.Trace, eventID, prefix string, isLast bool, color Color, ctx *Context, index *source.Index) {
	ev, ok := trace.Event(eventID)
	if !ok {
		return
	}

	connector := "|-- "
	childPrefix := "|   "
	if isLast {
		connector = "`-- "
		childPrefix = "    "
	}
	fmt.Fprintln(w, prefix+connector+EventTitle(ev, color))

	details := EventDetailLines(ev, color, ctx, index)
	for i, line := range details {
		detailConnector := "|-- "
		if i == len(details)-1 && len(ev.ChildEventIDs) == 0 {
			detailConnector = "`-- "
		}
		fmt.Fprintln(w, prefix+childPrefix+detailConnector+line)
	}

	for i, childID := range ev.ChildEventIDs {
		eventTree(w, trace, childID, prefix+childPrefix, i == len(ev.ChildEventIDs)-1, color, ctx, index)
	}
}

// EventTitle renders "[id] KIND Template.Choice". Ports event_title.
func EventTitle(ev *model.Event, color Color) string {
	return color.Apply("["+ev.EventID+"]", "gray") + " " +
		color.Apply(EventKindLabel(ev.Kind), EventColor(ev.Kind), "bold") + " " +
		color.Apply(EventTarget(ev), "bold")
}

// EventTarget is the template, with the choice appended when there is one.
func EventTarget(ev *model.Event) string {
	template := ShortTemplate(ev.Template)
	if template == "" {
		template = "<unknown>"
	}
	if ev.Choice != "" {
		return template + "." + ev.Choice
	}
	return template
}

// EventDetailLines renders the indented detail block under an event.
// Ports event_detail_lines.
func EventDetailLines(ev *model.Event, color Color, ctx *Context, index *source.Index) []string {
	var lines []string
	// The source line names the file and line only: the entity label already
	// says which template or choice it is.
	if index != nil {
		if loc := index.LocationForEvent(ev); loc != nil {
			lines = append(lines, LabelValue("source",
				fmt.Sprintf("%s:%d (%s)", filepath.Base(loc.Path), loc.Line, loc.Label), color))
		}
	}
	if ev.ContractID != "" {
		lines = append(lines, LabelValue("contract", Short(ev.ContractID, 66), color))
	}
	if ev.Consuming != nil && ev.Kind == model.KindExercise {
		lines = append(lines, LabelValue("consuming", strconv.FormatBool(*ev.Consuming), color))
	}
	lines = append(lines, ReassignmentDetailLines(ev, color, ctx)...)
	for _, group := range []struct {
		label   string
		parties []string
	}{
		{"actors", ev.ActingParties},
		{"signatories", ev.Signatories},
		{"observers", ev.Observers},
		{"witnesses", ev.Witnesses},
	} {
		if len(group.parties) == 0 {
			continue
		}
		rendered := make([]string, 0, len(group.parties))
		for _, party := range group.parties {
			rendered = append(rendered, ctx.Party(party))
		}
		lines = append(lines, LabelValue(group.label, strings.Join(rendered, ", "), color))
	}
	if ev.Argument != nil {
		lines = append(lines, BlockLines("argument", ev.Argument, color, ctx)...)
	}
	if ev.Payload != nil {
		lines = append(lines, BlockLines("payload", ev.Payload, color, ctx)...)
	}
	if ev.Result != nil {
		lines = append(lines, BlockLines("result", ev.Result, color, ctx)...)
	}
	return lines
}

// ReassignmentDetailLines renders where a contract moved. Emitted for any event
// carrying the metadata, so a partial reassignment payload still shows it.
// Ports reassignment_detail_lines.
func ReassignmentDetailLines(ev *model.Event, color Color, ctx *Context) []string {
	var lines []string
	if ev.SourceSynchronizer != "" || ev.TargetSynchronizer != "" {
		lines = append(lines, LabelValue("reassignment",
			Short(ev.SourceSynchronizer, 40)+" -> "+Short(ev.TargetSynchronizer, 40), color))
	}
	if ev.ReassignmentID != "" {
		lines = append(lines, LabelValue("reassignment id", Short(ev.ReassignmentID, 66), color))
	}
	if ev.ReassignmentCounter != nil {
		lines = append(lines, LabelValue("counter", strconv.FormatInt(*ev.ReassignmentCounter, 10), color))
	}
	if ev.Submitter != "" {
		lines = append(lines, LabelValue("submitter", ctx.Party(ev.Submitter), color))
	}
	return lines
}

// LabelValue renders "label: value" with the label styled.
func LabelValue(label, value string, color Color) string {
	return color.Apply(label+":", "cyan") + " " + value
}

// BlockLines renders a value inline when short, or as an indented block.
// Ports block_lines.
func BlockLines(label string, value any, color Color, ctx *Context) []string {
	rendered := RenderPrettyValue(value, ctx)
	if !strings.Contains(rendered, "\n") {
		return []string{LabelValue(label, Short(rendered, 120), color)}
	}
	lines := []string{color.Apply(label+":", "cyan")}
	for _, line := range strings.Split(rendered, "\n") {
		lines = append(lines, "  "+line)
	}
	return lines
}

// StateDiffSummary renders the per-kind event counts. Ports state_diff_summary.
func StateDiffSummary(trace *model.Trace, color Color) string {
	counts := map[string]int{}
	for _, ev := range trace.EventsByID {
		switch ev.Kind {
		case model.KindCreate, model.KindExercise, model.KindArchive, model.KindAssign, model.KindUnassign:
			counts[ev.Kind]++
		default:
			counts[model.KindEvent]++
		}
	}
	parts := []string{
		color.Apply(fmt.Sprintf("+%d create", counts[model.KindCreate]), "green"),
		color.Apply(fmt.Sprintf(">%d exercise", counts[model.KindExercise]), "yellow"),
		color.Apply(fmt.Sprintf("x%d archive", counts[model.KindArchive]), "red"),
	}
	if counts[model.KindUnassign] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("<%d unassign", counts[model.KindUnassign]), "magenta"))
	}
	if counts[model.KindAssign] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf(">%d assign", counts[model.KindAssign]), "magenta"))
	}
	if counts[model.KindEvent] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("%d other", counts[model.KindEvent]), "blue"))
	}
	return strings.Join(parts, ", ")
}

// EventKindLabel is the uppercase marker shown in the tree.
func EventKindLabel(kind string) string {
	switch kind {
	case model.KindCreate:
		return "CREATE"
	case model.KindExercise:
		return "EXERCISE"
	case model.KindArchive:
		return "ARCHIVE"
	case model.KindAssign:
		return "ASSIGN"
	case model.KindUnassign:
		return "UNASSIGN"
	}
	return strings.ToUpper(kind)
}

// EventColor is the style name for an event kind.
func EventColor(kind string) string {
	switch kind {
	case model.KindCreate:
		return "green"
	case model.KindExercise:
		return "yellow"
	case model.KindArchive:
		return "red"
	case model.KindAssign, model.KindUnassign:
		return "magenta"
	}
	return "blue"
}

// TraceArtifactSummary renders the header shown when opening an artifact.
// Ports trace_artifact_summary.
func TraceArtifactSummary(artifact *model.Object) string {
	trace, _ := model.ObjectField(artifact, "trace")
	participant, _ := model.ObjectField(artifact, "participant")
	events, _ := model.ObjectField(trace, "eventsById")

	readAs := model.ObjectStrings(participant, "readAs")
	readAsText := "-"
	if len(readAs) > 0 {
		readAsText = strings.Join(readAs, ", ")
	}

	return strings.Join([]string{
		"Trace artifact",
		"  schema:       " + model.ObjectString(artifact, "schema"),
		"  update:       " + orDashValue(model.ObjectString(trace, "updateId")),
		"  kind:         " + orDashValue(model.ObjectString(artifact, "kind")),
		"  source:       " + orDashValue(model.ObjectString(trace, "source")),
		"  read-as:      " + readAsText,
		"  events:       " + strconv.Itoa(events.Len()),
	}, "\n")
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func orDashValue(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// PreparedArtifactSummary renders the header shown after a prepare.
// Ports prepared_artifact_summary. The "committed: no" line and the closing
// sentence are not decoration: prepared data must never read as committed.
func PreparedArtifactSummary(artifact map[string]any) string {
	request, _ := artifact["request"].(map[string]any)
	response, _ := artifact["response"].(*model.Object)

	commands, _ := request["commands"].([]any)
	lines := []string{
		"Prepared command",
		"  schema:       " + stringField(artifact, "schema"),
		"  endpoint:     " + orDash(stringField(artifact, "sourceUrl")),
		"  committed:    no",
		"  command id:   " + orDash(stringField(request, "commandId")),
		"  act-as:       " + orDash(joinField(request, "actAs")),
		"  read-as:      " + orDash(joinField(request, "readAs")),
		fmt.Sprintf("  commands:     %d", len(commands)),
	}
	if response != nil {
		if hash := model.ObjectString(response, "preparedTransactionHash"); hash != "" {
			lines = append(lines, "  prepared hash:"+Short(hash, 80))
		}
		// Presence is not enough: a null costEstimation means none was
		// returned, and printing the line then is misleading.
		if value, present := response.Get("costEstimation"); present && value != nil {
			lines = append(lines, "  cost:         returned")
		}
	}
	lines = append(lines, "", "This is prepared transaction data from a non-committing prepare call.")
	return strings.Join(lines, "\n")
}

func stringField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	value, ok := obj[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func joinField(obj map[string]any, key string) string {
	if obj == nil {
		return ""
	}
	values, _ := obj[key].([]string)
	return strings.Join(values, ", ")
}

// PrintSummary writes the header the visualizer opens with.
// Ports print_summary.
func PrintSummary(w io.Writer, trace *model.Trace) {
	fmt.Fprintf(w, "update:      %s\n", trace.UpdateID)
	fmt.Fprintf(w, "source:      %s (%s)\n", trace.Source, orDash(trace.SourceURL))
	fmt.Fprintf(w, "record time: %s\n", orDash(trace.RecordTime))
	fmt.Fprintf(w, "offset:      %s\n", orDash(trace.Offset))
	fmt.Fprintf(w, "synchronizer:%s\n", orDash(trace.SynchronizerID))
	fmt.Fprintf(w, "projection:  %s\n", trace.Projection.Note)
	if len(trace.Projection.ReadAs) > 0 {
		fmt.Fprintf(w, "read-as:     %s\n", strings.Join(trace.Projection.ReadAs, ", "))
	}
	fmt.Fprintf(w, "events:      %d\n", len(trace.EventsByID))
}

// DebugContextReport states what a trace does and does not contain.
//
// The "not present" half is the point: an artifact is a participant projection,
// and a reader must not mistake absence for evidence. Ports debug_context_report.
func DebugContextReport(trace *model.Trace) string {
	seen := map[string]bool{}
	var packageIDs []string
	for _, ev := range trace.EventsByID {
		if id := PackageFromTemplate(ev.Template); id != "" && !seen[id] {
			seen[id] = true
			packageIDs = append(packageIDs, id)
		}
	}
	sort.Strings(packageIDs)

	structure := "flat event list"
	if len(trace.RootEventIDs) > 0 {
		structure = "event order and parent/child links"
	}
	present := []string{
		"participant-visible transaction tree",
		structure,
		"choice arguments and create payloads where exposed",
		"party/witness labels where exposed",
	}
	if len(packageIDs) > 0 {
		shown := packageIDs
		if len(shown) > 5 {
			shown = shown[:5]
		}
		present = append(present, "package ids referenced by events: "+strings.Join(shown, ", "))
	}

	missing := []string{
		"source metadata unless provided by the local project or registry",
		"full original command envelope unless captured separately",
		"private subtransactions outside this projection",
		"operator logs unless attached separately",
	}

	var b strings.Builder
	b.WriteString("\nTrace context assessment\n------------------------\nPresent in this trace:\n")
	for i, item := range present {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- " + item)
	}
	b.WriteString("\n\nNot present in this trace artifact:\n")
	for i, item := range missing {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("- " + item)
	}
	return b.String()
}
