package visualizer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// Stepper is the --visualize REPL. Ports Stepper.
type Stepper struct {
	Trace       *model.Trace
	Color       render.Color
	SourceIndex *source.Index
	// Bundle is the artifact the trace came from, when opened from one; its ACS
	// snapshot supplies input-contract payloads.
	Bundle *model.Object

	Order       []string
	Index       int
	Breakpoints []Breakpoint

	// Active narrows navigation to matching steps. Order is left whole so step
	// numbers stay stable: a reader who filters, reads step 7, then clears the
	// filter expects step 7 to be the same event.
	Active *Filter

	// Collapsed holds the event ids whose descendants the tree hides.
	Collapsed map[string]bool

	// ExpandPayloads renders value blocks in full instead of bounded.
	ExpandPayloads bool

	out io.Writer
}

// New builds a Stepper over a trace.
func New(trace *model.Trace, color render.Color, index *source.Index, out io.Writer) *Stepper {
	s := &Stepper{Trace: trace, Color: color, SourceIndex: index, out: out}
	s.Order = s.preorder()
	return s
}

// preorder walks roots depth-first, then picks up any event not reachable from
// a root so nothing is hidden. Ports Stepper._preorder.
func (s *Stepper) preorder() []string {
	seen := map[string]bool{}
	var order []string

	var visit func(string)
	visit = func(eventID string) {
		if seen[eventID] {
			return
		}
		ev, ok := s.Trace.EventsByID[eventID]
		if !ok {
			return
		}
		seen[eventID] = true
		order = append(order, eventID)
		for _, child := range ev.ChildEventIDs {
			visit(child)
		}
	}

	for _, root := range s.Trace.RootEventIDs {
		visit(root)
	}
	// Orphans, in a stable order: Go map iteration is randomized where Python
	// dict iteration is insertion-ordered.
	var remaining []string
	for eventID := range s.Trace.EventsByID {
		if !seen[eventID] {
			remaining = append(remaining, eventID)
		}
	}
	// Numeric-aware, like the model's own ordering: sort.Strings puts "10"
	// before "2" and would renumber the steps of a partially-linked trace.
	model.SortEventIDs(remaining)
	for _, eventID := range remaining {
		visit(eventID)
	}
	return order
}

// Run drives the REPL until the user quits or input ends.
func (s *Stepper) Run(in io.Reader) {
	render.PrintSummary(s.out, s.Trace)
	fmt.Fprintln(s.out, "\n"+s.Color.Apply("Visualizer commands:", "bold")+
		" n/next, p/prev, j <n>, s/source, vars, b <spec>, c/continue, tree, context, json, q")
	fmt.Fprintln(s.out, s.Color.Apply("search:", "bold"),
		"filter <value>, find <value>, matches -- `help` for the full form")
	fmt.Fprintln(s.out, s.Color.Apply("tree:", "bold"),
		"tree [depth], collapse [id|all], expand [id|all]")
	fmt.Fprintln(s.out, s.Color.Apply("values:", "bold"),
		"payload to expand a truncated value, payload <text> to search this event's fields")
	fmt.Fprintln(s.out, s.Color.Apply("state:", "bold"),
		"diff -- the contracts this transaction created and archived")
	if s.SourceIndex.HasSources() {
		fmt.Fprintln(s.out, s.Color.Apply("source roots:", "cyan"), strings.Join(s.SourceIndex.Roots, ", "))
	}
	if len(s.Order) == 0 {
		fmt.Fprintln(s.out, "No events found.")
		return
	}
	s.ShowCurrent()

	// Ctrl-C exits the REPL, not the process: the Python stepper catches
	// KeyboardInterrupt, and killing the process would skip any cleanup the
	// caller has pending.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	go func() {
		<-interrupt
		fmt.Fprintln(s.out)
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(s.out, s.prompt())
		if !scanner.Scan() {
			fmt.Fprintln(s.out)
			return
		}
		if s.Dispatch(strings.TrimSpace(scanner.Text())) {
			return
		}
	}
}

