package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// CompletionSourceDiagnostics resolves a completion's failure message against
// the loaded sources, most specific needle first, deduplicated by position.
// Ports completion_source_diagnostics.
func CompletionSourceDiagnostics(c *model.Completion, index *source.Index, maxLocations int) ([]source.Location, bool) {
	if index == nil || !index.HasSources() {
		return nil, false
	}
	_, message := c.StatusFields()

	seen := map[string]bool{}
	var result []source.Location
	capped := false

	for _, needle := range source.Needles(scalarText(message)) {
		for _, loc := range index.FindFailureText(needle, 5) {
			key := fmt.Sprintf("%s\x00%d\x00%d", loc.Path, loc.Line, loc.Column)
			if seen[key] {
				continue
			}
			seen[key] = true
			if len(result) >= maxLocations {
				capped = true
				break
			}
			result = append(result, loc)
		}
		if capped {
			break
		}
	}
	return result, capped
}

// SourceDiagnostic renders one location as a snippet with a caret.
// Ports render_source_diagnostic.
func SourceDiagnostic(loc source.Location, index *source.Index, color Color) string {
	if index == nil {
		return source.FormatPath(loc)
	}
	rendered := index.Snippet(loc, 2)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return source.FormatPath(loc)
	}
	lines[0] = color.Apply(lines[0], "cyan")
	if loc.Label != "" {
		lines = append(lines[:1], append([]string{"basis: " + loc.Label}, lines[1:]...)...)
	}
	return strings.Join(lines, "\n")
}

// PrintSourceDiagnostics writes the "Source diagnostics" block.
func PrintSourceDiagnostics(w io.Writer, locations []source.Location, capped bool, index *source.Index, color Color) {
	if len(locations) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.Apply("Source diagnostics", "cyan", "bold"))
	for _, loc := range locations {
		fmt.Fprintln(w, IndentText(SourceDiagnostic(loc, index, color)))
	}
	if capped {
		fmt.Fprintf(w, "  %s\n", color.Apply(
			fmt.Sprintf("(capped at %d locations; pass --max-source-locations to raise)", len(locations)), "gray"))
	}
}

// LabelTag classifies where a location came from, shown as [text match] or
// [debug-info]. The distinction matters: a debug-info match is confirmed
// against the compiled package, a text match is a local-source guess.
// Ports _label_tag.
func LabelTag(label string) string {
	if strings.Contains(label, "damlc inspect") || strings.Contains(strings.ToLower(label), "debug") {
		return "debug-info"
	}
	return "text match"
}

// labelDisplay drops the noise from a label for inline display.
// Ports _loc_label_display.
func labelDisplay(label string) string {
	if label == "" || label == "local source" {
		return ""
	}
	return strings.TrimPrefix(label, "damlc inspect: ")
}

// ShortSourcePath renders a path relative to a loaded source root, falling back
// to the base name. Ports _short_source_path.
func ShortSourcePath(loc source.Location, index *source.Index) string {
	if index != nil {
		for _, root := range index.Roots {
			if strings.HasPrefix(loc.Path, root+"/") {
				return loc.Path[len(root)+1:]
			}
		}
	}
	if i := strings.LastIndex(loc.Path, "/"); i >= 0 {
		return loc.Path[i+1:]
	}
	return loc.Path
}

// RenderLocationInline renders a location as a one-line header plus code, used
// by the compact submit-failure view. Ports _render_loc_inline.
//
// The entity hint ("in choice Asset.Withdraw") comes either from the caller --
// derived from the submitted command, needing no tooling -- or from the index,
// which knows where each template and choice is defined.
func RenderLocationInline(loc source.Location, index *source.Index, color Color, entityKind, entityName string) string {
	header := fmt.Sprintf("%s:%d", ShortSourcePath(loc, index), loc.Line)
	tag := LabelTag(loc.Label)
	if entityKind == "" && entityName == "" && index != nil {
		if kind, label, ok := index.EntityContaining(loc.Path, loc.Line); ok {
			entityKind, entityName = kind, label
		}
	}
	if entityKind != "" && entityName != "" {
		header += fmt.Sprintf("  in %s %s   [%s]", entityKind, entityName, tag)
	} else if desc := labelDisplay(loc.Label); desc != "" {
		header += fmt.Sprintf("  %s   [%s]", desc, tag)
	} else {
		header += fmt.Sprintf("   [%s]", tag)
	}

	headerLine := color.Apply(header, "cyan")
	if index == nil {
		return headerLine
	}
	snippet := index.Snippet(loc, 2)
	lines := strings.Split(snippet, "\n")
	if len(lines) <= 1 {
		return headerLine
	}
	return headerLine + "\n" + strings.Join(lines[1:], "\n")
}
