package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
	"github.com/walnuthq/dpm-trace/internal/visualizer"
)

// runOpen reopens an exported trace artifact. Ports run_open.
func runOpen(args []string) int {
	if wantsHelp(args) {
		commandHelp(os.Stdout, "dpm trace open <artifact.json> [flags]", "Reopen an exported trace artifact.", openFlags, "")
		return 0
	}
	var (
		path      string
		printJSON bool
		visualize bool
		debugInfo []string
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
			visualize = true
		case "--debug-info":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --debug-info requires a path")
				return 2
			}
			i++
			debugInfo = append(debugInfo, args[i])
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

	// An exported artifact can name its own debug-info files; those are used
	// alongside anything passed on the command line.
	index := source.NewIndex()
	if packages, ok := model.ObjectField(artifact, "packages"); ok {
		debugInfo = append(debugInfo, model.ObjectStrings(packages, "debugInfoPaths")...)
	}
	for _, path := range dedupe(debugInfo) {
		index.LoadDebugInfo(path)
	}

	color := render.ColorFromMode(colorMode, isTTY())
	// The summary is printed for both paths, as run_open does.
	fmt.Println(render.TraceArtifactSummary(artifact))
	if visualize {
		stepper := visualizer.New(trace, color, index, os.Stdout)
		stepper.Bundle = artifact
		stepper.Run(os.Stdin)
		return 0
	}
	render.PrettyTrace(os.Stdout, trace, color, index)
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

// dedupe preserves order while dropping repeats and empties, matching unique().
func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
