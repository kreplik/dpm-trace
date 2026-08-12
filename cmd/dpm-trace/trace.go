package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// hasFlag reports whether a flag appears in args.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// runTrace handles the bare `dpm trace` command. Only the --completion-file
// path is ported; fetching an update by id needs internal/ledger.
func runTrace(args []string) int {
	var (
		completionFile string
		colorMode      = "auto"
	)
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--completion-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-file requires a path")
				return 2
			}
			i++
			completionFile = args[i]
		case "--color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --color requires a value")
				return 2
			}
			i++
			colorMode = args[i]
		case "--daml-yaml", "--dar", "--damlc", "--debug-info":
			fmt.Fprintf(os.Stderr, "error: %s needs source diagnostics, which are not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		default:
			fmt.Fprintf(os.Stderr, "error: %q is not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		}
	}
	if completionFile == "" {
		fmt.Fprintln(os.Stderr, "error: fetching an update by id is not ported yet; use python -m dpm_trace.cli")
		return 2
	}

	completion, err := model.LoadCompletion(completionFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	render.CompletionTrace(os.Stdout, completion, render.ColorFromMode(colorMode, isTTY()))
	return 0
}
