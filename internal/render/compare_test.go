package render

import (
	"bytes"
	"path/filepath"
	"slices"
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

func TestPreparedUpdateComparisonMatchesGoldens(t *testing.T) {
	root := repoRoot(t)
	prepared, err := model.LoadPreparedArtifact(filepath.Join(root, "tests/fixtures/compare/prepared.json"))
	if err != nil {
		t.Fatalf("load prepared: %v", err)
	}
	artifact, err := model.LoadTraceArtifact(filepath.Join(root, "tests/fixtures/compare/trace-a.json"))
	if err != nil {
		t.Fatalf("load trace: %v", err)
	}
	trace, err := model.TraceFromArtifact(artifact)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	comparison := model.ComparePreparedToTrace(prepared, trace)

	for _, tc := range []struct {
		name    string
		compact bool
		golden  string
	}{
		{"compact", true, "tests/golden/compare-prepared-vs-update.txt"},
		{"full", false, "tests/golden/compare-prepared-vs-update-full.txt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			PreparedUpdateComparison(&buf, comparison, Color{Enabled: false}, tc.compact)
			got := strings.TrimRight(buf.String(), "\n")
			want := goldenStdout(t, filepath.Join(root, tc.golden))
			if got != want {
				t.Errorf("differs from %s:\n%s", tc.golden, firstDifference(got, want))
			}
		})
	}
}

// A payload that changed in three places was reported as one: the annotation
// returned on the first differing key, so which fields actually moved -- the
// question a comparison exists to answer -- was invisible past the first.
func TestChangedFieldsReportsEveryDifference(t *testing.T) {
	left := model.NewObject()
	left.Set("issuer", "Issuer::1220aa")
	left.Set("name", "GOLD")
	left.Set("owner", "Alice::1220bb")
	left.Set("quantity", "100")

	right := model.NewObject()
	right.Set("issuer", "Issuer::1220aa")
	right.Set("name", "SILVER")
	right.Set("owner", "Bob::1220cc")
	right.Set("quantity", "42")

	changed := changedFields(left, right)
	if len(changed) != 3 {
		t.Fatalf("changedFields = %v, want three entries", changed)
	}
	for _, want := range []string{"name: GOLD → SILVER", "quantity: 100 → 42"} {
		if !slices.Contains(changed, want) {
			t.Errorf("changedFields missing %q, got %v", want, changed)
		}
	}
	// Party ids are shortened: printed whole, two of them push the change off
	// the line.
	if !strings.Contains(strings.Join(changed, " "), "Alice::1220bb → Bob::1220cc") &&
		!strings.Contains(strings.Join(changed, " "), "...") {
		t.Errorf("party change not rendered readably: %v", changed)
	}
	// An unchanged field must not appear.
	if strings.Contains(strings.Join(changed, " "), "issuer") {
		t.Errorf("unchanged field reported: %v", changed)
	}
}
