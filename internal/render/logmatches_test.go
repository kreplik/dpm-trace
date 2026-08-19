package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

func decodeValue(t *testing.T, raw string) any {
	t.Helper()
	value, err := model.DecodeValue([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// Log matches correlate a failed submission with operator log lines. The list
// is capped so a noisy log does not bury the failure itself.
func TestPrintLogMatches(t *testing.T) {
	var buf bytes.Buffer
	printLogMatches(&buf, decodeValue(t, `[
	  {"file":"/var/log/canton.log","line":"120","text":"rejected cmd-1"},
	  {"file":"/var/log/other.log","error":"permission denied"}
	]`), plain())

	out := buf.String()
	if !strings.Contains(out, "Log matches") {
		t.Fatalf("no header:\n%s", out)
	}
	if !strings.Contains(out, "canton.log:120: rejected cmd-1") {
		t.Errorf("match not rendered:\n%s", out)
	}
	// A file that could not be read reports the error instead of a fake line.
	if !strings.Contains(out, "other.log: permission denied") {
		t.Errorf("read error not rendered:\n%s", out)
	}
}

func TestPrintLogMatchesCapsTheList(t *testing.T) {
	var entries []string
	for i := 0; i < 12; i++ {
		entries = append(entries, `{"file":"a.log","line":"1","text":"line"}`)
	}
	var buf bytes.Buffer
	printLogMatches(&buf, decodeValue(t, "["+strings.Join(entries, ",")+"]"), plain())

	if got := strings.Count(buf.String(), "a.log"); got != 8 {
		t.Errorf("rendered %d matches, want the list capped at 8", got)
	}
	if !strings.Contains(buf.String(), "4 more") {
		t.Errorf("no overflow notice:\n%s", buf.String())
	}
}

// Nothing to show must print nothing at all -- not an empty header.
func TestPrintLogMatchesSilentWhenEmpty(t *testing.T) {
	for _, raw := range []any{nil, "not a list", decodeValue(t, `[]`)} {
		var buf bytes.Buffer
		printLogMatches(&buf, raw, plain())
		if buf.Len() != 0 {
			t.Errorf("printed %q for %#v", buf.String(), raw)
		}
	}
}

// Colour is opt-in; when disabled the text must be untouched so output can be
// diffed and piped.
func TestColorApply(t *testing.T) {
	off := Color{Enabled: false}
	if got := off.Apply("text", "red", "bold"); got != "text" {
		t.Errorf("disabled colour changed the text: %q", got)
	}

	on := Color{Enabled: true}
	if got := on.Apply("text"); got != "text" {
		t.Errorf("no styles should leave the text alone: %q", got)
	}
	styled := on.Apply("text", "red")
	if !strings.Contains(styled, "text") || styled == "text" {
		t.Errorf("styled = %q, want escape codes around the text", styled)
	}
	// An unknown style is ignored rather than emitting a broken sequence.
	if got := on.Apply("text", "chartreuse"); !strings.Contains(got, "text") {
		t.Errorf("unknown style = %q", got)
	}
}
