package render

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/source"
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

// A difference in a child event is still a difference. Counting only roots
// summarised a changed transaction as "no differences" while the printer,
// which does recurse, listed that very change underneath -- a headline
// contradicting its own body.
func TestChildDifferencesAreCounted(t *testing.T) {
	child := func(owner string) model.CompareRow {
		payload := model.NewObject()
		payload.Set("owner", owner)
		return model.CompareRow{
			Kind: "create", Template: "pkg:Token:Token",
			ContractID: "00cc", Value: payload, ValueLabel: "payload",
		}
	}
	parent := func(owner string) model.CompareRow {
		return model.CompareRow{
			Kind: "exercise", Template: "pkg:Token:Token", Choice: "Transfer",
			ContractID: "00aa", Children: []model.CompareRow{child(owner)},
		}
	}

	left := []model.CompareRow{parent("Bob")}
	right := []model.CompareRow{parent("Mallory")}

	nValue, nOnlyA, nOnlyB := countEventDiffs(left, right)
	if nValue != 1 {
		t.Errorf("value differences = %d, want 1 from the child", nValue)
	}
	if nOnlyA != 0 || nOnlyB != 0 {
		t.Errorf("only-in counts = %d/%d, want 0/0", nOnlyA, nOnlyB)
	}

	// Identical subtrees stay identical.
	if v, a, b := countEventDiffs(left, []model.CompareRow{parent("Bob")}); v+a+b != 0 {
		t.Errorf("identical subtrees reported %d/%d/%d differences", v, a, b)
	}
}

// The trace-context loop hardcoded two spaces where every other label pads to
// a column, so short keys landed left of it.
func TestCompletionLabelsShareOneColumn(t *testing.T) {
	root := repoRoot(t)
	prepared, err := model.LoadPreparedArtifact(filepath.Join(root, "tests/fixtures/compare/prepared.json"))
	if err != nil {
		t.Fatalf("load prepared: %v", err)
	}
	completion, err := model.LoadCompletion(filepath.Join(root, "tests/fixtures/compare/completion-fail.json"))
	if err != nil {
		t.Fatalf("load completion: %v", err)
	}

	var buf bytes.Buffer
	PreparedCompletionComparison(&buf, model.ComparePreparedToCompletion(prepared, completion),
		Color{Enabled: false}, false, source.NewIndex(), 5)

	column, inCompletion := -1, false
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "Completion") {
			inCompletion = true
			continue
		}
		if !inCompletion || !strings.HasPrefix(line, "  ") || !strings.Contains(line, ":") {
			continue
		}
		idx := strings.Index(line, ":")
		value := strings.TrimLeft(line[idx+1:], " ")
		if value == "" {
			continue
		}
		at := len(line) - len(value)
		if column == -1 {
			column = at
			continue
		}
		if at != column {
			t.Errorf("label column %d, want %d: %q", at, column, line)
		}
	}
}

// CompareRow has always carried the exercise result, the stakeholders and the
// reassignment metadata, and nothing compared them: a transaction could change
// in any of those ways and the comparison called it identical.
func TestChangedRowFieldsCoversEveryCarriedField(t *testing.T) {
	counter := func(n int64) *int64 { return &n }
	base := func() model.CompareRow {
		payload := model.NewObject()
		payload.Set("owner", "Alice::1220aa")
		return model.CompareRow{
			Kind: "exercise", Template: "pkg:Token:Token", Choice: "Transfer",
			ContractID: "00aa", Value: payload, ValueLabel: "argument",
			Result:              "old",
			Signatories:         []string{"Alice::1220aa"},
			Observers:           []string{"Bob::1220bb"},
			Witnesses:           []string{"Alice::1220aa"},
			ActingParties:       []string{"Alice::1220aa"},
			SourceSynchronizer:  "sync-a::1220cc",
			TargetSynchronizer:  "sync-b::1220dd",
			ReassignmentCounter: counter(1),
		}
	}

	if changed := changedRowFields(base(), base()); len(changed) != 0 {
		t.Errorf("identical rows reported %v", changed)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*model.CompareRow)
		want   string
	}{
		{"result", func(r *model.CompareRow) { r.Result = "new" }, "result:"},
		{"signatories", func(r *model.CompareRow) { r.Signatories = []string{"Mallory::1220ff"} }, "signatories:"},
		{"observers", func(r *model.CompareRow) { r.Observers = nil }, "observers:"},
		{"witnesses", func(r *model.CompareRow) { r.Witnesses = []string{"Bob::1220bb"} }, "witnesses:"},
		{"acting", func(r *model.CompareRow) { r.ActingParties = []string{"Bob::1220bb"} }, "acting:"},
		{"source synchronizer", func(r *model.CompareRow) { r.SourceSynchronizer = "sync-z::1220ee" }, "source synchronizer:"},
		{"target synchronizer", func(r *model.CompareRow) { r.TargetSynchronizer = "" }, "target synchronizer:"},
		{"reassignment counter", func(r *model.CompareRow) { r.ReassignmentCounter = counter(2) }, "reassignment counter:"},
		{"counter cleared", func(r *model.CompareRow) { r.ReassignmentCounter = nil }, "reassignment counter:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			right := base()
			tc.mutate(&right)
			changed := changedRowFields(base(), right)
			if len(changed) == 0 {
				t.Fatalf("a changed %s was not reported", tc.name)
			}
			if !strings.Contains(strings.Join(changed, " "), tc.want) {
				t.Errorf("changed = %v, want it to name %q", changed, tc.want)
			}
		})
	}
}
