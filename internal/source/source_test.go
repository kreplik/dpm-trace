package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

func testIndex(t *testing.T) *Index {
	t.Helper()
	ix := NewIndex()
	ix.LoadDamlYAML(filepath.Join("..", "..", "tests/fixtures/source-pkg/daml.yaml"))
	if !ix.HasSources() {
		t.Fatal("no sources loaded from the fixture daml.yaml")
	}
	return ix
}

func TestFindFailureTextFallsBackToLocalSource(t *testing.T) {
	ix := testIndex(t)
	locations := ix.FindFailureText("Insufficient balance", 5)
	if len(locations) == 0 {
		t.Fatal("expected a match for the failure text")
	}
	loc := locations[0]
	// Without damlc inspect there are no inspected modules, so the label must
	// report the fallback rather than claiming inspect confirmed it.
	if loc.Label != "local source" {
		t.Errorf("label = %q, want %q", loc.Label, "local source")
	}
	if !strings.HasSuffix(loc.Path, "FailureDemo.daml") {
		t.Errorf("path = %q", loc.Path)
	}
	if loc.Line != 20 || loc.Column != 18 {
		t.Errorf("position = %d:%d, want 20:18", loc.Line, loc.Column)
	}
}

func TestSnippetRendersCaretUnderTheMatch(t *testing.T) {
	ix := testIndex(t)
	loc := ix.FindFailureText("Insufficient balance", 1)[0]
	snippet := ix.Snippet(loc, 2)
	if !strings.Contains(snippet, "> 20 |") {
		t.Errorf("matched line not marked:\n%s", snippet)
	}
	if !strings.Contains(snippet, "^") {
		t.Errorf("no caret:\n%s", snippet)
	}
	// The caret must sit under the match, not at the start of the line.
	for _, line := range strings.Split(snippet, "\n") {
		if strings.TrimSpace(line) == "^" && strings.Index(line, "^") < 10 {
			t.Errorf("caret column looks wrong: %q", line)
		}
	}
}

// Needles are ordered most-specific first: quoted fragments before the whole
// message, so the tightest match wins.
func TestNeedlesPrefersQuotedFragments(t *testing.T) {
	needles := Needles(`Command failed: abort "Insufficient balance" raised`)
	if len(needles) == 0 {
		t.Fatal("no needles extracted")
	}
	if needles[0] != "Insufficient balance" {
		t.Errorf("first needle = %q, want the quoted fragment", needles[0])
	}
	for _, needle := range needles {
		if len(needle) < 4 {
			t.Errorf("needle %q is shorter than the 4-character floor", needle)
		}
	}
}

func TestNeedlesDeduplicates(t *testing.T) {
	needles := Needles(`"same" and "same"`)
	seen := map[string]bool{}
	for _, needle := range needles {
		if seen[needle] {
			t.Errorf("duplicate needle %q", needle)
		}
		seen[needle] = true
	}
}

// A missing daml.yaml must not fail: source mapping is best-effort.
func TestMissingDamlYAMLIsIgnored(t *testing.T) {
	ix := NewIndex()
	ix.LoadDamlYAML("does/not/exist/daml.yaml")
	if ix.HasSources() {
		t.Error("expected no sources")
	}
	if locations := ix.FindFailureText("anything", 5); len(locations) != 0 {
		t.Errorf("expected no locations, got %d", len(locations))
	}
}

// Debug info is what resolves a template or choice to its definition; without
// it, LocationForEvent has nothing to look up.
func TestLoadDebugInfoResolvesTemplatesAndChoices(t *testing.T) {
	ix := NewIndex()
	ix.LoadDebugInfo(filepath.Join("..", "..", "tests/fixtures/debug-info/token-debug-info.json"))
	if !ix.HasSources() {
		t.Fatal("debug info did not load the referenced source file")
	}

	template := &model.Event{Template: "pkg2ccdd:Token:Token", Kind: model.KindCreate}
	loc := ix.LocationForEvent(template)
	if loc == nil {
		t.Fatal("template not resolved")
	}
	if loc.Line != 8 || loc.Label != "Token:Token" {
		t.Errorf("template at %d (%q), want line 8", loc.Line, loc.Label)
	}

	choice := &model.Event{Template: "pkg2ccdd:Token:Token", Kind: model.KindExercise, Choice: "Transfer"}
	loc = ix.LocationForEvent(choice)
	if loc == nil {
		t.Fatal("choice not resolved")
	}
	if loc.Line != 14 || loc.Label != "Token:Token.Transfer" {
		t.Errorf("choice at %d (%q), want line 14", loc.Line, loc.Label)
	}
}

// A package id that does not match must not resolve: debug info is per package.
func TestLocationForEventRequiresMatchingPackage(t *testing.T) {
	ix := NewIndex()
	ix.LoadDebugInfo(filepath.Join("..", "..", "tests/fixtures/debug-info/token-debug-info.json"))
	other := &model.Event{Template: "otherpkg:Token:Token", Kind: model.KindCreate}
	if loc := ix.LocationForEvent(other); loc != nil {
		t.Errorf("resolved %v for a different package", loc)
	}
}

// Source mapping is best-effort: a missing or malformed file must be ignored.
func TestLoadDebugInfoIgnoresBadInput(t *testing.T) {
	ix := NewIndex()
	ix.LoadDebugInfo("/nonexistent/debug.json")

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix.LoadDebugInfo(bad)

	noPackage := filepath.Join(t.TempDir(), "nopkg.json")
	if err := os.WriteFile(noPackage, []byte(`{"files": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ix.LoadDebugInfo(noPackage)

	if ix.HasSources() {
		t.Error("bad input should load nothing")
	}
}

func TestFormatPathOmitsColumnOne(t *testing.T) {
	if got := FormatPath(Location{Path: "/a/B.daml", Line: 8, Column: 1}); got != "/a/B.daml:8" {
		t.Errorf("got %q, want the column omitted", got)
	}
	if got := FormatPath(Location{Path: "/a/B.daml", Line: 8, Column: 18}); got != "/a/B.daml:8:18" {
		t.Errorf("got %q, want the column included", got)
	}
}
