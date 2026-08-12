package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
	"github.com/walnuthq/dpm-trace/internal/testrunner"
)

// runTest runs a package's Daml Script unit tests. Ports run_test.
//
// It is a CI gate: any non-passing test must exit non-zero.
func runTest(args []string) int {
	if wantsHelp(args) {
		commandHelp(os.Stdout, "dpm trace test <package-dir> [flags]",
			"Run Daml Script unit tests, render each script's transaction tree, and map failures to source.",
			testFlags, "--integration, --init, --dar, --damlc")
		return 0
	}

	opts := testrunner.Options{MaxSourceLocations: 6}
	var (
		root       string
		colorMode  = "auto"
		printJSON  bool
		noTrees    bool
		configPath string
		damlYAML   []string
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--print-json":
			printJSON = true
			continue
		case "--no-trees":
			noTrees = true
			continue
		case "--keep-artifacts":
			opts.KeepArtifacts = true
			continue
		case "--integration", "--init", "--no-unittests", "--no-ci":
			fmt.Fprintf(os.Stderr, "error: %s is not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		}
		if arg[0] != '-' {
			if root != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", arg)
				return 2
			}
			root = arg
			continue
		}
		if i+1 >= len(args) {
			fmt.Fprintf(os.Stderr, "error: %s requires a value\n", arg)
			return 2
		}
		i++
		value := args[i]
		switch arg {
		case "--daml":
			opts.Daml = value
		case "-p", "--test-pattern":
			opts.TestPattern = value
		case "--files":
			opts.Files = append(opts.Files, value)
		case "--junit":
			opts.JUnitOut = value
		case "--daml-yaml":
			damlYAML = append(damlYAML, value)
		case "--color":
			colorMode = value
		case "--config":
			configPath = value
		case "--max-source-locations":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --max-source-locations must be a number: %q\n", value)
				return 2
			}
			opts.MaxSourceLocations = parsed
		case "--dar", "--damlc":
			fmt.Fprintf(os.Stderr, "error: %s needs damlc inspect, which is not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		default:
			fmt.Fprintf(os.Stderr, "error: %q is not supported by dpm trace test yet; use python -m dpm_trace.cli\n", arg)
			return 2
		}
	}

	if _, err := config.Load(configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if root == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	opts.Root = absolute

	damlYAMLPath := filepath.Join(absolute, "daml.yaml")
	if _, err := os.Stat(damlYAMLPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: no daml.yaml found in %s; pass a package directory\n", absolute)
		return 2
	}
	if len(damlYAML) == 0 {
		damlYAML = []string{damlYAMLPath}
	}

	index := source.NewIndex()
	for _, path := range damlYAML {
		index.LoadDamlYAML(path)
	}

	result, err := testrunner.Run(os.Stderr, opts, index)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		return 2
	}

	if printJSON {
		encoded, err := model.Encode(testrunner.ReportJSON(absolute, result.Command, result.Cases, opts.MaxSourceLocations))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
	} else {
		color := render.ColorFromMode(colorMode, isTTY())
		render.TestReport(os.Stdout, absolute, result.Command, result.Cases, color, index, !noTrees)
		if !result.TreesAvailable && !noTrees {
			fmt.Printf("\n%s transaction trees were unavailable in this environment; showing results only.\n",
				color.Apply("note:", "gray"))
		}
	}

	if opts.KeepArtifacts && !printJSON {
		fmt.Printf("\nartifacts kept in: %s\n", result.WorkDir)
	}
	if result.Failed() {
		return 1
	}
	return 0
}

var testFlags = []flagDoc{
	{"--daml PATH", "Toolchain to run: daml, damlc or dpm. Defaults to daml."},
	{"-p, --test-pattern PAT", "Run only tests matching a pattern."},
	{"--files PATH", "Restrict to specific source files. Repeatable."},
	{"--junit PATH", "Also write JUnit XML for CI."},
	{"--print-json", "Print the dpm-trace/test-report/v0 report and exit."},
	{"--no-trees", "Summary and failures only, for compact CI logs."},
	{"--daml-yaml PATH", "daml.yaml for source diagnostics. Defaults to the package's."},
	{"--max-source-locations N", "Maximum diagnostics per failure. Defaults to 6."},
	{"--keep-artifacts", "Keep the temporary run directory."},
	{"--color MODE", "auto, always or never. Defaults to auto."},
	{"-h, --help", "Show this help."},
}
