package testrunner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/source"
)

// Options configure a `dpm trace test` run.
type Options struct {
	Root               string
	Daml               string
	TestPattern        string
	Files              []string
	JUnitOut           string
	MaxSourceLocations int
	KeepArtifacts      bool
}

// Result is a completed run.
type Result struct {
	Cases          []Case
	Command        []string
	TreesAvailable bool
	WorkDir        string
}

// Failed reports whether any case counts against the CI gate. `dpm trace test`
// must exit non-zero when this is true -- that contract is what makes it usable
// as a CI step.
func (r Result) Failed() bool {
	for _, testcase := range r.Cases {
		if testcase.Status != StatusPassed {
			return true
		}
	}
	return false
}

// Command builds the `daml test` invocation. Ports daml_test_command.
func Command(opts Options, junitPath, txnsDir, tableDir string) []string {
	executable := opts.Daml
	if executable == "" {
		executable = "daml"
	}
	name := filepath.Base(executable)

	var command []string
	if name == "damlc" {
		command = []string{executable, "test", "--package-root", opts.Root}
	} else {
		command = []string{executable, "test"}
		if name == "daml" {
			command = append(command, "--no-legacy-assistant-warning")
		}
	}
	command = append(command, "--junit", junitPath)
	if txnsDir != "" {
		command = append(command, "--transactions-output", txnsDir)
	}
	if tableDir != "" {
		command = append(command, "--table-output", tableDir)
	}
	if opts.TestPattern != "" {
		command = append(command, "--test-pattern", opts.TestPattern)
	}
	var files []string
	for _, file := range opts.Files {
		if file != "" {
			files = append(files, file)
		}
	}
	if len(files) > 0 {
		command = append(command, "--files")
		command = append(command, files...)
	}
	return command
}

// ChildEnv builds the environment for spawning daml/damlc.
//
// Two corrections, both load-bearing:
//   - DPM_RESOLUTION_FILE is dropped. When dpm runs this plugin it exports a
//     resolution context scoped to dpm-trace; a child daml would wrongly apply
//     it to the package under test ("Failed to find DPM package resolution").
//   - A UTF-8 locale is forced when the inherited one is not. `daml test`
//     writes box-drawing characters into its transaction HTML, and under a
//     C/POSIX locale (common in CI) damlc aborts with
//     "commitAndReleaseBuffer: invalid argument".
//
// Ports daml_child_env.
func ChildEnv(extra map[string]string) []string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, found := strings.Cut(entry, "="); found {
			env[key] = value
		}
	}
	delete(env, "DPM_RESOLUTION_FILE")

	ctype := env["LC_ALL"]
	if ctype == "" {
		ctype = env["LC_CTYPE"]
	}
	if ctype == "" {
		ctype = env["LANG"]
	}
	if !strings.Contains(strings.ToLower(ctype), "utf") {
		env["LANG"] = "C.UTF-8"
		env["LC_ALL"] = "C.UTF-8"
	}
	for key, value := range extra {
		env[key] = value
	}

	out := make([]string, 0, len(env))
	for key, value := range env {
		out = append(out, key+"="+value)
	}
	return out
}

