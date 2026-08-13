package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/ledger"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// runCompare compares two committed updates. Ports the update-vs-update branch
// of run_compare; --prepared comparisons are not ported yet.
func runCompare(args []string) int {
	if wantsHelp(args) {
		commandHelp(os.Stdout, "dpm trace compare <a.json> <b.json> [flags]\n  dpm trace compare --prepared <prepared.json> --update <trace.json> [flags]\n  dpm trace compare --prepared <prepared.json> --completion-file <completion.json> [flags]", "Compare prepared transactions, committed updates, or completions.", compareFlags, "--command-id, fetching updates by id")
		return 0
	}
	var (
		targets        []string
		prepared       string
		update         string
		completionFile string
		ledgerURL      string
		scanURL        string
		readAs         []string
		token          string
		tokenFile      string
		configPath     string
		dar            []string
		printJSON      bool
		full           bool
		colorMode      = "auto"
	)
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; arg {
		case "--print-json":
			printJSON = true
		case "--full":
			full = true
		case "--color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --color requires a value")
				return 2
			}
			i++
			colorMode = args[i]
		case "--prepared":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --prepared requires a path")
				return 2
			}
			i++
			prepared = args[i]
		case "--update":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --update requires a value")
				return 2
			}
			i++
			update = args[i]
		case "--completion-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-file requires a path")
				return 2
			}
			i++
			completionFile = args[i]
		case "--submitter", "--participant-url", "--ledger-url":
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
		case "--token-file", "--access-token-file":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a path\n", arg)
				return 2
			}
			i++
			tokenFile = args[i]
		case "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --config requires a path")
				return 2
			}
			i++
			configPath = args[i]
		case "--dar":
			// Recorded only: damlc-inspect verification is not ported.
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --dar requires a path")
				return 2
			}
			i++
			dar = append(dar, args[i])
		case "--command-id":
			fmt.Fprintln(os.Stderr, "error: --command-id needs a ledger connection, which is not ported yet; use python -m dpm_trace.cli")
			return 2
		default:
			targets = append(targets, arg)
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
	if len(readAs) == 0 {
		readAs = config.Strings(nil, cfg, "DPM_TRACE_READ_AS", "readAs", "read_as", "party")
	}
	fetch := traceFetcher{ledgerURL: ledgerURL, scanURL: scanURL, readAs: readAs, token: token, tokenFile: tokenFile}

	if prepared != "" {
		return runComparePrepared(prepared, update, completionFile, printJSON, full, colorMode, fetch)
	}

	if len(targets) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dpm trace compare <update-a> <update-b>")
		return 2
	}

	left, err := fetch.trace(targets[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	right, err := fetch.trace(targets[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.CompareTraces(left, right)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.UpdateComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}

// traceFetcher resolves a compare target: an artifact file if one exists at
// that path, otherwise an update id fetched from a participant.
// Ports fetch_trace_for_compare.
type traceFetcher struct {
	ledgerURL string
	scanURL   string
	readAs    []string
	token     string
	tokenFile string
}

func (f traceFetcher) trace(target string) (*model.Trace, error) {
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		artifact, err := model.LoadTraceArtifact(target)
		if err != nil {
			return nil, err
		}
		return model.TraceFromArtifact(artifact)
	}

	updateID := extractUpdateID(target)
	parties := ledger.ParseParties(f.readAs)
	resolvedToken, err := ledger.ResolveToken(f.token, f.tokenFile)
	if err != nil {
		return nil, err
	}

	var (
		raw       *model.Object
		sourceTag string
		url       string
	)
	switch {
	case f.scanURL != "":
		raw, sourceTag, url, err = ledger.New(f.scanURL, resolvedToken).LoadScanUpdate(updateID)
	case f.ledgerURL != "":
		raw, sourceTag, url, err = ledger.New(f.ledgerURL, resolvedToken).LoadUpdate(updateID, parties)
	default:
		return nil, ledger.ErrNoSource
	}
	if err != nil {
		return nil, err
	}
	return model.NormalizeTrace(raw, sourceTag, url, parties)
}

var updateURLPattern = regexp.MustCompile(`/update/([^/?#]+)`)

// extractUpdateID accepts a bare update id or a CantonScan update URL.
// Ports extract_update_id.
func extractUpdateID(target string) string {
	if match := updateURLPattern.FindStringSubmatch(target); match != nil {
		return match[1]
	}
	return target
}

// runComparePrepared compares a prepared command against a committed update.
func runComparePrepared(preparedPath, update, completionFile string, printJSON, full bool, colorMode string, fetch traceFetcher) int {
	if completionFile != "" {
		return runComparePreparedCompletion(preparedPath, completionFile, printJSON, full, colorMode)
	}
	if update == "" {
		fmt.Fprintln(os.Stderr, "error: --prepared needs --update, --command-id, or --completion-file")
		return 1
	}
	artifact, err := model.LoadPreparedArtifact(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	trace, err := fetch.trace(update)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.ComparePreparedToTrace(artifact, trace)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.PreparedUpdateComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}

// runComparePreparedCompletion compares a prepared command against a captured
// completion. Source diagnostics need internal/source and are not applied.
func runComparePreparedCompletion(preparedPath, completionPath string, printJSON, full bool, colorMode string) int {
	prepared, err := model.LoadPreparedArtifact(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	completion, err := model.LoadCompletion(completionPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	comparison := model.ComparePreparedToCompletion(prepared, completion)
	if printJSON {
		encoded, err := model.Encode(comparison.JSON())
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
	render.PreparedCompletionComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full)
	return 0
}
