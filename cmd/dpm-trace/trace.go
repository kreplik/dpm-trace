package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/ledger"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// runTrace handles the bare `dpm trace` command. Only the --completion-file
// path is ported; fetching an update by id needs internal/ledger.
func runTrace(args []string) int {
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
		case "--dar", "--damlc", "--debug-info":
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
	switch {
	case scanURL != "":
		raw, sourceTag, url, err = ledger.New(scanURL, resolvedToken).LoadScanUpdate(target)
	case ledgerURL != "":
		raw, sourceTag, url, err = ledger.New(ledgerURL, resolvedToken).LoadUpdate(target, readAs)
	default:
		fmt.Fprintf(os.Stderr, "error: %v\n", ledger.ErrNoSource)
		return 1
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	trace, err := model.NormalizeTrace(raw, sourceTag, url, readAs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	render.PrettyTrace(os.Stdout, trace, color)
	return 0
}
