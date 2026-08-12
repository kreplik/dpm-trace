package model

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Fixtures both implementations must normalize identically. Artifacts (which
// nest the update under "trace") and raw API responses are both included.
var differentialFixtures = []string{
	"tests/fixtures/reassignment/real-unassign-update.json",
	"tests/fixtures/reassignment/real-assign-update.json",
	"tests/fixtures/reassignment/unassign-update.json",
	"tests/fixtures/reassignment/assign-update.json",
	"tests/fixtures/reassignment/real-unassign-artifact.json",
	"tests/fixtures/reassignment/real-assign-artifact.json",
	"tests/fixtures/compare/trace-a.json",
	"tests/fixtures/compare/trace-b.json",
}

// TestDifferentialAgainstPython normalizes every fixture with both
// implementations and requires identical output.
//
// The golden harness (tests/check-golden.py with DPM_TRACE_BIN) is the real
// oracle for this port, but it can only compare implementations that have a CLI
// surface. This package sits below that, so until `trace open` is ported this
// is the only thing keeping the Go normalizer honest. Delete it when the Python
// implementation is retired.
//
// Skips when no Python interpreter is available, so a Go-only environment can
// still run `go test ./...`. Set DPM_TRACE_PYTHON to choose the interpreter.
func TestDifferentialAgainstPython(t *testing.T) {
	root := repoRoot(t)
	python := findPython(t, root)

	args := append([]string{filepath.Join(root, "tests", "dump-model.py")}, differentialFixtures...)
	cmd := exec.Command(python, args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running dump-model.py: %v\n%s", err, stderr.String())
	}

	var want any
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatalf("decoding python output: %v", err)
	}
	wantJSON, err := Encode(want)
	if err != nil {
		t.Fatalf("re-encoding python output: %v", err)
	}

	gotJSON, err := Encode(goDump(t, root))
	if err != nil {
		t.Fatalf("encoding go output: %v", err)
	}

	if !bytes.Equal(gotJSON, wantJSON) {
		t.Errorf("Go and Python normalization differ:\n%s", firstDifference(string(gotJSON), string(wantJSON)))
	}
}

// goDump mirrors tests/dump-model.py. Field names and ordering must match it
// exactly; both sides are re-encoded with sorted keys before comparison so only
// values can differ, never formatting.
func goDump(t *testing.T, root string) []any {
	t.Helper()
	dumps := make([]any, 0, len(differentialFixtures))

	for _, rel := range differentialFixtures {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		raw, err := Decode(data)
		if err != nil {
			t.Fatalf("decode %s: %v", rel, err)
		}
		if inner, ok := pickObject(raw, "trace"); ok {
			raw = inner
		}
		trace, err := NormalizeTrace(raw, "ledger-json-api", "", nil)
		if err != nil {
			t.Fatalf("normalize %s: %v", rel, err)
		}

		ids := make([]string, 0, len(trace.EventsByID))
		for id := range trace.EventsByID {
			ids = append(ids, id)
		}
		sortEventIDsLexically(ids)

		events := make([]any, 0, len(ids))
		for _, id := range ids {
			ev := trace.EventsByID[id]
			events = append(events, map[string]any{
				"eventId":             ev.EventID,
				"kind":                ev.Kind,
				"template":            nilIfEmpty(ev.Template),
				"contractId":          nilIfEmpty(ev.ContractID),
				"choice":              nilIfEmpty(ev.Choice),
				"consuming":           boolOrNil(ev.Consuming),
				"actingParties":       stringsOrEmpty(ev.ActingParties),
				"witnesses":           stringsOrEmpty(ev.Witnesses),
				"signatories":         stringsOrEmpty(ev.Signatories),
				"observers":           stringsOrEmpty(ev.Observers),
				"childEventIds":       stringsOrEmpty(ev.ChildEventIDs),
				"payload":             ev.Payload,
				"argument":            ev.Argument,
				"result":              ev.Result,
				"sourceSynchronizer":  nilIfEmpty(ev.SourceSynchronizer),
				"targetSynchronizer":  nilIfEmpty(ev.TargetSynchronizer),
				"reassignmentId":      nilIfEmpty(ev.ReassignmentID),
				"reassignmentCounter": intOrNil(ev.ReassignmentCounter),
				"submitter":           nilIfEmpty(ev.Submitter),
			})
		}

		dumps = append(dumps, map[string]any{
			"fixture":        filepath.Base(rel),
			"updateId":       trace.UpdateID,
			"offset":         nilIfEmpty(trace.Offset),
			"recordTime":     nilIfEmpty(trace.RecordTime),
			"synchronizerId": nilIfEmpty(trace.SynchronizerID),
			"rootEventIds":   stringsOrEmpty(trace.RootEventIDs),
			"events":         events,
		})
	}
	return dumps
}

// Python renders an absent string field as JSON null; Go renders "".
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolOrNil(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func intOrNil(n *int64) any {
	if n == nil {
		return nil
	}
	return *n
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func sortEventIDsLexically(ids []string) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return abs
}

func findPython(t *testing.T, root string) string {
	t.Helper()
	candidates := []string{
		os.Getenv("DPM_TRACE_PYTHON"),
		filepath.Join(root, ".venv", "bin", "python"),
		"python3",
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	t.Skip("no Python interpreter found; skipping differential test")
	return ""
}

// firstDifference reports the first differing line with a little context, so a
// failure points at the field rather than dumping both documents.
func firstDifference(got, want string) string {
	gotLines, wantLines := splitLines(got), splitLines(want)
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := lineAt(gotLines, i), lineAt(wantLines, i)
		if g != w {
			var b bytes.Buffer
			for j := max(0, i-3); j < i; j++ {
				b.WriteString("  " + lineAt(wantLines, j) + "\n")
			}
			b.WriteString("- go:     " + g + "\n")
			b.WriteString("+ python: " + w + "\n")
			return b.String()
		}
	}
	return "(no line differences; encodings differ)"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}

func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return "<missing>"
	}
	return lines[i]
}
