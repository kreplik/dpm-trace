package render

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/source"
)

func TestCompletionTraceMatchesGolden(t *testing.T) {
	root := repoRoot(t)
	completion, err := model.LoadCompletion(filepath.Join(root, "tests/fixtures/failed-with-source.completion.json"))
	if err != nil {
		t.Fatalf("load completion: %v", err)
	}
	var buf bytes.Buffer
	CompletionTrace(&buf, completion, Color{Enabled: false}, nil, 5)
	got := strings.TrimRight(buf.String(), "\n")
	want := goldenStdout(t, filepath.Join(root, "tests/golden/completion-plain.txt"))
	if got != want {
		t.Errorf("differs from the golden:\n%s", firstDifference(got, want))
	}
}

func TestPreparedCompletionComparisonMatchesGoldens(t *testing.T) {
	root := repoRoot(t)
	prepared, err := model.LoadPreparedArtifact(filepath.Join(root, "tests/fixtures/compare/prepared.json"))
	if err != nil {
		t.Fatalf("load prepared: %v", err)
	}
	for _, tc := range []struct {
		name       string
		completion string
		compact    bool
		golden     string
	}{
		{"fail", "completion-fail.json", true, "tests/golden/compare-prepared-vs-completion-fail.txt"},
		{"fail-full", "completion-fail.json", false, "tests/golden/compare-prepared-vs-completion-fail-full.txt"},
		{"ok", "completion-ok.json", true, "tests/golden/compare-prepared-vs-completion-ok.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			completion, err := model.LoadCompletion(filepath.Join(root, "tests/fixtures/compare", tc.completion))
			if err != nil {
				t.Fatalf("load completion: %v", err)
			}
			var buf bytes.Buffer
			PreparedCompletionComparison(&buf, model.ComparePreparedToCompletion(prepared, completion),
				Color{Enabled: false}, tc.compact, source.NewIndex(), 5)
			got := strings.TrimRight(buf.String(), "\n")
			want := goldenStdout(t, filepath.Join(root, tc.golden))
			if got != want {
				t.Errorf("differs from %s:\n%s", tc.golden, firstDifference(got, want))
			}
		})
	}
}

// A failed submission has no update id, so completion data is all there is.
func TestFailedCompletionIsNotCommitted(t *testing.T) {
	root := repoRoot(t)
	completion, err := model.LoadCompletion(filepath.Join(root, "tests/fixtures/compare/completion-fail.json"))
	if err != nil {
		t.Fatalf("load completion: %v", err)
	}
	if completion.Committed() {
		t.Error("a failed completion must not report as committed")
	}
	if !completion.Failed() {
		t.Error("a non-OK status with no update id must report as failed")
	}
}

// With sources loaded, the completion view gains a Source diagnostics block.
func TestCompletionTraceWithSourceMatchesGolden(t *testing.T) {
	root := repoRoot(t)
	completion, err := model.LoadCompletion(filepath.Join(root, "tests/fixtures/failed-with-source.completion.json"))
	if err != nil {
		t.Fatalf("load completion: %v", err)
	}
	index := source.NewIndex()
	index.LoadDamlYAML(filepath.Join(root, "tests/fixtures/source-pkg/daml.yaml"))

	var buf bytes.Buffer
	CompletionTrace(&buf, completion, Color{Enabled: false}, index, 5)
	got := strings.TrimRight(buf.String(), "\n")

	// The golden records absolute paths scrubbed to <root>; do the same here.
	got = strings.ReplaceAll(got, root, "<root>")
	want := goldenStdout(t, filepath.Join(root, "tests/golden/completion-with-source.txt"))
	if got != want {
		t.Errorf("differs from the golden:\n%s", firstDifference(got, want))
	}
}
