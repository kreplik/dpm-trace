package main

import (
	"fmt"
	"os"
)

// runTest would run Daml Script unit tests, or an lit suite with --integration.
//
// Not ported: it needs internal/testrunner and internal/integration. Exiting non-zero rather than
// guessing keeps the Python implementation the only one that answers.
func runTest(args []string) int {
	fmt.Fprintln(os.Stderr, "error: dpm trace test is not ported yet; use python -m dpm_trace.cli")
	return 2
}
