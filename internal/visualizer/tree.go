package visualizer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// A collapsed node hides its descendants behind a count. Real traces nest
// deeply -- an exercise that fetches, archives and re-creates buries the step a
// reader is looking for under a screen of context -- and the tree is only a
// navigation aid if it fits on a screen.
//
// Collapse is keyed by event id rather than by depth so it survives navigation:
// a reader who collapses a noisy subtree, steps away and comes back expects it
// to still be collapsed.

// ShowTree prints the trace with a cursor on the current step, honouring the
// collapsed set. `tree <depth>` collapses everything below the given depth,
// which is the fastest way to get an overview of a trace seen for the first
// time.
func (s *Stepper) ShowTree(arg string) {
	if depth, ok := parseDepth(arg); ok {
		s.collapseBelow(depth)
	} else if strings.TrimSpace(arg) != "" {
		fmt.Fprintln(s.out, s.Color.Apply("usage: tree [depth]", "yellow"))
		return
	}

	current := ""
	if len(s.Order) > 0 {
		current = s.Order[s.Index]
	}

	// prefix carries the ancestors' vertical bars; isLast picks the corner.
	// The connectors are render's, so the navigation tree and the printed
	// trace draw nesting the same way: depth conveyed by indentation alone
	// left a reader counting spaces to tell a sibling from a child.
	glyphs := render.TreeGlyphs()

	var visit func(string, string, bool)
	visit = func(eventID, prefix string, isLast bool) {
		ev, ok := s.Trace.EventsByID[eventID]
		if !ok {
			return
		}

		isCurrent := eventID == current
		cursor := "  "
		if isCurrent {
			cursor = s.Color.Apply("=>", "magenta", "bold")
		}

		label := render.EventTarget(ev)
		kind := fmt.Sprintf("%-8s", strings.ToUpper(ev.Kind))
		if isCurrent {
			kind = s.Color.Apply(kind, render.EventColor(ev.Kind), "bold")
			label = s.Color.Apply(label, "bold")
		}

		// A match marker earns its column only while a filter is set; without
		// one every node would be marked, which says nothing.
		match := ""
		if s.Active != nil {
			match = "  "
			if s.Active.Matches(ev) {
				match = s.Color.Apply("*", "cyan", "bold") + " "
			}
		}

		// The cursor and match markers live in a fixed left gutter, ahead of the
		// indentation. Placed after it they land in a different column at every
		// depth, so a reader cannot scan down for the matches -- which is the
		// whole point of marking them.
		connector := glyphs.Branch()
		childPrefix := glyphs.Vertical()
		if isLast {
			connector = glyphs.Last()
			childPrefix = glyphs.Blank()
		}
		fmt.Fprintf(s.out, "%s%s %s%s%s %s %s\n",
			match, cursor, prefix, connector,
			kind, ev.EventID, label)

		if s.isCollapsed(eventID) {
			if hidden := s.descendantCount(eventID); hidden > 0 {
				fmt.Fprintf(s.out, "%s%s%s\n",
					gutterPad(s.Active != nil),
					prefix+childPrefix,
					s.Color.Apply(fmt.Sprintf("... %s hidden (expand %s)",
						plural(hidden, "event"), eventID), "gray"))
			}
			return
		}
		for i, child := range ev.ChildEventIDs {
			visit(child, prefix+childPrefix, i == len(ev.ChildEventIDs)-1)
		}
	}

	shown := map[string]bool{}
	for i, root := range s.Trace.RootEventIDs {
		markShown(s.Trace, root, shown)
		visit(root, "", i == len(s.Trace.RootEventIDs)-1)
	}

	// The step view will happily show an event the roots cannot reach, and the
	// tree then draws no cursor at all -- two views disagreeing in silence.
	if current != "" && !shown[current] {
		fmt.Fprintln(s.out, s.Color.Apply(
			"  current step "+current+" is not reachable from the roots", "yellow"))
	}
}

func markShown(trace *model.Trace, eventID string, shown map[string]bool) {
	if shown[eventID] {
		return
	}
	ev, ok := trace.EventsByID[eventID]
	if !ok {
		return
	}
	shown[eventID] = true
	for _, child := range ev.ChildEventIDs {
		markShown(trace, child, shown)
	}
}

func (s *Stepper) isCollapsed(eventID string) bool {
	return s.Collapsed[eventID]
}

// Collapse hides a subtree. With no argument it acts on the current event, so
// the common case is one word.
func (s *Stepper) Collapse(arg string) {
	s.setCollapsed(arg, true)
}

// Expand reveals a subtree.
func (s *Stepper) Expand(arg string) {
	s.setCollapsed(arg, false)
}

