package visualizer

import (
	"fmt"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// Large payloads are the reason a step can be unreadable. A contract with forty
// fields prints seventy lines, so the event's identity -- template, parties,
// choice -- scrolls off the top before the reader has seen it, and stepping
// through a transaction becomes a fight with the scrollback.
//
// The answer is not to render less: the value is what the reader came for. It
// is to render it in a bounded way by default, say plainly how much is hidden,
// and give one command to see the rest and one to search it.

// payloadPreviewLines is how much of a value block is shown before it is cut.
// Chosen so an event with a large payload still fits beside its own header on
// an ordinary terminal.
const payloadPreviewLines = 8

// payloadBlock is one labelled value on an event.
type payloadBlock struct {
	label string
	value any
}

func eventBlocks(ev *model.Event) []payloadBlock {
	return []payloadBlock{
		{"payload", ev.Payload},
		{"choice argument", ev.Argument},
		{"choice result", ev.Result},
	}
}

// writeBlock renders one value, collapsed unless the reader asked otherwise.
func (s *Stepper) writeBlock(block payloadBlock, ctx *render.Context) {
	fmt.Fprintln(s.out, s.Color.Apply(block.label+":", "cyan"))
	text := render.IndentText(render.RenderPrettyValue(block.value, ctx))
	lines := strings.Split(text, "\n")

	if s.ExpandPayloads || len(lines) <= payloadPreviewLines {
		fmt.Fprintln(s.out, text)
		return
	}

	fmt.Fprintln(s.out, strings.Join(lines[:payloadPreviewLines], "\n"))
	hidden := len(lines) - payloadPreviewLines
	// Naming the command in the message is the "explicit show-more": a reader
	// should not have to consult help to find out how to see the rest.
	fmt.Fprintln(s.out, s.Color.Apply(
		fmt.Sprintf("  ... %s hidden (`payload` to expand, `payload <text>` to search)",
			plural(hidden, "line")), "gray"))
}

// TogglePayloads switches between the bounded and the full rendering.
func (s *Stepper) TogglePayloads() {
	s.ExpandPayloads = !s.ExpandPayloads
	state := "collapsed"
	if s.ExpandPayloads {
		state = "expanded"
	}
	fmt.Fprintln(s.out, s.Color.Apply("payloads "+state, "yellow"))
	s.ShowCurrent()
}

// SearchPayload prints the fields of the current event's values that match.
//
// This is the field search the reader needs when the collapsed view hides what
// they are looking for: `filter` decides which events to visit, this decides
// which lines of one event to read. Matching is on the rendered text, so what
// is typed matches what was on screen.
func (s *Stepper) SearchPayload(needle string) {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		s.TogglePayloads()
		return
	}

	ev := s.Current()
	if ev == nil {
		fmt.Fprintln(s.out, s.Color.Apply("no event", "yellow"))
		return
	}
	ctx := render.NewContext(s.Trace)
	lowered := strings.ToLower(needle)

	found := 0
	for _, block := range eventBlocks(ev) {
		if block.value == nil {
			continue
		}
		var matched []string
		for _, line := range strings.Split(render.RenderPrettyValue(block.value, ctx), "\n") {
			if strings.Contains(strings.ToLower(line), lowered) {
				matched = append(matched, line)
			}
		}
		if len(matched) == 0 {
			continue
		}
		found += len(matched)
		fmt.Fprintln(s.out, s.Color.Apply(block.label+":", "cyan"))
		for _, line := range matched {
			fmt.Fprintln(s.out, render.IndentText(line))
		}
	}

	if found == 0 {
		fmt.Fprintf(s.out, "%s\n", s.Color.Apply(
			"no field of this event matches "+needle, "yellow"))
	}
}

// writeBounded prints an already-rendered block under the same limit as an
// event's value blocks, so `vars` and the step view agree about how much of a
// large value is shown.
func (s *Stepper) writeBounded(text string) {
	lines := strings.Split(text, "\n")
	if s.ExpandPayloads || len(lines) <= payloadPreviewLines {
		fmt.Fprintln(s.out, text)
		return
	}
	fmt.Fprintln(s.out, strings.Join(lines[:payloadPreviewLines], "\n"))
	fmt.Fprintln(s.out, s.Color.Apply(
		fmt.Sprintf("  ... %s hidden (`payload` to expand, `payload <text>` to search)",
			plural(len(lines)-payloadPreviewLines, "line")), "gray"))
}
