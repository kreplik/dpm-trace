// Command dpm-trace inspects participant-scoped Canton transactions.
//
// Output is verified byte-for-byte by the golden harness in tests/golden
// (tests/check-golden.py, which locates the binary through DPM_TRACE_BIN).
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

	if len(args) == 0 || (len(args) == 1 && (args[0] == "-h" || args[0] == "--help")) {
		rootHelp(os.Stdout)
		return
	}

	// --version is an addition: cli.py has no version flag, but a standalone
	// binary needs one and goreleaser injects the values. A bare `version` word
	// is deliberately NOT an alias -- cli.py would treat it as an update id, and
	// shadowing a positional argument is a behavior change.
	if len(args) == 1 && args[0] == "--version" {
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