// Dispatch runs one command, reporting whether the REPL should exit.
// Separated from Run so the command table is testable without a terminal.
func (s *Stepper) Dispatch(cmd string) (quit bool) {
	switch {
	case cmd == "q" || cmd == "quit" || cmd == "exit":
		return true
	case cmd == "" || cmd == "n" || cmd == "next":
		s.step(1)
		s.ShowCurrent()
	case cmd == "p" || cmd == "prev":
		s.step(-1)
		s.ShowCurrent()
	case strings.HasPrefix(cmd, "j "):
		s.Jump(cmd)
	case cmd == "s" || cmd == "src" || cmd == "source":
		s.ShowSource()
	case cmd == "vars" || cmd == "locals":
		s.ShowVariables()
	case strings.HasPrefix(cmd, "b "):
		s.AddBreakpoint(cmd)
	case cmd == "bp" || cmd == "breakpoints":
		s.ListBreakpoints()
	case strings.HasPrefix(cmd, "clear"):
		s.ClearBreakpoints(cmd)
	case cmd == "c" || cmd == "continue":
		s.ContinueToBreakpoint()
	case cmd == "tree" || strings.HasPrefix(cmd, "tree "):
		s.ShowTree(strings.TrimPrefix(cmd, "tree"))
	case cmd == "collapse" || strings.HasPrefix(cmd, "collapse "):
		s.Collapse(strings.TrimPrefix(cmd, "collapse"))
	case cmd == "expand" || strings.HasPrefix(cmd, "expand "):
		s.Expand(strings.TrimPrefix(cmd, "expand"))
	case cmd == "context":
		fmt.Fprintln(s.out, render.DebugContextReport(s.Trace, s.SourceIndex))
	case cmd == "json":
		s.ShowJSON()
	case cmd == "filter" || strings.HasPrefix(cmd, "filter "):
		s.SetFilter(strings.TrimPrefix(cmd, "filter"))
	case cmd == "find" || strings.HasPrefix(cmd, "find "):
		s.Find(strings.TrimPrefix(cmd, "find"))
	case cmd == "matches":
		s.ShowMatches()
	case cmd == "diff" || cmd == "state":
		s.ShowStateDiff()
	case cmd == "payload" || strings.HasPrefix(cmd, "payload "):
		s.SearchPayload(strings.TrimPrefix(cmd, "payload"))
	case cmd == "help":
		fmt.Fprintln(s.out, "n/next, p/prev, j <index>, s/source, vars, b <spec>, bp, clear [n], c/continue, tree, context, json, q")
		fmt.Fprintln(s.out, "  filter ["+strings.Join(filterFields, "|")+"] <value>   narrow navigation")
		fmt.Fprintln(s.out, "  filter                                                            clear the filter")
		fmt.Fprintln(s.out, "  find <value>                                                      jump to the next match")
		fmt.Fprintln(s.out, "  matches                                                           list what the filter selects")
		fmt.Fprintln(s.out, "  diff/state                                                        contracts this transaction created and archived")
		fmt.Fprintln(s.out, "  payload [text]                                                    expand values, or search this event's fields")
		fmt.Fprintln(s.out, "  tree [depth]                                                      collapse below a depth")
		fmt.Fprintln(s.out, "  collapse|expand [event-id|all]                                    hide or reveal a subtree")
		fmt.Fprintln(s.out, "  source-linked replay is best-effort: unsupported expressions are shown as (not evaluated)")
	default:
		fmt.Fprintln(s.out, "unknown command; try `help`")
	}
	return false
}

// Current returns the event under the cursor.
func (s *Stepper) Current() *model.Event {
	return s.Trace.EventsByID[s.Order[s.Index]]
}

