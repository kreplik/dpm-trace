package main

import (
	"fmt"
	"os"

	"github.com/walnuthq/dpm-trace/internal/ledger"
	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
)

// runSubmit submits a command and prints the resulting update id.
// Ports run_submit.
//
// A rejected submission has no update id, so the completion view is printed
// instead of a transaction tree. --allow-fail makes that a success exit, which
// is how integration tests capture a rejection.
func runSubmit(args []string) int {
	opts, spec, rc := parseSubmissionFlags("submit", args)
	if rc != 0 {
		return rc
	}

	commands, err := spec.Commands()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	actAs := ledger.ParseParties(opts.actAs)
	if len(actAs) == 0 {
		fmt.Fprintln(os.Stderr, "error: --act-as is required")
		return 1
	}
	readAs := excludeParties(ledger.ParseParties(append(opts.readAs, opts.party...)), actAs)
	if opts.ledgerURL == "" {
		fmt.Fprintln(os.Stderr, "error: --submitter/--participant-url/--ledger-url is required")
		return 1
	}

	commandID := opts.commandID
	if commandID == "" {
		commandID = ledger.GenerateCommandID("dpm-trace-submit-")
	}
	request := map[string]any{
		"commandId": commandID,
		"commands":  commands,
		"actAs":     actAs,
		"readAs":    readAs,
	}
	if userID := ledger.UserID(opts.userID, opts.ledgerURL, opts.token, opts.tokenFile); userID != "" {
		request["userId"] = userID
	}

	token, err := ledger.ResolveToken(opts.token, opts.tokenFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	ok, response, _, err := ledger.New(opts.ledgerURL, token).SubmitAndWait(request)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if !ok {
		if opts.printJSON {
			encoded, encodeErr := model.Encode(response)
			if encodeErr != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", encodeErr)
				return 1
			}
			fmt.Println(string(encoded))
		} else {
			completion := &model.Completion{Raw: model.NormalizeCompletion(response)}
			if completion.String("commandId", "command_id") == "" {
				completion.Raw.Set("commandId", commandID)
			}
			if opts.full {
				render.CompletionTrace(os.Stdout, completion,
					render.ColorFromMode(opts.colorMode, isTTY()), nil, 5)
			} else {
				render.SubmitFailure(os.Stdout, completion, request,
					render.ColorFromMode(opts.colorMode, isTTY()), nil, 5)
			}
		}
		if opts.allowFail {
			return 0
		}
		return 1
	}

	if opts.printJSON {
		encoded, err := model.Encode(response)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println(string(encoded))
		return 0
	}

	updateID := model.ObjectString(response, "updateId")
	if updateID == "" {
		fmt.Fprintln(os.Stderr, "error: submit-and-wait returned no updateId")
		return 1
	}
	fmt.Println(updateID)
	return 0
}
