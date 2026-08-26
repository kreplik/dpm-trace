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

// A rejected submission names its command id inside the error context rather
// than at the top level, so the header showed "-" for a value the payload had.
func TestCompletionRecoversCommandIDFromContext(t *testing.T) {
	raw, err := model.Decode([]byte(`{
	  "code": "DAML_FAILURE",
	  "cause": "Interpretation error",
	  "context": {
	    "commands": "{readAs: [], submissionId: 'sub-9', commandId: 'dpm-trace-submit-abc123', actAs: [Alice]}",
	    "error_id": "UNHANDLED_EXCEPTION/DA.Exception.AssertionFailed",
	    "definite_answer": "false",
	    "participant": "'participant1'",
	    "exercise_trace": "    in choice pkg:Asset:Asset:Withdraw on contract 00abc (#0)\n"
	  }
	}`))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	CompletionTrace(&buf, &model.Completion{Raw: raw}, Color{}, nil, 5)
	out := buf.String()
	for _, want := range []string{
		"command id: dpm-trace-submit-abc123",
		"submission: sub-9",
		"error id:   UNHANDLED_EXCEPTION/DA.Exception.AssertionFailed",
		"definite:   false",
		"rejected:   participant1",
		"in choice pkg:Asset:Asset:Withdraw on contract 00abc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

// Error details say whether retrying can help and how to find the submission
// in participant logs. They were dropped entirely.
func TestCompletionShowsErrorDetails(t *testing.T) {
	raw, err := model.Decode([]byte(`{
	  "code": "DAML_FAILURE",
	  "cause": "boom",
	  "errorCategory": 9,
	  "grpcCodeValue": 9,
	  "traceId": "027a17197c88188ee626eb6feb347bcb",
	  "correlationId": "d3c8f879-2d60-4f5b-9dfd-a67eed185d71",
	  "resources": [["ErrorResource(CONTRACT_ID)", "00abcdef"]]
	}`))
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	CompletionTrace(&buf, &model.Completion{Raw: raw}, Color{}, nil, 5)
	out := buf.String()
	for _, want := range []string{
		"category:   9 (gRPC 9)",
		"trace id:   027a17197c88188ee626eb6feb347bcb",
		"corr id:    d3c8f879-2d60-4f5b-9dfd-a67eed185d71",
		"resources:  CONTRACT_ID 00abcdef",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}

// A completion without those fields must not grow empty lines for them.
func TestCompletionOmitsAbsentErrorDetails(t *testing.T) {
	raw, err := model.Decode([]byte(`{"commandId": "c1", "status": {"code": 9, "message": "no"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	CompletionTrace(&buf, &model.Completion{Raw: raw}, Color{}, nil, 5)
	for _, absent := range []string{"category:", "trace id:", "corr id:", "resources:"} {
		if strings.Contains(buf.String(), absent) {
			t.Errorf("printed %q for a completion without it:\n%s", absent, buf.String())
		}
	}
}
