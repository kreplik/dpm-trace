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
