package visualizer

import (
	"fmt"
	"strconv"
	"strings"

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

// markerWidth keeps the +/- column aligned with nodes that have no children.
const markerWidth = 2

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

	var visit func(string, int)
	visit = func(eventID string, depth int) {
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
		fmt.Fprintf(s.out, "%s%s %s%s%s %s %s\n",
			match, cursor, strings.Repeat("  ", depth), s.marker(ev.EventID),
			kind, ev.EventID, label)

		if s.isCollapsed(eventID) {
			if hidden := s.descendantCount(eventID); hidden > 0 {
				fmt.Fprintf(s.out, "%s%s%s\n",
					gutterPad(s.Active != nil),
					strings.Repeat("  ", depth+1)+strings.Repeat(" ", markerWidth),
					s.Color.Apply(fmt.Sprintf("... %s hidden (expand %s)",
						plural(hidden, "event"), eventID), "gray"))
			}
			return
		}
		for _, child := range ev.ChildEventIDs {
			visit(child, depth+1)
		}
	}

	for _, root := range s.Trace.RootEventIDs {
		visit(root, 0)
	}
}

// marker flags a collapsed node. Only "+" is drawn: an expanded node is already
// evident from the children beneath it, so marking it too adds a column of
// noise to every line of the common, fully-expanded case.
func (s *Stepper) marker(eventID string) string {
	if s.isCollapsed(eventID) {
		if ev, ok := s.Trace.EventsByID[eventID]; ok && len(ev.ChildEventIDs) > 0 {
			return s.Color.Apply("+", "yellow", "bold") + " "
		}
	}
	return strings.Repeat(" ", markerWidth)
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

	eventID := arg
	if eventID == "" {
		if len(s.Order) == 0 {
			fmt.Fprintln(s.out, s.Color.Apply("no events", "yellow"))
			return
		}
		eventID = s.Order[s.Index]
	}

	ev, ok := s.Trace.EventsByID[eventID]
	if !ok {
		fmt.Fprintf(s.out, "%s\n", s.Color.Apply("no event "+eventID, "yellow"))
		return
	}
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
