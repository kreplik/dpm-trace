package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// runCompare compares two committed updates. Ports the update-vs-update branch
// of run_compare; --prepared comparisons are not ported yet.
func runCompare(args []string) int {
	var (
		targets   []string
		printJSON bool
		full      bool
		colorMode = "auto"
	)
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--print-json":
			printJSON = true
		case "--full":
			full = true
		case "--color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --color requires a value")
				return 2
			}
			i++
			colorMode = args[i]
		case "--prepared", "--update", "--completion-file", "--command-id":
			fmt.Fprintf(os.Stderr, "error: %s comparisons are not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		default:
			targets = append(targets, arg)
		}
	}
	if len(targets) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dpm trace compare <update-a> <update-b>")
		return 2
	}

	left, err := traceFromFile(targets[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	right, err := traceFromFile(targets[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.CompareTraces(left, right)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.UpdateComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}

// traceFromFile reads a committed trace artifact. Fetching by update id needs
// internal/ledger and is not ported yet.
func traceFromFile(target string) (*model.Trace, error) {
	artifact, err := model.LoadTraceArtifact(target)
	if err != nil {
		return nil, err
	}
	return model.TraceFromArtifact(artifact)
}
