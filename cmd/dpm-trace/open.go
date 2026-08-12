package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// runOpen reopens an exported trace artifact. Ports run_open, minus --visualize
// (internal/visualizer) and source diagnostics (internal/source).
func runOpen(args []string) int {
	var (
		path      string
		printJSON bool
		colorMode = "auto"
	)
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--print-json":
			printJSON = true
		case "--color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --color requires a value")
				return 2
			}
			i++
			colorMode = args[i]
		case "--visualize":
			fmt.Fprintln(os.Stderr, "error: --visualize is not ported yet; use python -m dpm_trace.cli")
			return 2
		default:
			if path != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", arg)
				return 2
			}
			path = arg
		}
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "error: an artifact path is required")
		return 2
	}

	artifact, err := model.LoadTraceArtifact(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", errorText(err, path))
		return 1
	}
	trace, err := model.TraceFromArtifact(artifact)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if printJSON {
		encoded, err := model.Encode(artifact)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	fmt.Println(render.TraceArtifactSummary(artifact))
	render.PrettyTrace(os.Stdout, trace, render.ColorFromMode(colorMode, isTTY()))
	return 0
}

// errorText matches the Python message for a missing file, which surfaces the
// OSError rather than a Go-style wrapped path error.
func errorText(err error, path string) string {
	if os.IsNotExist(err) {
		return fmt.Sprintf("[Errno 2] No such file or directory: '%s'", path)
	}
	return err.Error()
}
