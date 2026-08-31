package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/config"
	"github.com/walnuthq/dpm-trace/internal/ledger"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// runCompare compares two committed updates. Ports the update-vs-update branch
// of run_compare.
func runCompare(args []string) int {
	if wantsHelp(args) {
		commandHelp(os.Stdout, "dpm trace compare <a.json> <b.json> [flags]\n  dpm trace compare <update-id-a> <update-id-b> --submitter <url> --read-as <party> [flags]\n  dpm trace compare --prepared <prepared.json> --update <trace.json> [flags]\n  dpm trace compare --prepared <prepared.json> --command-id <id> --submitter <url> --act-as <party> [flags]\n  dpm trace compare --prepared <prepared.json> --completion-file <completion.json> [flags]", "Compare prepared transactions, committed updates, or completions.", compareFlags, "")
		return 0
	}
	var (
		targets            []string
		prepared           string
		update             string
		completionFile     string
		commandID          string
		actAs              []string
		ledgerURL          string
		scanURL            string
		readAs             []string
		token              string
		tokenFile          string
		configPath         string
		dar                []string
		damlYAML           []string
		sourceRoots        []string
		debugInfo          []string
		damlc              string
		logFile            []string
		printJSON          bool
		full               bool
		colorMode          = "auto"
		sourceMode         = "auto"
		completionUserID   string
		beginExclusive     string
		completionLimit    = 100
		completionTimeout  = 1000
		maxSourceLocations = 5
		waitSeconds        int
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
		case "--act-as":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --act-as requires a party id")
				return 2
			}
			i++
			party, ok := partyFlag(arg, args[i])
			if !ok {
				return 2
			}
			actAs = append(actAs, party)
		case "--read-as", "--party":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a party id\n", arg)
				return 2
			}
			i++
			party, ok := partyFlag(arg, args[i])
			if !ok {
				return 2
			}
			readAs = append(readAs, party)
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
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --command-id requires a value")
				return 2
			}
			i++
			commandID = args[i]
		case "--daml-yaml":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --daml-yaml requires a path")
				return 2
			}
			i++
			damlYAML = append(damlYAML, args[i])
		case "--source-root":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --source-root requires a path")
				return 2
			}
			i++
			sourceRoots = append(sourceRoots, args[i])
		case "--debug-info":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --debug-info requires a path")
				return 2
			}
			i++
			debugInfo = append(debugInfo, args[i])
		case "--damlc":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --damlc requires a value")
				return 2
			}
			i++
			damlc = args[i]
		case "--log-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --log-file requires a path")
				return 2
			}
			i++
			logFile = append(logFile, args[i])
		case "--completion-user-id":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-user-id requires a value")
				return 2
			}
			i++
			completionUserID = args[i]
		case "--begin-exclusive":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --begin-exclusive requires a value")
				return 2
			}
			i++
			// int("") raises; an explicitly empty value is a mistake, not zero.
			if strings.TrimSpace(args[i]) == "" {
				fmt.Fprintln(os.Stderr, "error: --begin-exclusive must be an integer offset")
				return 2
			}
			beginExclusive = args[i]
		case "--completion-limit":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-limit requires a value")
				return 2
			}
			i++
			if n, err := strconv.Atoi(args[i]); err == nil {
				completionLimit = n
			}
		case "--completion-timeout-ms":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-timeout-ms requires a value")
				return 2
			}
			i++
			if n, err := strconv.Atoi(args[i]); err == nil {
				completionTimeout = n
			}
		case "--max-source-locations":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --max-source-locations requires a value")
				return 2
			}
			i++
			if n, err := strconv.Atoi(args[i]); err == nil {
				maxSourceLocations = n
			}
		case "--wait":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --wait requires a value")
				return 2
			}
			i++
			if n, err := strconv.Atoi(args[i]); err == nil {
				waitSeconds = n
			}
		case "--source":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --source requires a value")
				return 2
			}
			i++
			switch args[i] {
			case "auto", "scan", "ledger":
				sourceMode = args[i]
			default:
				fmt.Fprintf(os.Stderr, "error: --source must be auto, scan or ledger, got %q\n", args[i])
				return 2
			}
		case "-v", "--verbose":
			// Accepted for parity; compare has no extra output to gate on it.
		default:
			// Without this guard an unknown flag becomes a positional and is
			// fetched as an update id, so a typo reports "update not found".
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "error: unknown flag %q\n", arg)
				return 2
			}
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
	// A source index so a prepared-vs-completion comparison can resolve the
	// rejection to source, the way `trace --completion-file` does.
	index := source.NewIndex()
	for _, path := range debugInfo {
		index.LoadDebugInfo(path)
	}
	for _, path := range damlYAML {
		index.LoadDamlYAML(path)
	}
	for _, root := range sourceRoots {
		index.LoadSourceRoot(root)
	}
	for _, path := range dar {
		index.LoadDARInspect(path, orDefaultString(damlc, "daml"))
	}

	fetch := traceFetcher{
		ledgerURL: ledgerURL, scanURL: scanURL, readAs: readAs,
		token: token, tokenFile: tokenFile,
		sourceMode: sourceMode, waitSeconds: waitSeconds,
	}
	lookup := completionLookup{
		ledgerURL: ledgerURL, actAs: actAs, readAs: readAs,
		token: token, tokenFile: tokenFile, commandID: commandID,
		userID: completionUserID, beginExclusive: beginExclusive,
		limit: completionLimit, timeoutMs: completionTimeout,
		logFile: logFile,
	}

	if prepared != "" {
		return runComparePrepared(prepared, update, completionFile, printJSON, full, colorMode, fetch, lookup, index, maxSourceLocations)
	}

	if len(targets) != 2 {
		// Matches cli.py verbatim, including the "error:" prefix and exit 1:
		// run_compare reports this through the same path as any other failure.
		fmt.Fprintln(os.Stderr, "error: usage: dpm trace compare <update-a> <update-b> or --prepared prepared.json --update <update-id>")
		return 1
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
	ledgerURL   string
	scanURL     string
	readAs      []string
	token       string
	tokenFile   string
	sourceMode  string
	waitSeconds int
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
func runComparePrepared(preparedPath, update, completionFile string, printJSON, full bool, colorMode string, fetch traceFetcher, lookup completionLookup, index *source.Index, maxSourceLocations int) int {
	// --update wins when both are given, as run_compare checks args.update
	// first. Reversing it silently runs a different comparison.
	if update == "" && (completionFile != "" || lookup.commandID != "") {
		return runComparePreparedCompletion(preparedPath, completionFile, printJSON, full, colorMode, lookup, index, maxSourceLocations)
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
func runComparePreparedCompletion(preparedPath, completionPath string, printJSON, full bool, colorMode string, lookup completionLookup, index *source.Index, maxSourceLocations int) int {
	prepared, err := model.LoadPreparedArtifact(preparedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	var completion *model.Completion
	if completionPath != "" {
		completion, err = model.LoadCompletion(completionPath)
	} else {
		completion, err = lookup.fetch()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	completion.Raw = model.AttachLogMatches(completion.Raw, lookup.logFile)

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
	render.PreparedCompletionComparison(os.Stdout, comparison, render.ColorFromMode(colorMode, isTTY()), !full, index, maxSourceLocations)
	return 0
}

// completionLookup resolves --command-id against a participant, for the
// prepared-vs-completion comparison.
type completionLookup struct {
	ledgerURL      string
	actAs          []string
	readAs         []string
	token          string
	tokenFile      string
	commandID      string
	userID         string
	beginExclusive string
	limit          int
	timeoutMs      int
	logFile        []string
}

func (l completionLookup) fetch() (*model.Completion, error) {
	resolvedToken, err := ledger.ResolveToken(l.token, l.tokenFile)
	if err != nil {
		return nil, err
	}
	parties := make([]string, 0, len(l.actAs)+len(l.readAs))
	parties = append(parties, l.actAs...)
	parties = append(parties, l.readAs...)
	return ledger.New(l.ledgerURL, resolvedToken).FetchCompletion(ledger.CompletionLookup{
		CommandID: l.commandID,
		Parties:   ledger.ParseParties(parties),
		Limit:     100,
		TimeoutMs: 1000,
	})
}
