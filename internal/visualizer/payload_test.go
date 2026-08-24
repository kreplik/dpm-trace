package visualizer

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

func bigPayloadStepper(t *testing.T) (*Stepper, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	trace := load(t, "tests/fixtures/large-payload.trace.json")
	s := New(trace, render.Color{Enabled: false}, source.NewIndex(), &buf)
	for i, id := range s.Order {
		if id == "#5:3" {
			s.Index = i
		}
	}
	return s, &buf
}

// A forty-field contract printed seventy lines, so the event's own identity --
// template, parties, choice -- scrolled off before the reader saw it.
func TestLargePayloadIsBoundedByDefault(t *testing.T) {
	s, buf := bigPayloadStepper(t)
	s.ShowCurrent()

	out := buf.String()
	if lines := strings.Count(out, "\n"); lines > 25 {
		t.Errorf("event printed %d lines, want it bounded:\n%s", lines, out)
	}
	if !strings.Contains(out, "hidden") {
		t.Errorf("nothing says the value was cut:\n%s", out)
	}
	// The show-more affordance must name the command, or the reader has to
	// consult help to see the rest.
	if !strings.Contains(out, "`payload`") {
		t.Errorf("no explicit show-more:\n%s", out)
	}
	// Later fields are genuinely absent, not merely off-screen.
	if strings.Contains(out, "field40") {
		t.Errorf("the tail was not cut:\n%s", out)
	}
}

func TestPayloadExpandsAndCollapsesAgain(t *testing.T) {
	s, buf := bigPayloadStepper(t)

	s.Dispatch("payload")
	if !strings.Contains(buf.String(), "field40") {
		t.Errorf("expanding did not reveal the tail:\n%s", buf.String())
	}

	buf.Reset()
	s.Dispatch("payload")
	if strings.Contains(buf.String(), "field40") {
		t.Errorf("a second `payload` did not collapse again:\n%s", buf.String())
	}
}

// Field search is what makes a collapsed value usable: it answers "is this
// value in there" without printing the other fifty lines.
func TestPayloadSearchFindsAHiddenField(t *testing.T) {
	s, buf := bigPayloadStepper(t)
	s.Dispatch("payload field37")

	out := buf.String()
	if !strings.Contains(out, "field37") {
		t.Errorf("search did not find a hidden field:\n%s", out)
	}
	if strings.Contains(out, "field01") {
		t.Errorf("search printed non-matching fields:\n%s", out)
	}

	buf.Reset()
	s.Dispatch("payload definitely-not-present")
	if !strings.Contains(buf.String(), "no field") {
		t.Errorf("a search with no hits said nothing:\n%s", buf.String())
	}
}

// A small payload must not gain a "hidden" notice it does not need.
func TestSmallPayloadIsUntouched(t *testing.T) {
	s, buf := newStepper(t)
	s.ShowCurrent()
	if strings.Contains(buf.String(), "hidden") {
		t.Errorf("a small payload was truncated:\n%s", buf.String())
	}
}
