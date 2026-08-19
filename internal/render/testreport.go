package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/scaffold"
	"github.com/walnuthq/dpm-trace/internal/source"
	"github.com/walnuthq/dpm-trace/internal/testrunner"
)

// TestReport writes the `dpm trace test` report. Ports print_test_report.
func TestReport(w io.Writer, root string, command []string, cases []testrunner.Case,
	color Color, index *source.Index, showTrees bool) {

	passed, failed := 0, 0
	for _, testcase := range cases {
		switch {
		case testcase.Status == testrunner.StatusPassed:
			passed++
		case testcase.Failed():
			failed++
		}
	}

	fmt.Fprintln(w, color.Apply("DPM trace test", "bold"))
	fmt.Fprintf(w, "  package:  %s\n", root)
	fmt.Fprintf(w, "  command:  %s\n", strings.Join(DisplayCommand(command), " "))
	fmt.Fprintf(w, "  result:   %s\n", testResultBanner(passed, failed, len(cases), color))
	fmt.Fprintln(w)

	fmt.Fprintln(w, color.Apply("Results", "cyan", "bold"))
	width := 0
	for _, testcase := range cases {
		if len(testcase.Name) > width {
			width = len(testcase.Name)
		}
	}
	for _, testcase := range cases {
		line := fmt.Sprintf("  %s  %-*s", testStatusIcon(testcase.Status, color), width, testcase.Name)
		if testcase.Status == testrunner.StatusPassed {
			line += "  " + testStatsText(testcase.Stats, color)
		}
		fmt.Fprintln(w, line)

		if !testcase.Failed() {
			continue
		}
		message := testcase.Message
		if message == "" {
			message = "-"
		}
		fmt.Fprintf(w, "        %s %s\n", color.Apply("message:", "gray"), message)
		for _, loc := range testcase.Diagnostics {
			fmt.Fprintln(w, indentBy(SourceDiagnostic(loc, index, color), "        "))
		}
		switch {
		case testcase.DiagnosticsCapped:
			fmt.Fprintf(w, "        %s (capped at %d locations; pass --max-source-locations to raise)\n",
				color.Apply("source:", "gray"), len(testcase.Diagnostics))
		case len(testcase.Diagnostics) == 0:
			fmt.Fprintf(w, "        %s no matching local source found\n", color.Apply("source:", "gray"))
		}
	}

	if !showTrees {
		return
	}
	var trees []testrunner.Case
	for _, testcase := range cases {
		if testcase.TransactionsText != "" {
			trees = append(trees, testcase)
		}
	}
	if len(trees) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.Apply("Transaction trees", "cyan", "bold"))
	for _, testcase := range trees {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  "+color.Apply("── "+testcase.Name+" ──", "magenta", "bold"))
		fmt.Fprintln(w, indentBy(testcase.TransactionsText, "  "))
	}
}

func testResultBanner(passed, failed, total int, color Color) string {
	if failed > 0 {
		return color.Apply(fmt.Sprintf("%d failed, %d passed, %d total", failed, passed, total), "red", "bold")
	}
	return color.Apply(fmt.Sprintf("all %d passed (%d total)", passed, total), "green", "bold")
}

func testStatusIcon(status string, color Color) string {
	switch status {
	case testrunner.StatusPassed:
		return color.Apply("PASS", "green", "bold")
	case testrunner.StatusSkipped:
		return color.Apply("SKIP", "gray", "bold")
	}
	return color.Apply("FAIL", "red", "bold")
}

func testStatsText(stats map[string]int, color Color) string {
	if stats == nil || stats["transactions"] == 0 {
		return color.Apply("no transactions", "gray")
	}
	parts := []string{fmt.Sprintf("%d tx", stats["transactions"])}
	if stats["creates"] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("+%d create", stats["creates"]), "green"))
	}
	if stats["exercises"] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf(">%d exercise", stats["exercises"]), "yellow"))
	}
	if stats["archives"] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("x%d archive", stats["archives"]), "red"))
	}
	if stats["expectedFailures"] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("!%d expected-fail", stats["expectedFailures"]), "blue"))
	}
	if _, hasCreates := stats["creates"]; !hasCreates && stats["activeContracts"] > 0 {
		parts = append(parts, color.Apply(fmt.Sprintf("%d active", stats["activeContracts"]), "gray"))
	}
	return strings.Join(parts, "  ")
}

// DisplayCommand strips output-file flags so the reported command is the one a
// user would run. Ports display_command.
func DisplayCommand(command []string) []string {
	if len(command) == 0 {
		return command
	}
	cleaned := []string{baseName(command[0])}
	skipNext := false
	for _, token := range command[1:] {
		if skipNext {
			skipNext = false
			continue
		}
		switch token {
		case "--junit", "--transactions-output", "--table-output", "--package-root":
			skipNext = true
			continue
		case "--no-legacy-assistant-warning":
			continue
		}
		cleaned = append(cleaned, token)
	}
	return cleaned
}

func baseName(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

// indentBy prefixes every non-empty line, matching textwrap.indent.
func indentBy(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// ScaffoldReport writes the summary of a `dpm trace test --init` run.
// Ports the tail of run_init.
func ScaffoldReport(w io.Writer, root string, result scaffold.Result, color Color, itestsDir, unitTestsDir string) {
	fmt.Fprintln(w, color.Apply("dpm trace test --init", "bold"))
	fmt.Fprintf(w, "  package: %s\n", root)
	for _, path := range result.Created {
		fmt.Fprintf(w, "  %s  %s\n", color.Apply("created", "green"), path)
	}
	for _, path := range result.Kept {
		fmt.Fprintf(w, "  %s     %s (already exists)\n", color.Apply("kept", "gray"), path)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintf(w, "  unit tests:        %s (self-contained package; or `dpm trace test .` for this package's scripts)\n",
		color.Apply("dpm trace test "+unitTestsDir, "cyan"))
	fmt.Fprintf(w, "  integration tests: %s\n",
		color.Apply("dpm trace test . --integration "+itestsDir+" --canton-jar <canton.jar>", "cyan"))
}