// ShowCurrent prints the event under the cursor. Ports Stepper.show_current.
func (s *Stepper) ShowCurrent() {
	ctx := render.NewContext(s.Trace)
	ev := s.Current()
	color := s.Color

	fmt.Fprintln(s.out, "\n"+color.Apply(strings.Repeat("-", 72), "gray"))
	fmt.Fprintln(s.out, color.Apply(fmt.Sprintf("Step %d/%d", s.Index+1, len(s.Order)), "bold"),
		color.Apply(strings.ToUpper(ev.Kind), render.EventColor(ev.Kind), "bold"),
		color.Apply(ev.EventID, "gray"))
	fmt.Fprintln(s.out, render.LabelValue("template", orDash(ev.Template), color))
	if loc := s.location(ev); loc != nil {
		fmt.Fprintln(s.out, render.LabelValue("source",
			fmt.Sprintf("%s  (%s)", source.FormatPath(*loc), loc.Label), color))
	}
	fmt.Fprintln(s.out, render.LabelValue("contract", render.Short(ev.ContractID, 32), color))
	if ev.Choice != "" {
		fmt.Fprintln(s.out, render.LabelValue("choice",
			fmt.Sprintf("%s  consuming=%s", ev.Choice, boolText(ev.Consuming)), color))
	}
	for _, line := range render.ReassignmentDetailLines(ev, color, ctx) {
		fmt.Fprintln(s.out, line)
	}
	if len(ev.ActingParties) > 0 {
		fmt.Fprintln(s.out, render.LabelValue("actors", ctx.Parties(ev.ActingParties), color))
	}
	if len(ev.Witnesses) > 0 {
		fmt.Fprintln(s.out, render.LabelValue("witness", ctx.Parties(ev.Witnesses), color))
	}
	if len(ev.Signatories) > 0 || len(ev.Observers) > 0 {
		fmt.Fprintln(s.out, render.LabelValue("stakeholders",
			fmt.Sprintf("signatories=%s observers=%s",
				pythonList(ctx, ev.Signatories), pythonList(ctx, ev.Observers)), color))
	}
	for _, block := range eventBlocks(ev) {
		if block.value == nil {
			continue
		}
		s.writeBlock(block, ctx)
	}
	if len(ev.ChildEventIDs) > 0 {
		fmt.Fprintln(s.out, render.LabelValue("children", strings.Join(ev.ChildEventIDs, ", "), color))
	}
}

// ShowSource prints the source snippet for the current event.
func (s *Stepper) ShowSource() {
	loc := s.location(s.Current())
	if loc == nil {
		fmt.Fprintln(s.out, s.Color.Apply(
			"no source location available for this event; provide matching source metadata", "yellow"))
		return
	}
	fmt.Fprintln(s.out, s.SourceIndex.Snippet(*loc, 2))
}

// ShowVariables prints the event fields the replay would bind.
// Ports Stepper.show_variables.
func (s *Stepper) ShowVariables() {
	ctx := render.NewContext(s.Trace)
	ev := s.Current()
	fmt.Fprintln(s.out, s.Color.Apply("variables", "bold"))

	vars, keys := s.StepVariables(ev, ctx)
	if len(keys) == 0 {
		fmt.Fprintln(s.out, "  -")
		return
	}
	for _, key := range keys {
		rendered := render.RenderPrettyValue(vars[key], ctx)
		if strings.Contains(rendered, "\n") {
			fmt.Fprintf(s.out, "  %s\n", s.Color.Apply(key+":", "cyan"))
			// The same bound as the step view. Without it `vars` printed
			// fifty lines for an event the step view had just shown in
			// seventeen, which made the bounding look broken rather than
			// deliberate.
			s.writeBounded(render.IndentText(rendered))
			continue
		}
		fmt.Fprintf(s.out, "  %s %s\n", s.Color.Apply(key+":", "cyan"), rendered)
	}
}

// StepVariables binds the event fields present at this step, then applies party
// aliases. Ports Stepper.step_variables: fields are included whenever present,
// not by event kind.
//
// The key order is returned alongside, because the display iterates in
// insertion order and a Go map would randomize it.
func (s *Stepper) StepVariables(ev *model.Event, ctx *render.Context) (map[string]any, []string) {
	vars := map[string]any{}
	var order []string
	set := func(key string, value any) {
		vars[key] = ctx.RenderValue(value)
		order = append(order, key)
	}

	set("eventId", ev.EventID)
	set("kind", ev.Kind)
	if ev.Template != "" {
		set("template", ev.Template)
	}
	if ev.ContractID != "" {
		// Shortened, as the tree and the step view show it: a full id is 130
		// characters and wraps, and the reader who wants it whole has `json`.
		set("contractId", render.Short(ev.ContractID, 66))
	}
	if ev.Choice != "" {
		set("choice", ev.Choice)
	}
	if ev.Consuming != nil && ev.Kind == model.KindExercise {
		set("consuming", *ev.Consuming)
	}
	if len(ev.ActingParties) > 0 {
		set("actors", ev.ActingParties)
	}
	if len(ev.Witnesses) > 0 {
		set("witnesses", ev.Witnesses)
	}
	if len(ev.Signatories) > 0 {
		set("signatories", ev.Signatories)
	}
	if ev.Payload != nil {
		set("createPayload", render.SimplifyLFValue(ev.Payload))
	}
	if ev.Argument != nil {
		set("choiceArgument", render.SimplifyLFValue(ev.Argument))
	}
	if ev.Result != nil {
		set("choiceResult", render.SimplifyLFValue(ev.Result))
	}
	if len(ev.Observers) > 0 {
		set("observers", ev.Observers)
	}
	if len(ev.ChildEventIDs) > 0 {
		set("children", strings.Join(ev.ChildEventIDs, ", "))
	}
	if loc := s.location(ev); loc != nil {
		set("source", fmt.Sprintf("%s:%d", filepath.Base(loc.Path), loc.Line))
	}
	if payload := s.inputContractPayload(ev.ContractID); payload != nil {
		set("inputContract", payload)
	}
	return vars, order
}