func (s *Stepper) setCollapsed(arg string, collapsed bool) {
	done := "expanded"
	if collapsed {
		done = "collapsed"
	}
	arg = strings.TrimSpace(arg)

	if arg == "all" {
		s.setAll(collapsed)
		fmt.Fprintln(s.out, s.Color.Apply(done+" all", "yellow"))
		s.ShowTree("")
		return
	}

	if arg == "" {
		if len(s.Order) == 0 {
			fmt.Fprintln(s.out, s.Color.Apply("no events", "yellow"))
			return
		}
		arg = s.Order[s.Index]
	}

	eventID, ok := s.ResolveEvent(arg)
	if !ok {
		fmt.Fprintf(s.out, "%s\n", s.Color.Apply("no event "+arg, "yellow"))
		return
	}
	ev := s.Trace.EventsByID[eventID]
	if len(ev.ChildEventIDs) == 0 {
		// Silently succeeding would look like the command did nothing.
		fmt.Fprintf(s.out, "%s\n", s.Color.Apply(eventID+" has no children", "yellow"))
		return
	}

	if s.Collapsed == nil {
		s.Collapsed = map[string]bool{}
	}
	if collapsed {
		s.Collapsed[eventID] = true
	} else {
		delete(s.Collapsed, eventID)
	}
	// Say which event this was, because the argument is ambiguous: `tree 2`
	// means a depth while `collapse 2` means a step, and against a Canton
	// update -- where ids are bare integers -- a digit is a plausible id too.
	// Echoing what it resolved turns a silent wrong guess into a visible one.
	fmt.Fprintln(s.out, s.Color.Apply(done+" "+s.describeEvent(eventID), "yellow"))
	s.ShowTree("")
}

func (s *Stepper) setAll(collapsed bool) {
	if !collapsed {
		s.Collapsed = map[string]bool{}
		return
	}
	s.Collapsed = map[string]bool{}
	for id, ev := range s.Trace.EventsByID {
		if len(ev.ChildEventIDs) > 0 {
			s.Collapsed[id] = true
		}
	}
}

// collapseBelow collapses every node at or beyond depth, leaving the levels
// above it open. Depth 0 collapses the roots.
func (s *Stepper) collapseBelow(depth int) {
	s.Collapsed = map[string]bool{}
	var visit func(string, int)
	visit = func(eventID string, at int) {
		ev, ok := s.Trace.EventsByID[eventID]
		if !ok || len(ev.ChildEventIDs) == 0 {
			return
		}
		if at >= depth {
			s.Collapsed[eventID] = true
			return
		}
		for _, child := range ev.ChildEventIDs {
			visit(child, at+1)
		}
	}
	for _, root := range s.Trace.RootEventIDs {
		visit(root, 0)
	}
}

// descendantCount counts everything hidden below a node, not just its direct
// children: "3 hidden" must mean three lines, or the number misleads.
func (s *Stepper) descendantCount(eventID string) int {
	ev, ok := s.Trace.EventsByID[eventID]
	if !ok {
		return 0
	}
	total := 0
	for _, child := range ev.ChildEventIDs {
		total += 1 + s.descendantCount(child)
	}
	return total
}

// gutterPad is the blank width of the cursor/match gutter, so continuation
// lines start under the tree rather than under the markers.
func gutterPad(filtered bool) string {
	width := 3 // cursor plus its separating space
	if filtered {
		width += 2
	}
	return strings.Repeat(" ", width)
}

func parseDepth(arg string) (int, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, false
	}
	depth, err := strconv.Atoi(arg)
	if err != nil || depth < 0 {
		return 0, false
	}
	return depth, true
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// ResolveEvent turns what a reader typed into an event id.
//
// A ledger's event ids are not one shape: an update fetched from Canton uses
// bare integers ("0", "1"), while other sources use "#2:0". On top of that the
// stepper numbers steps from 1, so the number on screen next to an event is
// often not its id. Breakpoints have always accepted all three forms
// (Breakpoint.Matches), and collapse/expand accepting fewer meant the same
// event had a different name depending on which command you were typing.
//
// Accepted: the event id, the id with a leading "#", and the 1-based step
// number.
func (s *Stepper) ResolveEvent(spec string) (string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false
	}
	if _, ok := s.Trace.EventsByID[spec]; ok {
		return spec, true
	}
	if trimmed := strings.TrimPrefix(spec, "#"); trimmed != spec {
		if _, ok := s.Trace.EventsByID[trimmed]; ok {
			return trimmed, true
		}
	}
	// The "#" is punctuation a reader drops as often as types, so match with
	// it added as well as removed.
	if _, ok := s.Trace.EventsByID["#"+spec]; ok {
		return "#" + spec, true
	}
	for i, id := range s.Order {
		if spec == strconv.Itoa(i+1) {
			return id, true
		}
	}
	return "", false
}

// describeEvent names an event the way the reader can address it again: the id
// the tree prints, and the step number `j` takes when they differ.
func (s *Stepper) describeEvent(eventID string) string {
	for i, id := range s.Order {
		if id == eventID {
			return fmt.Sprintf("%s (step %d)", eventID, i+1)
		}
	}
	return eventID
}
