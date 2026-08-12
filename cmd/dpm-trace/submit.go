package main

import (
	"fmt"
	"os"
)

// runSubmit would submit-and-wait a command and print the update id.
//
// Not ported: it needs internal/ledger. Exiting non-zero rather than
// guessing keeps the Python implementation the only one that answers.
func runSubmit(args []string) int {
	fmt.Fprintln(os.Stderr, "error: dpm trace submit is not ported yet; use python -m dpm_trace.cli")
	return 2
}
