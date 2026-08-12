// Command dpm-trace is the Go port of the participant-scoped Canton
// transaction visualizer.
//
// This is scaffolding only: it exists so the build, release and CI
// configuration have a target. Subcommands are ported one at a time, each
// verified against the golden harness in tests/golden (see tests/check-golden.py,
// which takes DPM_TRACE_BIN). Until a subcommand is ported it is not accepted
// here -- the Python implementation in src/dpm_trace remains the shipping one.
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

	fmt.Fprintln(os.Stderr, "error: the Go port is scaffolding only; no subcommand is implemented yet")
	fmt.Fprintln(os.Stderr, "use the Python implementation: python -m dpm_trace.cli")
	os.Exit(2)
}
