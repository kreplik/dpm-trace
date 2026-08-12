package main

import (
	"fmt"
	"os"
)

// runInstallPlugin would register the binary as a DPM component.
//
// Not ported: it needs internal/plugin. Exiting non-zero rather than
// guessing keeps the Python implementation the only one that answers.
func runInstallPlugin(args []string) int {
	fmt.Fprintln(os.Stderr, "error: dpm trace install-plugin is not ported yet; use python -m dpm_trace.cli")
	return 2
}
