package render

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/source"
)

// The tag tells a reader how much to trust a location: text matching is a
// guess about the source, debug info and damlc inspect are confirmed against
// the compiled package.
func TestLabelTag(t *testing.T) {
	for _, tc := range []struct{ label, want string }{
		{"damlc inspect: Asset", "debug-info"},
		{"debug info", "debug-info"},
		{"Debug-Info", "debug-info"},
		{"local source", "text match"},
		{"", "text match"},
	} {
		if got := LabelTag(tc.label); got != tc.want {
			t.Errorf("LabelTag(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

// "local source" carries no information inline, so it is dropped; the damlc
// prefix is noise once the tag already says debug-info.
func TestLabelDisplay(t *testing.T) {
	for _, tc := range []struct{ label, want string }{
		{"local source", ""},
		{"", ""},
		{"damlc inspect: Asset", "Asset"},
		{"something else", "something else"},
	} {
		if got := labelDisplay(tc.label); got != tc.want {
			t.Errorf("labelDisplay(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}

// Paths are shown relative to a loaded source root so output does not leak a
// developer's home directory; without a matching root, the base name is used.
func TestShortSourcePath(t *testing.T) {
	ix := source.NewIndex()
	ix.LoadDamlYAML(filepath.Join("..", "..", "tests/fixtures/source-pkg/daml.yaml"))
	if len(ix.Roots) == 0 {
		t.Fatal("fixture loaded no source roots")
	}
	root := ix.Roots[0]

	if got := ShortSourcePath(source.Location{Path: root + "/FailureDemo.daml"}, ix); got != "FailureDemo.daml" {
		t.Errorf("relative to root = %q", got)
	}
	if got := ShortSourcePath(source.Location{Path: "/elsewhere/Other.daml"}, ix); got != "Other.daml" {
		t.Errorf("outside any root = %q, want the base name", got)
	}
	if got := ShortSourcePath(source.Location{Path: "Bare.daml"}, nil); got != "Bare.daml" {
		t.Errorf("no index = %q", got)
	}
}

// The inline header is what the compact submit-failure view prints; it must
// carry file:line and the trust tag in every branch.
func TestRenderLocationInline(t *testing.T) {
	color := Color{Enabled: false}
	loc := source.Location{Path: "/pkg/Asset.daml", Line: 54, Column: 20, Label: "local source"}

	plain := RenderLocationInline(loc, nil, color, "", "")
	if !strings.Contains(plain, "Asset.daml:54") || !strings.Contains(plain, "[text match]") {
		t.Errorf("plain = %q", plain)
	}

	withEntity := RenderLocationInline(loc, nil, color, "choice", "Withdraw")
	if !strings.Contains(withEntity, "in choice Withdraw") {
		t.Errorf("entity hint missing from %q", withEntity)
	}

	inspected := source.Location{Path: "/pkg/Asset.daml", Line: 54, Label: "damlc inspect: Asset"}
	tagged := RenderLocationInline(inspected, nil, color, "", "")
	if !strings.Contains(tagged, "[debug-info]") || !strings.Contains(tagged, "Asset") {
		t.Errorf("inspect label = %q", tagged)
	}
}