// Run executes `daml test` and assembles the results. Ports run_test.
func Run(stderr io.Writer, opts Options, index *source.Index) (Result, error) {
	work, err := os.MkdirTemp("", "dpm-trace-test-")
	if err != nil {
		return Result{}, err
	}
	result := Result{WorkDir: work}
	if !opts.KeepArtifacts {
		defer os.RemoveAll(work)
	}

	junitPath := filepath.Join(work, "results.xml")
	txnsDir := filepath.Join(work, "transactions")
	tableDir := filepath.Join(work, "tables")
	env := ChildEnv(map[string]string{"DAML_PACKAGE": opts.Root})

	command := Command(opts, junitPath, txnsDir, tableDir)
	stdout, _ := runCommand(command, opts.Root, env)

	result.TreesAvailable = fileExists(junitPath)
	if !result.TreesAvailable {
		// Belt and braces: the UTF-8 locale in ChildEnv handles the known
		// abort, so this fires only for some other JUnit-generation failure.
		// Logged rather than silent, so a real compile failure is not masked.
		fmt.Fprintln(stderr, "warning: first `daml test` run produced no JUnit; retrying without "+
			"transaction/table output (trees will be unavailable).")
		command = Command(opts, junitPath, "", "")
		stdout, _ = runCommand(command, opts.Root, env)
	}
	result.Command = command

	if !fileExists(junitPath) {
		fmt.Fprint(stderr, stdout)
		return result, fmt.Errorf("'%s' produced no test results; the package may not compile",
			strings.Join(command, " "))
	}

	cases, err := ParseJUnit(junitPath)
	if err != nil {
		return result, err
	}
	summary := ParseSummary(stdout)
	maxLocations := opts.MaxSourceLocations
	if maxLocations <= 0 {
		maxLocations = 6
	}

	for i := range cases {
		testcase := &cases[i]
		if testcase.Status == StatusPassed {
			htmlPath := filepath.Join(txnsDir, "transaction-"+testcase.Name+".html")
			if result.TreesAvailable && fileExists(htmlPath) {
				data, readErr := os.ReadFile(htmlPath)
				if readErr == nil {
					testcase.TransactionsText = TransactionHTMLToText(string(data))
					testcase.Stats = TransactionStats(testcase.TransactionsText)
					testcase.TouchedLocations = TransactionLocations(testcase.TransactionsText)
					continue
				}
			}
			testcase.Stats = summary[testcase.Name]
			continue
		}
		testcase.Diagnostics, testcase.DiagnosticsCapped = FailureLocations(testcase.Message, index, maxLocations)
	}
	result.Cases = cases

	if opts.JUnitOut != "" {
		if err := copyFile(junitPath, opts.JUnitOut); err != nil {
			return result, err
		}
	}
	return result, nil
}

// runCommand captures stdout and stderr together, as run_test does.
func runCommand(command []string, dir string, env []string) (string, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(source, dest string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// ReportJSON renders the dpm-trace/test-report/v0 document. The shape is a
// stability contract for downstream CI consumers. Ports test_report_json.
func ReportJSON(root string, command []string, cases []Case, maxSourceLocations int) map[string]any {
	passed, failed, errored := 0, 0, 0
	for _, testcase := range cases {
		switch testcase.Status {
		case StatusPassed:
			passed++
		case StatusFailed:
			failed++
		case StatusError:
			errored++
		}
	}

	tests := make([]any, 0, len(cases))
	for _, testcase := range cases {
		diagnostics := make([]any, 0, len(testcase.Diagnostics))
		for _, loc := range testcase.Diagnostics {
			diagnostics = append(diagnostics, map[string]any{
				"path": loc.Path, "line": loc.Line, "column": loc.Column, "basis": loc.Label,
			})
		}
		tests = append(tests, map[string]any{
			"name":               testcase.Name,
			"classname":          testcase.Classname,
			"status":             testcase.Status,
			"time":               floatOrNil(testcase.Time),
			"message":            stringOrNil(testcase.Message),
			"stats":              statsJSON(testcase.Stats),
			"touchedLocations":   stringsOrEmpty(testcase.TouchedLocations),
			"diagnostics":        diagnostics,
			"diagnosticsCapped":  testcase.DiagnosticsCapped,
			"maxSourceLocations": maxSourceLocations,
			"transactions":       stringOrNil(testcase.TransactionsText),
		})
	}

	return map[string]any{
		"schema":  "dpm-trace/test-report/v0",
		"package": root,
		"command": command,
		"summary": map[string]any{
			"total":   len(cases),
			"passed":  passed,
			"failed":  failed,
			"errored": errored,
			"ok":      failed == 0 && errored == 0,
		},
		"tests": tests,
	}
}

func floatOrNil(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func stringOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func statsJSON(stats map[string]int) map[string]any {
	out := map[string]any{}
	for key, value := range stats {
		out[key] = value
	}
	return out
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
