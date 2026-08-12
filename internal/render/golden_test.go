package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// The golden harness (tests/check-golden.py with DPM_TRACE_BIN) is the oracle
// for the port, but it only runs when someone remembers to point it at the Go
// binary. These tests read the same committed goldens directly, so a rendering
// regression fails `go test ./...` as well.
var goldenCases = []struct {
	name     string
	artifact string
	golden   string
}{
	{"trace-a", "tests/fixtures/compare/trace-a.json", "tests/golden/open-trace-a.txt"},
	{"trace-b", "tests/fixtures/compare/trace-b.json", "tests/golden/open-trace-b.txt"},
	{"reassign-unassign", "tests/fixtures/reassignment/real-unassign-artifact.json", "tests/golden/open-reassign-unassign.txt"},
	{"reassign-assign", "tests/fixtures/reassignment/real-assign-artifact.json", "tests/golden/open-reassign-assign.txt"},
}

func TestRendersMatchGoldens(t *testing.T) {
	root := repoRoot(t)
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			artifact, err := model.LoadTraceArtifact(filepath.Join(root, tc.artifact))
			if err != nil {
				t.Fatalf("load artifact: %v", err)
			}
			trace, err := model.TraceFromArtifact(artifact)
			if err != nil {
				t.Fatalf("read trace: %v", err)
			}

			var buf bytes.Buffer
			buf.WriteString(TraceArtifactSummary(artifact))
			buf.WriteString("\n")
			PrettyTrace(&buf, trace, Color{Enabled: false})

			got := strings.TrimRight(buf.String(), "\n")
			want := goldenStdout(t, filepath.Join(root, tc.golden))
			if got != want {
				t.Errorf("render differs from %s:\n%s", tc.golden, firstDifference(got, want))
			}
		})
	}
}

// Payload key order is the reason model.Object exists; assert it at the
// rendering layer too, since that is where a regression would show.
func TestPayloadRendersInDocumentOrder(t *testing.T) {
	root := repoRoot(t)
	artifact, err := model.LoadTraceArtifact(filepath.Join(root, "tests/fixtures/reassignment/real-assign-artifact.json"))
	if err != nil {
		t.Fatalf("load artifact: %v", err)
	}
	trace, err := model.TraceFromArtifact(artifact)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}

	var buf bytes.Buffer
	PrettyTrace(&buf, trace, Color{Enabled: false})
	const want = "payload: { payer: Alice, owner: Bob, amount: {\"currency\": \"USD\", \"value\": \"100.0000000000\"}, viewers: [] }"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("payload line missing or reordered; want:\n%s", want)
	}
}

// goldenStdout extracts the recorded stdout section of a golden file.
func goldenStdout(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	const marker = "--- stdout:\n"
	index := strings.Index(string(data), marker)
	if index < 0 {
		t.Fatalf("golden %s has no stdout section", path)
	}
	body := string(data)[index+len(marker):]
	if end := strings.Index(body, "\n--- stderr:"); end >= 0 {
		body = body[:end]
	}
	return strings.TrimRight(body, "\n")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return abs
}

func firstDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := lineAt(gotLines, i), lineAt(wantLines, i)
		if g != w {
			return "- go:     " + g + "\n+ python: " + w
		}
	}
	return "(no line differences)"
}

func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return "<missing>"
	}
	return lines[i]
}
