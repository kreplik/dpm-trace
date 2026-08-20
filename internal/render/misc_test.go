package render

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

func loadTrace(t *testing.T, rel string) *model.Trace {
	t.Helper()
	artifact, err := model.LoadTraceArtifact(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := model.TraceFromArtifact(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return trace
}

// --explain-apis is documentation the tool prints; it must name both APIs and
// the endpoints it actually calls, or it is worse than nothing.
func TestExplainAPIs(t *testing.T) {
	got := ExplainAPIs("/v0/scan/update", "/v2/updates/update-by-id")
	for _, want := range []string{"Scan API", "Ledger API", "/v0/scan/update", "/v2/updates/update-by-id"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// The visualizer header; it must carry the projection caveat.
func TestPrintSummary(t *testing.T) {
	var buf bytes.Buffer
	PrintSummary(&buf, loadTrace(t, "examples/create.trace.json"))
	out := buf.String()
	for _, want := range []string{"update:", "source:", "offset:", "projection:", "events:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// `context` in the visualizer: which packages appear and what metadata is
// available for them.
func TestDebugContextReport(t *testing.T) {
	got := DebugContextReport(loadTrace(t, "examples/create.trace.json"))
	if got == "" {
		t.Fatal("empty report")
	}
	if !strings.Contains(got, "b9e9e043") {
		t.Errorf("package id missing from:\n%s", got)
	}
}

func TestPackageFromTemplate(t *testing.T) {
	if got := PackageFromTemplate("pkg123:Asset:Asset"); got != "pkg123" {
		t.Errorf("got %q", got)
	}
	for _, none := range []string{"", "Asset:Asset", "bare"} {
		if got := PackageFromTemplate(none); got != "" {
			t.Errorf("PackageFromTemplate(%q) = %q, want empty", none, got)
		}
	}
}

// Party lists are rendered through the alias context so long ids stay readable.
func TestContextParties(t *testing.T) {
	trace := loadTrace(t, "examples/create.trace.json")
	ctx := NewContext(trace)
	got := ctx.Parties([]string{"Issuer::122036f58f09b1879fdc99a950478166fd73076d1aab38de51f9aec4282dc17213a4"})
	if got == "" {
		t.Fatal("rendered nothing")
	}
	if strings.Contains(got, "122036f58f09b1879fdc99a950478166fd73076d1aab38de51f9aec4282dc17213a4") {
		t.Errorf("full party id leaked into %q", got)
	}
	// A plain join: callers decide whether an empty list needs a placeholder.
	if got := ctx.Parties(nil); got != "" {
		t.Errorf("empty list = %q, want empty", got)
	}
}

func TestColorFromMode(t *testing.T) {
	if ColorFromMode("always", false).Enabled != true {
		t.Error("always must enable colour even without a tty")
	}
	if ColorFromMode("never", true).Enabled != false {
		t.Error("never must disable colour even with a tty")
	}
	if ColorFromMode("auto", true).Enabled != true {
		t.Error("auto must follow the tty")
	}
	if ColorFromMode("auto", false).Enabled != false {
		t.Error("auto must follow the tty")
	}
}

func TestEqualStringSlices(t *testing.T) {
	if !equalStringSlices(nil, nil) || !equalStringSlices([]string{"a"}, []string{"a"}) {
		t.Error("equal slices reported different")
	}
	if equalStringSlices([]string{"a"}, []string{"b"}) || equalStringSlices([]string{"a"}, nil) {
		t.Error("different slices reported equal")
	}
}

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
