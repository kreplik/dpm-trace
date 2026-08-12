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
		targets        []string
		prepared       string
		update         string
		completionFile string
		printJSON      bool
		full           bool
		colorMode      = "auto"
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
		case "--prepared":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --prepared requires a path")
				return 2
			}
			i++
			prepared = args[i]
		case "--update":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --update requires a value")
				return 2
			}
			i++
			update = args[i]
		case "--completion-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-file requires a path")
				return 2
			}
			i++
			completionFile = args[i]
		case "--command-id":
			fmt.Fprintln(os.Stderr, "error: --command-id needs a ledger connection, which is not ported yet; use python -m dpm_trace.cli")
			return 2
		default:
			targets = append(targets, arg)
		}
	}
	if prepared != "" {
		return runComparePrepared(prepared, update, completionFile, printJSON, full, colorMode)
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

// runComparePrepared compares a prepared command against a committed update.
func runComparePrepared(preparedPath, update, completionFile string, printJSON, full bool, colorMode string) int {
	if completionFile != "" {
		return runComparePreparedCompletion(preparedPath, completionFile, printJSON, full, colorMode)
	}
	if update == "" {
		fmt.Fprintln(os.Stderr, "error: --prepared needs --update, --command-id, or --completion-file")
		return 1
	}
	artifact, err := model.LoadPreparedArtifact(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	trace, err := traceFromFile(update)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.ComparePreparedToTrace(artifact, trace)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.PreparedUpdateComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}

// runComparePreparedCompletion compares a prepared command against a captured
// completion. Source diagnostics need internal/source and are not applied.
func runComparePreparedCompletion(preparedPath, completionPath string, printJSON, full bool, colorMode string) int {
	prepared, err := model.LoadPreparedArtifact(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	completion, err := model.LoadCompletion(completionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.ComparePreparedToCompletion(prepared, completion)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.PreparedCompletionComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}
