package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/ledger"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// runTrace handles the bare `dpm trace` command. Only the --completion-file
// path is ported; fetching an update by id needs internal/ledger.
func runTrace(args []string) int {
	if wantsHelp(args) {
		rootHelp(os.Stdout)
		return 0
	}
	var (
		target             string
		completionFile     string
		colorMode          = "auto"
		damlYAML           []string
		maxSourceLocations = 5
		ledgerURL          string
		scanURL            string
		readAs             []string
		token              string
		tokenFile          string
		configPath         string
		exportPath         string
		waitSeconds        float64
		dar                []string
		printJSON          bool
	)
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--completion-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-file requires a path")
				return 2
			}
			i++
			completionFile = args[i]
		case "--color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --color requires a value")
				return 2
			}
			i++
			colorMode = args[i]
		case "--daml-yaml":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --daml-yaml requires a path")
				return 2
			}
			i++
			damlYAML = append(damlYAML, args[i])
		case "--damlc", "--debug-info":
			fmt.Fprintf(os.Stderr, "error: %s needs damlc inspect, which is not ported yet; use python -m dpm_trace.cli\n", arg)
			return 2
		case "--submitter", "--ledger-url":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a URL\n", arg)
				return 2
			}
			i++
			ledgerURL = args[i]
		case "--scan-url":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --scan-url requires a URL")
				return 2
			}
			i++
			scanURL = args[i]
		case "--read-as", "--party":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a party id\n", arg)
				return 2
			}
			i++
			readAs = append(readAs, args[i])
		case "--token":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --token requires a value")
				return 2
			}
			i++
			token = args[i]
		case "--export", "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a path\n", arg)
				return 2
			}
			i++
			exportPath = args[i]
		case "--print-json":
			printJSON = true
		case "--wait":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --wait requires seconds")
				return 2
			}
			i++
			parsed, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --wait must be a number: %q\n", args[i])
				return 2
			}
			waitSeconds = parsed
		case "--dar":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --dar requires a path")
				return 2
			}
			i++
			dar = append(dar, args[i])
		case "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --config requires a path")
				return 2
			}
			i++
			configPath = args[i]
		case "--token-file", "--access-token-file":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a path\n", arg)
				return 2
			}
			i++
			tokenFile = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "error: %q is not ported yet; use python -m dpm_trace.cli\n", arg)
				return 2
			}
			if target != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", arg)
				return 2
			}
			target = arg
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	ledgerURL = config.String(ledgerURL, cfg, "DPM_TRACE_LEDGER_URL", "ledgerUrl", "ledger_url")
	scanURL = config.String(scanURL, cfg, "DPM_TRACE_SCAN_URL", "scanUrl", "scan_url")
	token = config.String(token, cfg, "DPM_TRACE_TOKEN", "token")
	tokenFile = config.String(tokenFile, cfg, "DPM_TRACE_TOKEN_FILE", "tokenFile", "token_file")
	readAs = config.Strings(readAs, cfg, "DPM_TRACE_READ_AS", "readAs", "read_as", "party")
	damlYAML = config.Strings(damlYAML, cfg, "DPM_TRACE_DAML_YAML", "damlYamlPaths", "daml_yaml_paths", "damlYaml", "daml_yaml")

	index := source.NewIndex()
	for _, path := range damlYAML {
		index.LoadDamlYAML(path)
	}
	color := render.ColorFromMode(colorMode, isTTY())

	if completionFile != "" {
		completion, err := model.LoadCompletion(completionFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		render.CompletionTrace(os.Stdout, completion, color, index, maxSourceLocations)
		return 0
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "error: an update id or --completion-file is required")
		return 2
	}

	resolvedToken, err := ledger.ResolveToken(token, tokenFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	var (
		raw       *model.Object
		sourceTag string
		url       string
	)
	fetch := func() error {
		switch {
		case scanURL != "":
			raw, sourceTag, url, err = ledger.New(scanURL, resolvedToken).LoadScanUpdate(target)
		case ledgerURL != "":
			raw, sourceTag, url, err = ledger.New(ledgerURL, resolvedToken).LoadUpdate(target, readAs)
		default:
			return ledger.ErrNoSource
		}
		return err
	}
	// --wait retries while the update is not yet visible, e.g. a second
	// participant still ingesting it. Distinct from the client's retry, which
	// retries a failing request rather than an absent update.
	if err := fetchWithWait(fetch, waitSeconds); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	trace, err := model.NormalizeTrace(raw, sourceTag, url, readAs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	artifact := model.NewTraceArtifact(trace, ledgerURL, scanURL, dar, nil)
	if exportPath != "" {
		encoded, err := model.Encode(artifact)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(exportPath, append(encoded, '\n'), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Printf("wrote trace artifact: %s\n", exportPath)
	}
	if printJSON {
		encoded, err := model.Encode(model.TraceToJSON(trace))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.PrettyTrace(os.Stdout, trace, color)
	return 0
}

// fetchWithWait retries an absent update until the deadline. Ports
// load_update_with_wait.
func fetchWithWait(fetch func() error, waitSeconds float64) error {
	err := fetch()
	if err == nil || waitSeconds <= 0 {
		return err
	}
	deadline := time.Now().Add(time.Duration(waitSeconds * float64(time.Second)))
	for {
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		if err = fetch(); err == nil {
			return nil
		}
	}
}