// inputContractPayload looks up a consumed contract's payload in the artifact's
// ACS snapshot, when one was exported. Ports input_contract_payload.
func (s *Stepper) inputContractPayload(contractID string) any {
	if s.Bundle == nil || contractID == "" {
		return nil
	}
	acs, ok := model.ObjectField(s.Bundle, "acsSnapshot")
	if !ok {
		return nil
	}
	response, ok := acs.Get("response")
	if !ok {
		return nil
	}
	return findContractPayload(response, contractID)
}

// findContractPayload walks a decoded ACS response for a contract's arguments.
func findContractPayload(value any, contractID string) any {
	switch v := value.(type) {
	case *model.Object:
		if model.ObjectString(v, "contractId") == contractID ||
			model.ObjectString(v, "contract_id") == contractID {
			for _, key := range []string{"createArguments", "createArgument", "create_arguments", "payload"} {
				if payload, ok := v.Get(key); ok && payload != nil {
					return payload
				}
			}
		}
		for _, key := range v.Keys() {
			item, _ := v.Get(key)
			if found := findContractPayload(item, contractID); found != nil {
				return found
			}
		}
	case []any:
		for _, item := range v {
			if found := findContractPayload(item, contractID); found != nil {
				return found
			}
		}
	}
	return nil
}

// AddBreakpoint registers a breakpoint spec.
func (s *Stepper) AddBreakpoint(cmd string) {
	spec := strings.TrimSpace(strings.TrimPrefix(cmd, "b "))
	if spec == "" {
		fmt.Fprintln(s.out, s.Color.Apply("usage: b <event-id|template.choice|file:line>", "yellow"))
		return
	}
	s.Breakpoints = append(s.Breakpoints, Breakpoint{Spec: spec})
	fmt.Fprintf(s.out, "%s %d set: %s\n", s.Color.Apply("breakpoint", "magenta", "bold"), len(s.Breakpoints), spec)
}

// ListBreakpoints prints the registered breakpoints.
func (s *Stepper) ListBreakpoints() {
	if len(s.Breakpoints) == 0 {
		fmt.Fprintln(s.out, s.Color.Apply("no breakpoints", "yellow"))
		return
	}
	for i, bp := range s.Breakpoints {
		fmt.Fprintf(s.out, "%s %s\n", s.Color.Apply(strconv.Itoa(i+1)+":", "gray"), bp.Spec)
	}
}

// ClearBreakpoints removes one breakpoint or all of them.
func (s *Stepper) ClearBreakpoints(cmd string) {
	// partition(" "), not TrimPrefix: "clearX" has no argument and clears
	// everything, where trimming the prefix would leave "X" and print usage.
	_, arg, _ := strings.Cut(cmd, " ")
	arg = strings.TrimSpace(arg)
	if arg == "" {
		s.Breakpoints = nil
		fmt.Fprintln(s.out, s.Color.Apply("cleared all breakpoints", "yellow"))
		return
	}
	index, err := strconv.Atoi(arg)
	if err != nil {
		fmt.Fprintln(s.out, s.Color.Apply("usage: clear [breakpoint-number]", "yellow"))
		return
	}
	if index < 1 || index > len(s.Breakpoints) {
		fmt.Fprintln(s.out, s.Color.Apply(
			fmt.Sprintf("breakpoint must be between 1 and %d", len(s.Breakpoints)), "yellow"))
		return
	}
	removed := s.Breakpoints[index-1]
	s.Breakpoints = append(s.Breakpoints[:index-1], s.Breakpoints[index:]...)
	fmt.Fprintln(s.out, s.Color.Apply("cleared breakpoint:", "yellow"), removed.Spec)
}

