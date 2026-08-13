package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/integration"
	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/scaffold"
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
		initMode        bool
		integrationOpts integration.Options
		scaffoldOpts    scaffold.Options
		root            string
		colorMode       = "auto"
		printJSON       bool
		noTrees         bool
		configPath      string
		damlYAML        []string
		darPaths        []string
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
		case "--init":
			initMode = true
			continue
		case "--no-unittests":
			scaffoldOpts.NoUnittests = true
			continue
		case "--no-ci":
			scaffoldOpts.NoCI = true
			continue
		case "--verbose":
			integrationOpts.Verbose = true
			continue
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
		case "--integration":
			integrationOpts.TestDir = value
		case "--canton-jar":
			integrationOpts.CantonJar = value
		case "--lit":
			integrationOpts.Lit = value
		case "--parties":
			integrationOpts.Parties = value
		case "--sequencer-type":
			integrationOpts.SequencerType = value
		case "--canton-timeout":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --canton-timeout must be a number: %q\n", value)
				return 2
			}
			integrationOpts.CantonTimeout = parsed
		case "--itests-dir":
			scaffoldOpts.ITestsDir = value
		case "--unittests-dir":
			scaffoldOpts.UnitTestsDir = value
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
		case "--dar":
			// Recorded for --integration, which uses it instead of building.
			// damlc-inspect verification of failures is still unported.
			darPaths = append(darPaths, value)
		case "--damlc":
			fmt.Fprintf(os.Stderr, "error: --damlc needs damlc inspect, which is not ported yet; use python -m dpm_trace.cli\n")
			return 2
		default:
			fmt.Fprintf(os.Stderr, "error: %q is not supported by dpm trace test yet; use python -m dpm_trace.cli\n", arg)
			return 2
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	darPaths = config.Strings(darPaths, cfg, "DPM_TRACE_DAR", "darPaths", "dar_paths", "dar")
	damlYAML = config.Strings(damlYAML, cfg, "DPM_TRACE_DAML_YAML", "damlYamlPaths", "daml_yaml_paths", "damlYaml", "daml_yaml")

	if root == "" {
		root = "."
	}
	// Path.resolve() in Python follows symlinks, which matters where a temp
	// directory is reached through one: the reported package path must match.
	absolute, resolveErr := resolvePath(root)
	if err = resolveErr; err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	opts.Root = absolute

	if integrationOpts.TestDir != "" {
		integrationOpts.Root = absolute
		integrationOpts.Daml = opts.Daml
		integrationOpts.DAR = firstOrEmpty(darPaths)
		integrationOpts.KeepArtifacts = opts.KeepArtifacts
		// The generated lit.cfg.py builds %dpm as "<python> -m dpm_trace.cli",
		// so the suite drives the Python implementation. Point it at the
		// interpreter and source tree the same way run_integration_tests does;
		// switching %dpm to this binary would change what the suite tests.
		integrationOpts.Python = os.Getenv("DPM_TRACE_IT_PYTHON")
		if integrationOpts.Python == "" {
			integrationOpts.Python = os.Getenv("DPM_TRACE_PYTHON")
		}
		integrationOpts.SrcRoot = os.Getenv("DPM_TRACE_IT_SRC")
		if integrationOpts.CantonTimeout == 0 {
			integrationOpts.CantonTimeout = 180
		}
		code, err := integration.Run(os.Stdout, os.Stderr, integrationOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return code
		}
		return code
	}

	if initMode {
		scaffoldOpts.Root = absolute
		result, err := scaffold.Init(os.Stdout, scaffoldOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
		render.ScaffoldReport(os.Stdout, absolute, result,
			render.ColorFromMode(colorMode, isTTY()),
			orDefaultString(scaffoldOpts.ITestsDir, "itests"),
			orDefaultString(scaffoldOpts.UnitTestsDir, "unittests"))
		return 0
	}

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

func orDefaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// resolvePath makes a path absolute and follows symlinks, matching
// pathlib.Path.expanduser().resolve(). Ports resolve_package_root.
func resolvePath(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// A path that does not exist yet cannot be resolved; Python returns it
	// unchanged in that case too.
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return absolute, nil
	}
	return resolved, nil
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
