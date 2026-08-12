// Command dpm-trace is the Go port of the participant-scoped Canton
// transaction visualizer.
//
// Subcommands are ported one at a time and each is verified against the golden
// harness in tests/golden (tests/check-golden.py, which takes DPM_TRACE_BIN).
// Anything not yet ported exits 2 rather than guessing: the Python
// implementation in src/dpm_trace remains the shipping one.
package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// Set via -ldflags at release time; see .goreleaser.yaml.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args := os.Args[1:]
	// The DPM plugin invokes components without the "trace" command name;
	// accept an explicit leading "trace" too, matching main() in cli.py.
	if len(args) > 0 && args[0] == "trace" {
		args = args[1:]
	}

	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Printf("dpm-trace %s (%s, built %s)\n", version, commit, date)
		return
	}

	if len(args) > 0 && args[0] == "open" {
		os.Exit(runOpen(args[1:]))
	}

	if len(args) > 0 && args[0] == "compare" {
		os.Exit(runCompare(args[1:]))
	}

	fmt.Fprintln(os.Stderr, "error: this subcommand is not ported yet; use python -m dpm_trace.cli")
	os.Exit(2)
}

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

func isTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