// ContinueToBreakpoint advances to the next matching step, or to the end.
func (s *Stepper) ContinueToBreakpoint() {
	if len(s.Breakpoints) == 0 {
		fmt.Fprintln(s.out, s.Color.Apply("no breakpoints set", "yellow"))
		return
	}
	if len(s.Order) == 0 {
		fmt.Fprintln(s.out, s.Color.Apply("no events", "yellow"))
		return
	}
	for step := s.Index + 1; step < len(s.Order); step++ {
		eventID := s.Order[step]
		ev := s.Trace.EventsByID[eventID]
		loc := s.location(ev)
		for _, bp := range s.Breakpoints {
			if bp.Matches(step, eventID, ev, loc) {
				s.Index = step
				s.ShowCurrent()
				return
			}
		}
	}
	fmt.Fprintln(s.out, s.Color.Apply("no later breakpoint hit", "yellow"))
}

// ShowJSON prints the current event in the artifact encoding.
func (s *Stepper) ShowJSON() {
	encoded, err := model.Encode(model.EventToJSON(s.Current()))
	if err != nil {
		fmt.Fprintf(s.out, "error: %v\n", err)
		return
	}
	fmt.Fprintln(s.out, string(encoded))
}

// Jump moves the cursor to a 1-based step number.
func (s *Stepper) Jump(cmd string) {
	_, value, _ := strings.Cut(cmd, " ")
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		fmt.Fprintln(s.out, "usage: j <step-number>")
		return
	}
	if index < 1 || index > len(s.Order) {
		fmt.Fprintf(s.out, "step must be between 1 and %d\n", len(s.Order))
		return
	}
	s.Index = index - 1
	s.ShowCurrent()
}

