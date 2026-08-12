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

	if len(args) > 0 {
		switch args[0] {
		case "open":
			os.Exit(runOpen(args[1:]))
		case "compare":
			os.Exit(runCompare(args[1:]))
		case "prepare":
			os.Exit(runPrepare(args[1:]))
		case "submit":
			os.Exit(runSubmit(args[1:]))
		case "test":
			os.Exit(runTest(args[1:]))
		case "install-plugin":
			os.Exit(runInstallPlugin(args[1:]))
		}
	}

	// Anything else is the bare `dpm trace` command: an update id, a captured
	// completion, or flags that configure either. cli.py dispatches the same
	// way -- only the named subcommands are special.
	os.Exit(runTrace(args))
}

func isTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
