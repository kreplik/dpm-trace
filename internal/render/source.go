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
