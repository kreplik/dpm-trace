package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// A transaction tree carries no archived event: the Ledger API reports an
// archive as `consuming: true` on the exercise. Counting kinds alone therefore
// reported "x0 archive" for examples/archive.trace.json, which is a consuming
// Burn and archives exactly one contract.
func TestStateDiffSummaryCountsConsumingExercisesAsArchives(t *testing.T) {
	for _, tc := range []struct {
		artifact string
		want     string
	}{
		{"examples/archive.trace.json", "x1 archive"},
		// Split is consuming too: it archives the original and creates two.
		{"examples/exercise-child-create.trace.json", "x1 archive"},
		// A plain create archives nothing.
		{"examples/create.trace.json", "x0 archive"},
	} {
		trace := loadTrace(t, tc.artifact)
		got := StateDiffSummary(trace, Color{})
		if !strings.Contains(got, tc.want) {
			t.Errorf("%s = %q, want %q", tc.artifact, got, tc.want)
		}
	}
}

// The consuming exercise stays in the exercise count as well: it really is an
// exercise, and dropping it there would make the exercise example report none.
func TestStateDiffSummaryKeepsConsumingExercisesInTheExerciseCount(t *testing.T) {
	trace := loadTrace(t, "examples/exercise-child-create.trace.json")
	if got := StateDiffSummary(trace, Color{}); !strings.Contains(got, ">1 exercise") {
		t.Errorf("got %q, want >1 exercise", got)
	}
}

// Consuming is a *bool, so an absent flag must not read as an archive.
func TestIsConsumingExerciseRequiresAnExplicitTrue(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name string
		ev   *model.Event
		want bool
	}{
		{"consuming exercise", &model.Event{Kind: model.KindExercise, Consuming: &yes}, true},
		{"non-consuming exercise", &model.Event{Kind: model.KindExercise, Consuming: &no}, false},
		{"flag absent", &model.Event{Kind: model.KindExercise}, false},
		{"create with the flag set", &model.Event{Kind: model.KindCreate, Consuming: &yes}, false},
	} {
		if got := isConsumingExercise(tc.ev); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The trace view and the compare view must not disagree about one transaction.
func TestStateDiffCountsAgreesWithTheSummary(t *testing.T) {
	trace := loadTrace(t, "examples/archive.trace.json")
	if got := model.StateDiffCounts(trace)["archive"]; got != 1 {
		t.Errorf("StateDiffCounts archive = %d, want 1 to match the summary", got)
	}
}

// Box-drawing needs a UTF-8 console, so an override has to exist for the ones
// that cannot: mojibake is worse than the ASCII it replaced.
func TestTreeGlyphsHonourTheASCIIOverride(t *testing.T) {
	for _, on := range []string{"1", "true", "YES", " yes "} {
		t.Setenv("DPM_TRACE_ASCII", on)
		if TreeGlyphs() != asciiGlyphs {
			t.Errorf("DPM_TRACE_ASCII=%q did not select ASCII", on)
		}
	}
	for _, off := range []string{"0", "false", "no"} {
		t.Setenv("DPM_TRACE_ASCII", off)
		if TreeGlyphs() != unicodeGlyphs {
			t.Errorf("DPM_TRACE_ASCII=%q did not select unicode", off)
		}
	}
	// An unrecognised value must not silently mean "ascii".
	t.Setenv("DPM_TRACE_ASCII", "maybe")
	if TreeGlyphs() != unicodeGlyphs {
		t.Error("an unrecognised DPM_TRACE_ASCII value changed the default")
	}
}

// The ASCII tree must stay a glyph-for-glyph swap, not a second layout: same
// columns, same nesting, so only the characters differ.
func TestASCIITreeMatchesTheUnicodeLayout(t *testing.T) {
	trace := loadTrace(t, "examples/exercise-child-create.trace.json")

	var unicode bytes.Buffer
	t.Setenv("DPM_TRACE_ASCII", "0")
	PrettyTrace(&unicode, trace, Color{}, nil)

	var ascii bytes.Buffer
	t.Setenv("DPM_TRACE_ASCII", "1")
	PrettyTrace(&ascii, trace, Color{}, nil)

	swap := strings.NewReplacer("├── ", "|-- ", "└── ", "`-- ", "│   ", "|   ")
	if got := swap.Replace(unicode.String()); got != ascii.String() {
		t.Errorf("ascii output is not a glyph swap:\n%s", firstDifference(got, ascii.String()))
	}
}
