package model

import (
	"os"
	"path/filepath"
	"testing"
)

// An exported artifact must reopen into the same trace: export and open are
// the two halves of one contract.
func TestTraceArtifactRoundTrip(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests/fixtures/compare/trace-b.json"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	original, err := TraceFromArtifact(source)
	if err != nil {
		t.Fatal(err)
	}

	artifact := NewTraceArtifact(original, "http://localhost:7575", "", nil, nil)
	encoded, err := Encode(artifact)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := TraceFromArtifact(reopened)
	if err != nil {
		t.Fatal(err)
	}

	if restored.UpdateID != original.UpdateID {
		t.Errorf("update id = %q, want %q", restored.UpdateID, original.UpdateID)
	}
	if len(restored.EventsByID) != len(original.EventsByID) {
		t.Errorf("events = %d, want %d", len(restored.EventsByID), len(original.EventsByID))
	}
	if restored.Projection.Note != original.Projection.Note {
		t.Errorf("projection note not preserved")
	}
	for id, want := range original.EventsByID {
		got, ok := restored.EventsByID[id]
		if !ok {
			t.Errorf("event %q missing after round trip", id)
			continue
		}
		if got.Kind != want.Kind || got.Template != want.Template {
			t.Errorf("event %q = %s/%s, want %s/%s", id, got.Kind, got.Template, want.Kind, want.Template)
		}
		if len(got.ChildEventIDs) != len(want.ChildEventIDs) {
			t.Errorf("event %q children = %v, want %v", id, got.ChildEventIDs, want.ChildEventIDs)
		}
	}
}

// The privacy block travels with the artifact: it records which projection the
// data came from and what is deliberately absent.
func TestTraceArtifactCarriesPrivacyScope(t *testing.T) {
	trace := &Trace{
		UpdateID:   "update-1",
		EventsByID: map[string]*Event{},
		Projection: Projection{Note: "Authorized participant projection.", ReadAs: []string{"Alice::1220ab"}},
	}
	artifact := NewTraceArtifact(trace, "http://localhost:7575", "", nil, nil)

	privacy, ok := artifact["privacy"].(map[string]any)
	if !ok {
		t.Fatal("no privacy block")
	}
	if privacy["scope"] != "Authorized participant projection." {
		t.Errorf("scope = %v", privacy["scope"])
	}
	if privacy["missingPrivateDataPolicy"] == nil {
		t.Error("missingPrivateDataPolicy absent")
	}
	if artifact["kind"] != "committed-update" {
		t.Errorf("kind = %v", artifact["kind"])
	}
}

// Missing DAR paths are reported as missing rather than silently dropped.
func TestPackageMetadataSplitsMissingPaths(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "real.dar")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	context := packageMetadataContext([]string{existing, "/nope/absent.dar"}, nil, []string{"pkg1"})

	if context["available"] != true {
		t.Error("available should be true when a DAR exists")
	}
	if got := context["darPaths"].([]string); len(got) != 1 {
		t.Errorf("darPaths = %v", got)
	}
	if got := context["missingDarPaths"].([]string); len(got) != 1 {
		t.Errorf("missingDarPaths = %v", got)
	}
}
