package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture loads a committed test fixture from the repository root.
func fixture(t *testing.T, rel string) *Object {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	obj, err := Decode(data)
	if err != nil {
		t.Fatalf("decode fixture %s: %v", rel, err)
	}
	return obj
}

func normalize(t *testing.T, rel string, parties ...string) *Trace {
	t.Helper()
	trace, err := NormalizeTrace(fixture(t, rel), "ledger-json-api", "", parties)
	if err != nil {
		t.Fatalf("normalize %s: %v", rel, err)
	}
	return trace
}

func onlyEvent(t *testing.T, trace *Trace) *Event {
	t.Helper()
	if len(trace.EventsByID) != 1 {
		t.Fatalf("expected 1 event, got %d", len(trace.EventsByID))
	}
	for _, ev := range trace.EventsByID {
		return ev
	}
	return nil
}

// The real captures are the ones that matter: an earlier Python version handled
// only the hand-written variant names and produced untyped events against a
// live ledger while every test still passed.
func TestNormalizeRealUnassign(t *testing.T) {
	trace := normalize(t, "tests/fixtures/reassignment/real-unassign-update.json")
	ev := onlyEvent(t, trace)

	if ev.Kind != KindUnassign {
		t.Errorf("kind = %q, want %q", ev.Kind, KindUnassign)
	}
	if want := ":Iou:Iou"; !hasSuffix(ev.Template, want) {
		t.Errorf("template = %q, want suffix %q", ev.Template, want)
	}
	if !hasPrefix(ev.SourceSynchronizer, "sync-a::") {
		t.Errorf("source = %q, want sync-a prefix", ev.SourceSynchronizer)
	}
	if !hasPrefix(ev.TargetSynchronizer, "sync-b::") {
		t.Errorf("target = %q, want sync-b prefix", ev.TargetSynchronizer)
	}
	if ev.ReassignmentCounter == nil || *ev.ReassignmentCounter != 1 {
		t.Errorf("counter = %v, want 1", ev.ReassignmentCounter)
	}
	if !hasPrefix(ev.Submitter, "Alice::") {
		t.Errorf("submitter = %q", ev.Submitter)
	}
	if ev.ReassignmentID == "" {
		t.Error("reassignment id is empty")
	}
	// The unassign is committed on the source synchronizer.
	if !hasPrefix(trace.SynchronizerID, "sync-a::") {
		t.Errorf("synchronizer = %q, want sync-a prefix", trace.SynchronizerID)
	}
	if ev.EventID != "0" {
		t.Errorf("event id = %q, want %q (nodeId 0 must not fall back)", ev.EventID, "0")
	}
}

func TestNormalizeRealAssign(t *testing.T) {
	trace := normalize(t, "tests/fixtures/reassignment/real-assign-update.json")
	ev := onlyEvent(t, trace)

	if ev.Kind != KindAssign {
		t.Errorf("kind = %q, want %q", ev.Kind, KindAssign)
	}
	// Contract data is lifted out of the nested created event.
	if want := ":Iou:Iou"; !hasSuffix(ev.Template, want) {
		t.Errorf("template = %q, want suffix %q", ev.Template, want)
	}
	payload, ok := asObject(ev.Payload)
	if !ok {
		t.Fatalf("payload is %T, want an *Object", ev.Payload)
	}
	if amount, present := payload.Get("amount"); !present || amount == nil {
		t.Errorf("payload has no amount: %v", payload.Keys())
	}
	// Document order must survive decoding: the renderer prints payloads inline
	// in this order, and a Go map would randomize it.
	if got, want := payload.Keys(), []string{"payer", "owner", "amount", "viewers"}; !equalStrings(got, want) {
		t.Errorf("payload key order = %v, want %v", got, want)
	}
	if len(ev.Signatories) != 1 {
		t.Errorf("signatories = %v, want 1", ev.Signatories)
	}
	if len(ev.Observers) != 1 {
		t.Errorf("observers = %v, want 1", ev.Observers)
	}
	// Reassignment metadata comes from the event, not the nested created event.
	if !hasPrefix(ev.SourceSynchronizer, "sync-a::") || !hasPrefix(ev.TargetSynchronizer, "sync-b::") {
		t.Errorf("source/target = %q/%q", ev.SourceSynchronizer, ev.TargetSynchronizer)
	}
	// The assign is committed on the target synchronizer.
	if !hasPrefix(trace.SynchronizerID, "sync-b::") {
		t.Errorf("synchronizer = %q, want sync-b prefix", trace.SynchronizerID)
	}
}

