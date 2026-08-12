package main

import (
	"fmt"
	"os"
)

// runPrepare would prepare a command without committing it.
//
// Not ported: it needs internal/ledger. Exiting non-zero rather than
// guessing keeps the Python implementation the only one that answers.
func runPrepare(args []string) int {
	fmt.Fprintln(os.Stderr, "error: dpm trace prepare is not ported yet; use python -m dpm_trace.cli")
	return 2
}