func (s *Stepper) location(ev *model.Event) *source.Location {
	if s.SourceIndex == nil {
		return nil
	}
	return s.SourceIndex.LocationForEvent(ev)
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// boolText renders a tri-state consuming flag the way Python prints it.
func boolText(value *bool) string {
	if value == nil {
		return "None"
	}
	if *value {
		return "True"
	}
	return "False"
}

// pythonList renders aliased parties as a Python list literal, matching the
// stakeholders line.
func pythonList(ctx *render.Context, parties []string) string {
	if len(parties) == 0 {
		return "[]"
	}
	quoted := make([]string, 0, len(parties))
	for _, party := range parties {
		quoted = append(quoted, "'"+ctx.Party(party)+"'")
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// step moves by one position, skipping events the active filter excludes and
// stopping at the ends rather than wrapping.
func (s *Stepper) step(delta int) {
	for i := s.Index + delta; i >= 0 && i < len(s.Order); i += delta {
		if s.matchesActive(i) {
			s.Index = i
			return
		}
	}
	// No further match: stay put rather than jumping somewhere unrelated.
	if s.Active != nil {
		fmt.Fprintln(s.out, s.Color.Apply("no further match for filter "+s.Active.Describe(), "yellow"))
	}
}

func (s *Stepper) matchesActive(i int) bool {
	if s.Active == nil {
		return true
	}
	ev, ok := s.Trace.EventsByID[s.Order[i]]
	return ok && s.Active.Matches(ev)
}

// SetFilter applies `filter ...`, or clears it when given no argument.
func (s *Stepper) SetFilter(arg string) {
	if strings.TrimSpace(arg) == "" {
		if s.Active == nil {
			fmt.Fprintln(s.out, s.Color.Apply("no filter set", "yellow"))
			return
		}
		s.Active = nil
		fmt.Fprintln(s.out, s.Color.Apply("filter cleared", "yellow"))
		return
	}

	filter, err := ParseFilter(arg)
	if err != nil {
		fmt.Fprintln(s.out, s.Color.Apply(err.Error(), "yellow"))
		return
	}
	filter.Ctx = render.NewContext(s.Trace)

	matches := s.matchingSteps(filter)
	if len(matches) == 0 {
		// Not applied: a filter that selects nothing would strand navigation
		// with no way to see why.
		fmt.Fprintln(s.out, s.Color.Apply("no events match "+filter.Describe()+"; filter not applied", "yellow"))
		return
	}

	s.Active = &filter
	fmt.Fprintf(s.out, "%s %s\n",
		s.Color.Apply("filter:", "cyan"),
		fmt.Sprintf("%s  (%d of %d steps)", filter.Describe(), len(matches), len(s.Order)))

	// Land on a matching step so the view agrees with the filter.
	if !s.matchesActive(s.Index) {
		s.Index = matches[0]
	}
	s.ShowCurrent()
}

// Find jumps to the next matching step without changing the active filter.
func (s *Stepper) Find(arg string) {
	if strings.TrimSpace(arg) == "" {
		fmt.Fprintln(s.out, s.Color.Apply(
			"usage: find ["+strings.Join(filterFields, "|")+"] <value>", "yellow"))
		return
	}
	filter, err := ParseFilter(arg)
	if err != nil {
		fmt.Fprintln(s.out, s.Color.Apply(err.Error(), "yellow"))
		return
	}
	filter.Ctx = render.NewContext(s.Trace)
	for i := s.Index + 1; i < len(s.Order); i++ {
		if ev, ok := s.Trace.EventsByID[s.Order[i]]; ok && filter.Matches(ev) {
			s.Index = i
			s.ShowCurrent()
			return
		}
	}
	fmt.Fprintln(s.out, s.Color.Apply("no match after this step for "+filter.Describe(), "yellow"))
}

// ShowMatches lists the steps the active filter selects, so a reader can see
// the shape of the result before walking it.
func (s *Stepper) ShowMatches() {
	if s.Active == nil {
		fmt.Fprintln(s.out, s.Color.Apply("no filter set; use `filter <value>`", "yellow"))
		return
	}
	matches := s.matchingSteps(*s.Active)
	fmt.Fprintf(s.out, "%s %s\n", s.Color.Apply("matches:", "cyan"), s.Active.Describe())
	width := 0
	for _, i := range matches {
		if id := s.Order[i]; len(id) > width {
			width = len(id)
		}
	}
	for _, i := range matches {
		ev := s.Trace.EventsByID[s.Order[i]]
		marker := " "
		if i == s.Index {
			marker = ">"
		}
		// Both numbers, because they are different numbers and each addresses
		// a different command: [step] is what `j` takes, the event id is what
		// `collapse` and the tree show. Printing only the step left a reader
		// unable to tell which tree line a match referred to.
		//
		// The kind carries as much signal as the template here: a filter on a
		// party typically selects a mix of creates and exercises.
		fmt.Fprintf(s.out, " %s %s %s %s %s\n",
			marker,
			s.Color.Apply(fmt.Sprintf("[%d]", i+1), "gray"),
			s.Color.Apply(fmt.Sprintf("%-*s", width, ev.EventID), "gray"),
			s.Color.Apply(render.EventKindLabel(ev.Kind), render.EventColor(ev.Kind)),
			render.EventTarget(ev))
	}
}

func (s *Stepper) matchingSteps(filter Filter) []int {
	var matches []int
	for i, id := range s.Order {
		if ev, ok := s.Trace.EventsByID[id]; ok && filter.Matches(ev) {
			matches = append(matches, i)
		}
	}
	return matches
}

// prompt names the parties the session is reading as.
//
// The projection is stated in the header, but the header scrolls away: a
// reader ten steps in is looking at contract payloads with no reminder that
// this is one participant's view and that absence is not evidence. Naming the
// parties in the prompt keeps it in front of them for the whole session, which
// is what "participant/projection labels shown in the visualizer" is for.
//
// It also doubles as a filter indicator, so the two pieces of state that
// change what is on screen are visible in the same place.
func (s *Stepper) prompt() string {
	label := "dpm-trace"
	if parties := s.Trace.Projection.ReadAs; len(parties) > 0 {
		ctx := render.NewContext(s.Trace)
		short := make([]string, 0, len(parties))
		for _, party := range parties {
			short = append(short, ctx.Party(party))
		}
		label += "[" + strings.Join(short, ",") + "]"
	}
	if s.Active != nil {
		label += " filter:" + s.Active.Describe()
	}
	return label + "> "
}