// The synthetic fixtures cover the alternate variant names and the singular
// event shape, neither of which the real captures exercise.
func TestNormalizeSyntheticReassignment(t *testing.T) {
	for _, tc := range []struct {
		file string
		kind string
	}{
		{"tests/fixtures/reassignment/unassign-update.json", KindUnassign},
		{"tests/fixtures/reassignment/assign-update.json", KindAssign},
	} {
		trace := normalize(t, tc.file)
		ev := onlyEvent(t, trace)
		if ev.Kind != tc.kind {
			t.Errorf("%s: kind = %q, want %q", tc.file, ev.Kind, tc.kind)
		}
		if ev.ReassignmentCounter == nil || *ev.ReassignmentCounter != 1 {
			t.Errorf("%s: counter = %v, want 1", tc.file, ev.ReassignmentCounter)
		}
	}
}

func TestNormalizeSingleEventShape(t *testing.T) {
	raw := fixture(t, "tests/fixtures/reassignment/assign-update.json")
	reassignment, _ := pickObject(raw, "reassignment")
	events, _ := reassignment.Get("events")
	reassignment.Delete("events")
	reassignment.Set("event", events.([]any)[0])

	trace, err := NormalizeTrace(raw, "ledger-json-api", "", nil)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if ev := onlyEvent(t, trace); ev.Kind != KindAssign {
		t.Errorf("kind = %q, want %q", ev.Kind, KindAssign)
	}
}

// Trace artifacts nest the trace under a "trace" key; the committed compare
// fixtures are the ones the golden harness renders.
func TestNormalizeTransactionArtifact(t *testing.T) {
	raw := fixture(t, "tests/fixtures/compare/trace-b.json")
	inner, _ := pickObject(raw, "trace")
	trace, err := NormalizeTrace(inner, "ledger-json-api", "", nil)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if trace.UpdateID != "update-fixture-trace-b-0002" {
		t.Errorf("update id = %q", trace.UpdateID)
	}
	if len(trace.EventsByID) != 2 {
		t.Fatalf("events = %d, want 2", len(trace.EventsByID))
	}
	if len(trace.RootEventIDs) != 1 {
		t.Errorf("roots = %v, want 1", trace.RootEventIDs)
	}
	root, ok := trace.Event(trace.RootEventIDs[0])
	if !ok {
		t.Fatalf("root %q missing", trace.RootEventIDs[0])
	}
	if root.Kind != KindExercise {
		t.Errorf("root kind = %q, want %q", root.Kind, KindExercise)
	}
	if root.Consuming == nil || !*root.Consuming {
		t.Errorf("root consuming = %v, want true", root.Consuming)
	}
	if len(root.ChildEventIDs) != 1 {
		t.Fatalf("root children = %v, want 1", root.ChildEventIDs)
	}
	child, ok := trace.Event(root.ChildEventIDs[0])
	if !ok || child.Kind != KindCreate {
		t.Errorf("child = %v, want a create", child)
	}
}

// Offsets are large integers: float64 would both lose precision and reformat
// them on the way back out.
func TestDecodeKeepsLargeIntegersExact(t *testing.T) {
	obj, err := Decode([]byte(`{"offset": 9007199254740993}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	offset, _ := obj.Get("offset")
	if got := toString(offset); got != "9007199254740993" {
		t.Errorf("offset = %q, want %q", got, "9007199254740993")
	}
}

// Python does not escape these; Go does by default.
func TestEncodeDoesNotEscapeHTML(t *testing.T) {
	encoded, err := Encode(map[string]any{"note": "a < b & c > d"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := "{\n  \"note\": \"a < b & c > d\"\n}"
	if string(encoded) != want {
		t.Errorf("encode =\n%s\nwant\n%s", encoded, want)
	}
}

func TestNormalizeRejectsResponseWithoutUpdateID(t *testing.T) {
	empty := NewObject()
	empty.Set("events", []any{})
	if _, err := NormalizeTrace(empty, "ledger-json-api", "", nil); err == nil {
		t.Error("expected an error when no update id is present")
	}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Python's json.dumps defaults to ensure_ascii=True, so every artifact and
// report it has ever written escapes non-ASCII. daml test puts box-drawing
// characters in transaction trees, so this is reachable in real output.
func TestEncodeEscapesNonASCII(t *testing.T) {
	obj := NewObject()
	obj.Set("tree", "\u2514\u2500> creates")
	encoded, err := Encode(obj)
	if err != nil {
		t.Fatal(err)
	}
	got := string(encoded)
	if !strings.Contains(got, `\u2514`) || !strings.Contains(got, `\u2500`) {
		t.Errorf("non-ASCII not escaped: %s", got)
	}
	if strings.ContainsRune(got, '\u2514') {
		t.Errorf("raw non-ASCII survived: %s", got)
	}
}

func TestEncodeEscapesAstralAsSurrogatePair(t *testing.T) {
	obj := NewObject()
	obj.Set("emoji", "\U0001F600")
	encoded, err := Encode(obj)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded); !strings.Contains(got, `\ud83d\ude00`) {
		t.Errorf("astral rune not encoded as a surrogate pair: %s", got)
	}
}
