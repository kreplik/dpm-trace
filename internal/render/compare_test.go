package render

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

var compareGoldens = []struct {
	name    string
	compact bool
	golden  string
}{
	{"compact", true, "tests/golden/compare-update-vs-update.txt"},
	{"full", false, "tests/golden/compare-update-vs-update-full.txt"},
}

func TestUpdateComparisonMatchesGoldens(t *testing.T) {
	root := repoRoot(t)
	load := func(rel string) *model.Trace {
		t.Helper()
		artifact, err := model.LoadTraceArtifact(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("load %s: %v", rel, err)
		}
		trace, err := model.TraceFromArtifact(artifact)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return trace
	}

	left := load("tests/fixtures/compare/trace-a.json")
	right := load("tests/fixtures/compare/trace-b.json")
	comparison := model.CompareTraces(left, right)

	for _, tc := range compareGoldens {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			UpdateComparison(&buf, comparison, Color{Enabled: false}, tc.compact)
			got := strings.TrimRight(buf.String(), "\n")
			want := goldenStdout(t, filepath.Join(root, tc.golden))
			if got != want {
				t.Errorf("differs from %s:\n%s", tc.golden, firstDifference(got, want))
			}
		})
	}
}

// comparable_value treats numbers and strings alike, so 1 and "1" must not
// register as a difference.
func TestComparableEqualIgnoresScalarType(t *testing.T) {
	one, _ := model.Decode([]byte(`{"v": 1}`))
	oneString, _ := model.Decode([]byte(`{"v": "1"}`))
	two, _ := model.Decode([]byte(`{"v": 2}`))
	if !comparableEqual(one, oneString) {
		t.Error("1 and \"1\" should compare equal")
	}
	if comparableEqual(one, two) {
		t.Error("1 and 2 should not compare equal")
	}
}
