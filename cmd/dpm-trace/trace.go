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
	"github.com/walnuthq/dpm-trace/internal/visualizer"
)

// runTrace handles the bare `dpm trace` command: an update id, a captured
// completion, or a command id to look one up by.
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
		commandID          string
		actAs              []string
		completionUserID   string
		beginExclusive     string
		completionLimit    = 100
		completionTimeout  = 1000
		logFile            []string
		sourceRoot         []string
		damlc              string
		debugInfo          []string
		sourceMode         = "auto"
		explainAPIs        bool
		exportPath         string
		waitSeconds        float64
		dar                []string
		printJSON          bool
		visualize          bool
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
			switch args[i] {
			case "auto", "always", "never":
				colorMode = args[i]
			default:
				fmt.Fprintf(os.Stderr, "error: --color must be auto, always or never, got %q\n", args[i])
				return 2
			}
		case "--daml-yaml":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --daml-yaml requires a path")
				return 2
			}
			i++
			damlYAML = append(damlYAML, args[i])
		case "--damlc":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --damlc requires a path")
				return 2
			}
			i++
			damlc = args[i]
		case "--debug-info":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --debug-info requires a path")
				return 2
			}
			i++
			debugInfo = append(debugInfo, args[i])
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
		case "--export", "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "error: %s requires a path\n", arg)
				return 2
			}
			i++
			exportPath = args[i]
		case "--print-json":
			printJSON = true
		case "--visualize":
			visualize = true
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
		case "--explain-apis":
			explainAPIs = true
			continue
		case "--source-root":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --source-root requires a path")
				return 2
			}
			i++
			sourceRoot = append(sourceRoot, args[i])
		case "--source":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --source requires auto, scan or ledger")
				return 2
			}
			i++
			sourceMode = args[i]
			switch sourceMode {
			case "auto", "scan", "ledger":
			default:
				fmt.Fprintf(os.Stderr, "error: --source must be auto, scan or ledger, not %q\n", sourceMode)
				return 2
			}
		case "--max-source-locations":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --max-source-locations requires a number")
				return 2
			}
			i++
			parsed, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --max-source-locations must be a number: %q\n", args[i])
				return 2
			}
			maxSourceLocations = parsed
		case "--log-file":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --log-file requires a path")
				return 2
			}
			i++
			logFile = append(logFile, args[i])
		case "--command-id":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --command-id requires a value")
				return 2
			}
			i++
			commandID = args[i]
		case "--act-as":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --act-as requires a party id")
				return 2
			}
			i++
			actAs = append(actAs, args[i])
		case "--completion-user-id":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-user-id requires a value")
				return 2
			}
			i++
			completionUserID = args[i]
		case "--begin-exclusive":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --begin-exclusive requires an offset")
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
				fmt.Fprintln(os.Stderr, "error: --completion-limit requires a number")
				return 2
			}
			i++
			parsed, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --completion-limit must be a number: %q\n", args[i])
				return 2
			}
			completionLimit = parsed
		case "--completion-timeout-ms":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --completion-timeout-ms requires a number")
				return 2
			}
			i++
			parsed, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: --completion-timeout-ms must be a number: %q\n", args[i])
				return 2
			}
			completionTimeout = parsed
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
				fmt.Fprintf(os.Stderr, "error: unknown flag %q\n", arg)
				return 2
			}
			if target != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", arg)
				return 2
			}
			target = arg
		}
	}
	if explainAPIs {
		fmt.Println(render.ExplainAPIs(ledger.ScanUpdatePath+"{update_id}", ledger.UpdateByIDPath))
		if target == "" && commandID == "" {
			return 0
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
	sourceRoot = config.Strings(sourceRoot, cfg, "DPM_TRACE_SOURCE_ROOT", "sourceRoots", "source_roots", "sourceRoot", "source_root")
	dar = config.Strings(dar, cfg, "DPM_TRACE_DAR", "darPaths", "dar_paths", "dar")
	debugInfo = config.Strings(debugInfo, cfg, "DPM_TRACE_DEBUG_INFO", "debugInfoPaths", "debug_info_paths", "debugInfo", "debug_info")
	damlc = config.String(damlc, cfg, "DPM_TRACE_DAMLC", "damlc")

	index := source.NewIndex()
	// Debug info first, then daml.yaml, then source roots, then DARs. The
	// order is observable: ShortSourcePath strips the first matching root, and
	// the summary prints roots in load order.
	for _, path := range debugInfo {
		index.LoadDebugInfo(path)
	}
	for _, path := range damlYAML {
		index.LoadDamlYAML(path)
	}
	for _, root := range sourceRoot {
		index.LoadSourceRoot(root)
	}
	// damlc inspect confirms a failure literal exists in the compiled package,
	// so matches are labelled "damlc inspect: <module>" rather than the weaker
	// "local source".
	if len(dar) > 0 {
		for _, path := range dar {
			index.LoadDARInspect(path, orDefaultString(damlc, "daml"))
		}
	}
	color := render.ColorFromMode(colorMode, isTTY())

	if completionFile != "" {
		completion, err := model.LoadCompletion(completionFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		completion.Raw = model.AttachLogMatches(completion.Raw, logFile)
		if printJSON {
			return printCompletionJSON(completion)
		}
		render.CompletionTrace(os.Stdout, completion, color, index, maxSourceLocations)
		if visualize {
			fmt.Println("")
			fmt.Println("No transaction tree is available from completion data alone. Use dpm trace <update-id> if the completion includes an update id.")
		}
		return 0
	}

	if commandID != "" {
		lookupToken, err := ledger.ResolveToken(token, tokenFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		// parse_parties over act-as, read-as and party, in that order. --party
		// is folded into readAs when the flags are parsed.
		parties := make([]string, 0, len(actAs)+len(readAs))
		parties = append(parties, actAs...)
		parties = append(parties, readAs...)
		completion, err := ledger.New(ledgerURL, lookupToken).FetchCompletion(ledger.CompletionLookup{
			CommandID:      commandID,
			Parties:        ledger.ParseParties(parties),
			UserID:         completionUserID,
			BeginExclusive: beginExclusive,
			Limit:          completionLimit,
			TimeoutMs:      completionTimeout,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		completion.Raw = model.AttachLogMatches(completion.Raw, logFile)
		if printJSON {
			return printCompletionJSON(completion)
		}
		render.CompletionTrace(os.Stdout, completion, color, index, maxSourceLocations)
		if visualize {
			fmt.Println("")
			fmt.Println("No transaction tree is available from completion data alone. Use dpm trace <update-id> if the completion includes an update id.")
		}
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

	// run_trace normalizes both before fetching: a pasted CantonScan URL is
	// reduced to its update id, and --read-as is comma-split, so
	// `--read-as 'Alice,Bob'` is two parties rather than one bogus one. The
	// parties also label the projection, so NormalizeTrace takes the same list.
	updateID := extractUpdateID(target)
	parties := ledger.ParseParties(readAs)

	var (
		raw       *model.Object
		sourceTag string
		url       string
	)
	fetch := func() error {
		switch {
		case sourceMode == "scan" || (sourceMode == "auto" && scanURL != ""):
			raw, sourceTag, url, err = ledger.New(scanURL, resolvedToken).LoadScanUpdate(updateID)
		case sourceMode == "ledger" || (sourceMode == "auto" && ledgerURL != ""):
			raw, sourceTag, url, err = ledger.New(ledgerURL, resolvedToken).LoadUpdate(updateID, parties)
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

	trace, err := model.NormalizeTrace(raw, sourceTag, url, parties)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	artifact := model.NewTraceArtifact(trace, ledgerURL, scanURL, dar, debugInfo)
	if printJSON {
		encoded, err := model.Encode(model.TraceToJSON(trace))
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}
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
	if visualize {
		stepper := visualizer.New(trace, color, index, os.Stdout)
		stepper.Run(os.Stdin)
		return 0
	}
	render.PrettyTrace(os.Stdout, trace, color, index)
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

// printCompletionJSON emits a completion for --print-json, as run_trace does on
// the --completion-file and --command-id paths.
func printCompletionJSON(completion *model.Completion) int {
	encoded, err := model.Encode(completion.Raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Println(string(encoded))
	return 0
}
