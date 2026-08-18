package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// CompletionTrace writes the completion view of a submission.
// Ports print_completion_trace's non-compact path; the compact path is
// SubmitFailure, used by `dpm trace submit`.
func CompletionTrace(w io.Writer, c *model.Completion, color Color, index *source.Index, maxSourceLocations int) {
	statusCode, message := c.StatusFields()
	updateID := c.String("updateId", "update_id")
	committed := updateID != ""
	failed := c.Failed()

	lookup, hasLookup := model.ObjectField(c.Raw, "lookup")

	commandID := c.String("commandId", "command_id")
	if commandID == "" && hasLookup {
		commandID = model.ObjectString(lookup, "commandId")
	}

	fmt.Fprintln(w, color.Apply("DPM trace completion", "bold"))
	fmt.Fprintf(w, "  result:     %s\n", completionResult(committed, failed, color))
	fmt.Fprintf(w, "  command id: %s\n", orDash(commandID))
	fmt.Fprintf(w, "  submission: %s\n", orDash(c.String("submissionId", "submission_id")))
	fmt.Fprintf(w, "  update id:  %s\n", Short(orDashValue(updateID), 80))
	fmt.Fprintf(w, "  offset:     %s\n", orDash(scalarText(c.Get("offset"))))
	fmt.Fprintf(w, "  status:     %s\n", orDashIfNil(statusCode))
	fmt.Fprintf(w, "  message:    %s\n", orDash(scalarText(message)))
	if hasLookup {
		fmt.Fprintf(w, "  parties:    %s\n", partyListSummary(model.ObjectStrings(lookup, "parties")))
		fmt.Fprintf(w, "  source:     %s\n", orDash(model.ObjectString(c.Raw, "source")))
	}
	if !committed {
		fmt.Fprintln(w, "  trace:      no committed transaction tree is available for this completion")
	}
	locations, capped := CompletionSourceDiagnostics(c, index, maxSourceLocations)
	PrintSourceDiagnostics(w, locations, capped, index, color)
	printLogMatches(w, c.Get("logMatches"), color)
}

// PreparedCompletionComparison writes a prepared-vs-completion comparison.
// Ports print_prepared_completion_comparison, minus source diagnostics.
func PreparedCompletionComparison(w io.Writer, c *model.CompletionComparison, color Color, compact bool, index *source.Index, maxSourceLocations int) {
	completion := c.Completion
	committed := c.CommittedUpdateAvailable
	failed := !committed && isFailingStatus(c.StatusCode)
	statusText := scalarText(c.StatusCode)

	if compact {
		switch {
		case failed:
			label := statusText
			if label == "" {
				label = "FAILED"
			}
			fmt.Fprintf(w, "%s completion failed — %s     kind: %s\n", color.Apply("✗", "red", "bold"), label, c.Kind)
		case committed:
			fmt.Fprintf(w, "%s completion committed     kind: %s\n", color.Apply("✓", "green", "bold"), c.Kind)
		default:
			fmt.Fprintf(w, "%s completion pending     kind: %s\n", color.Apply("?", "yellow", "bold"), c.Kind)
		}
		fmt.Fprintln(w)

		fmt.Fprintf(w, "  A  command %s   (prepared)\n", Short(orDashValue(c.Left.CommandID), 20))
		completionID := Short(orDashValue(completion.String("commandId", "command_id")), 20)
		offset := orDashFalsy(completion.Get("offset"))
		if committed {
			updateID := Short(completion.String("updateId", "update_id"), 12)
			fmt.Fprintf(w, "  B  completion %s   update %s   offset %s\n", completionID, updateID, offset)
		} else {
			fmt.Fprintf(w, "  B  completion %s   offset %s   %s\n", completionID, offset, orDashIfNil(c.StatusCode))
		}
		fmt.Fprintln(w)

		if messageText := scalarText(c.Message); messageText != "" && failed {
			fmt.Fprintf(w, "  %s\n", color.Apply(pythonRepr(messageText), "yellow"))
			fmt.Fprintln(w)
		}

		var ctxParts []string
		if submission := completion.String("submissionId", "submission_id"); submission != "" {
			ctxParts = append(ctxParts, "submission "+Short(submission, 20))
		}
		if sync := scalarText(completion.Get("synchronizerTime", "synchronizer_time")); sync != "" {
			ctxParts = append(ctxParts, "sync "+sync)
		}
		if len(ctxParts) > 0 {
			fmt.Fprintf(w, "  Context   %s\n", strings.Join(ctxParts, "   "))
		}
		return
	}

	fmt.Fprintln(w, color.Apply("DPM trace comparison", "bold"))
	fmt.Fprintln(w, "  kind:   "+c.Kind)
	fmt.Fprintf(w, "  result: %s\n", completionResult(committed, failed, color))
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.Apply("Prepared command", "cyan", "bold"))
	fmt.Fprintf(w, "  command id: %s\n", orDash(c.Left.CommandID))
	fmt.Fprintf(w, "  commands:   %d\n", c.Left.Commands)
	fmt.Fprintln(w)

	fmt.Fprintln(w, color.Apply("Completion", "cyan", "bold"))
	marker := ""
	if c.CommandIDMatch {
		marker = " " + color.Apply("match", "green")
	}
	fmt.Fprintf(w, "  command id: %s%s\n", orDash(completion.String("commandId", "command_id")), marker)
	fmt.Fprintf(w, "  submission: %s\n", orDash(completion.String("submissionId", "submission_id")))
	fmt.Fprintf(w, "  update id:  %s\n", Short(orDashValue(completion.String("updateId", "update_id")), 80))
	fmt.Fprintf(w, "  offset:     %s\n", orDashFalsy(completion.Get("offset")))
	fmt.Fprintf(w, "  status:     %s\n", orDashIfNil(c.StatusCode))
	fmt.Fprintf(w, "  message:    %s\n", orDash(scalarText(c.Message)))

	traceContext := completion.Get("traceContext")
	if obj, ok := traceContext.(*model.Object); ok {
		keys := obj.Keys()
		sort.Strings(keys)
		for _, key := range keys {
			value, _ := obj.Get(key)
			if text := scalarText(value); text != "" {
				fmt.Fprintf(w, "  %s:  %s\n", key, Short(text, 80))
			}
		}
	} else if traceContext != nil {
		fmt.Fprintf(w, "  trace context: %s\n", compactValue(traceContext))
	}
	if sync := completion.Get("synchronizerTime", "synchronizer_time"); sync != nil {
		fmt.Fprintf(w, "  sync time:  %s\n", scalarText(sync))
	}
	locations, capped := CompletionSourceDiagnostics(c.Completion, index, maxSourceLocations)
	PrintSourceDiagnostics(w, locations, capped, index, color)
	printLogMatches(w, completion.Get("logMatches"), color)
}

func printLogMatches(w io.Writer, raw any, color Color) {
	matches, ok := raw.([]any)
	if !ok || len(matches) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.Apply("Log matches", "cyan", "bold"))
	for i, item := range matches {
		if i >= 8 {
			break
		}
		match, isObject := item.(*model.Object)
		if !isObject {
			continue
		}
		if errText := model.ObjectString(match, "error"); errText != "" {
			fmt.Fprintf(w, "  %s: %s\n", model.ObjectString(match, "file"), errText)
			continue
		}
		fmt.Fprintf(w, "  %s:%s: %s\n",
			model.ObjectString(match, "file"),
			model.ObjectString(match, "line"),
			model.ObjectString(match, "text"))
	}
	if len(matches) > 8 {
		fmt.Fprintf(w, "  ... %d more\n", len(matches)-8)
	}
}

func completionResult(committed, failed bool, color Color) string {
	if committed {
		return color.Apply("completion committed", "green", "bold")
	}
	if failed {
		return color.Apply("completion failed", "red", "bold")
	}
	return color.Apply("completion status available", "yellow", "bold")
}

func isFailingStatus(code any) bool {
	switch scalarText(code) {
	case "", "OK", "0":
		return false
	}
	return true
}

// scalarText renders a decoded JSON scalar the way Python's str() would.
// orDashFalsy renders a value the way Python's `x or '-'` does: a zero offset
// or an empty string both show as "-", because both mean "not reported".
func orDashFalsy(value any) string {
	text := scalarText(value)
	if text == "" || text == "0" {
		return "-"
	}
	return text
}

func scalarText(value any) string {
	if value == nil {
		return ""
	}
	if obj, ok := value.(*model.Object); ok {
		return compactValue(obj)
	}
	return fmt.Sprint(value)
}

func orDashIfNil(value any) string {
	if value == nil {
		return "-"
	}
	return scalarText(value)
}

// pythonRepr renders a string the way Python's repr() does: single quotes
// unless the text contains one, with backslashes escaped.
func pythonRepr(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	// repr escapes control characters; without this a multi-line failure
	// message breaks the single-line compact layout.
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + escaped + `"`
	}
	escaped = strings.ReplaceAll(escaped, "'", `\'`)
	return "'" + escaped + "'"
}

// SubmitFailure writes the compact failure view `dpm trace submit` prints when
// a submission is rejected. Ports print_completion_trace's compact path.
//
// It leads with what you sent rather than with completion metadata, because a
// rejected submission has no transaction to inspect -- the command and the
// error are the whole story.
func SubmitFailure(w io.Writer, c *model.Completion, request map[string]any, color Color, index *source.Index, maxSourceLocations int) {
	statusCode, message := c.StatusFields()
	commandID := c.String("commandId", "command_id")
	if commandID == "" {
		commandID = "-"
	}
	offset := orDash(scalarText(c.Get("offset")))

	statusText := scalarText(statusCode)
	if statusText == "" {
		statusText = "FAILED"
	}
	fmt.Fprintf(w, "%s  %s\n", color.Apply("✗ submission failed", "red", "bold"), statusText)
	if messageText := scalarText(message); messageText != "" {
		fmt.Fprintf(w, "  %s\n", color.Apply(pythonRepr(messageText), "yellow"))
	}
	fmt.Fprintln(w)

	const col = 52
	fmt.Fprintf(w, "  %-*s (what you sent)\n", col, "Command")
	commands, _ := request["commands"].([]any)
	for i, command := range commands {
		if i >= 3 {
			break
		}
		fmt.Fprintf(w, "    %s\n", formatCommandSummary(command))
	}
	actAs, _ := request["actAs"].([]string)
	limit := actAs
	if len(limit) > 2 {
		limit = limit[:2]
	}
	rendered := make([]string, 0, len(limit))
	for _, party := range limit {
		rendered = append(rendered, ShortParty(party))
	}
	fmt.Fprintf(w, "    act-as %s    command id %s    offset %s\n",
		strings.Join(rendered, ", "), Short(commandID, 20), offset)
	fmt.Fprintln(w)

	locations, capped := CompletionSourceDiagnostics(c, index, maxSourceLocations)
	if len(locations) > 0 {
		// The submitted command names the choice, so the hint needs no tooling.
		entityKind, entityName := EntityFromRequest(request)
		fmt.Fprintf(w, "  %-*s (best match first)\n", col, "Where")
		fmt.Fprintln(w, IndentText(RenderLocationInline(locations[0], index, color, entityKind, entityName)))
		rest := locations[1:]
		if len(rest) > 0 || capped {
			shown := rest
			if len(shown) > 2 {
				shown = shown[:2]
			}
			also := make([]string, 0, len(shown))
			for _, loc := range shown {
				also = append(also, fmt.Sprintf("%s:%d [%s]", ShortSourcePath(loc, index), loc.Line, LabelTag(loc.Label)))
			}
			extra := len(rest) - len(shown)
			if capped {
				extra++
			}
			suffix := "  (--full to expand)"
			if extra > 0 {
				suffix = fmt.Sprintf("  (+%d more; --full to expand)", extra)
			}
			fmt.Fprintf(w, "    also: %s%s\n", strings.Join(also, ", "), suffix)
		}
		fmt.Fprintln(w)
	}

	// A log match is how a rejected submission is tied to operator logs; the
	// term that matched is named so the correlation is auditable.
	if matches, ok := c.Get("logMatches").([]any); ok && len(matches) > 0 {
		if first, isObject := matches[0].(*model.Object); isObject {
			fileRef := fmt.Sprintf("%s:%s", model.ObjectString(first, "file"), model.ObjectString(first, "line"))
			term := ""
			if matchedBy := logMatchedBy(c); matchedBy != "" {
				term = "  matched by " + matchedBy
			}
			more := ""
			if len(matches) > 1 {
				more = "  (--full for context)"
			}
			fmt.Fprintf(w, "  Logs   %s%s%s\n", fileRef, term, more)
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintln(w, "  Next")
	fmt.Fprintf(w, "    dpm trace compare --command-id %s --prepared <prepared.json>\n", commandID)
}

// formatCommandSummary renders one submitted command. Ports _format_command_summary.
func formatCommandSummary(command any) string {
	obj, ok := command.(map[string]any)
	if !ok {
		return fmt.Sprint(command)
	}
	if body, ok := commandBody(obj, "ExerciseCommand", "exerciseCommand"); ok {
		template := orQuestion(ShortTemplate(fmt.Sprint(body["templateId"])))
		choice := orQuestion(fmt.Sprint(body["choice"]))
		contract := Short(fmt.Sprint(body["contractId"]), 10)
		return strings.TrimRight(fmt.Sprintf("exercise %s:%s on %s  %s",
			template, choice, contract, summarizeArgs(body["choiceArgument"])), " ")
	}
	if body, ok := commandBody(obj, "CreateCommand", "createCommand"); ok {
		template := orQuestion(ShortTemplate(fmt.Sprint(body["templateId"])))
		return strings.TrimRight(fmt.Sprintf("create %s  %s",
			template, summarizeArgs(body["createArguments"])), " ")
	}
	return fmt.Sprint(obj)
}

func commandBody(obj map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if body, ok := obj[key].(map[string]any); ok {
			return body, true
		}
	}
	return nil, false
}

// summarizeArgs renders the first three fields in the order they were given.
// Python iterates the dict, so --arg flag order is what shows.
func summarizeArgs(value any) string {
	args, ok := value.(*model.Object)
	if !ok || args.Len() == 0 {
		return ""
	}
	keys := args.Keys()
	if len(keys) > 3 {
		keys = keys[:3]
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		item, _ := args.Get(key)
		parts = append(parts, key+"="+FormatScalar(item, nil))
	}
	return strings.Join(parts, "  ")
}

func orQuestion(value string) string {
	if value == "" {
		return "?"
	}
	return value
}

// logMatchedBy names the correlation term that matched a log line, so a reader
// can tell whether the match was on a command id or a trace id.
// Ports _log_matched_by.
func logMatchedBy(c *model.Completion) string {
	terms := map[string]bool{}
	if list, ok := c.Get("logMatchTerms").([]any); ok {
		for _, term := range list {
			terms[scalarText(term)] = true
		}
	}
	if len(terms) == 0 {
		return ""
	}

	for _, field := range []struct{ key, name string }{
		{"commandId", "command id"},
		{"command_id", "command id"},
		{"traceId", "trace id"},
		{"submissionId", "submission id"},
		{"submission_id", "submission id"},
		{"updateId", "update id"},
		{"update_id", "update id"},
	} {
		if value := c.String(field.key); value != "" && terms[value] {
			return field.name
		}
	}
	if status, ok := model.ObjectField(c.Raw, "status"); ok {
		for _, field := range []struct{ key, name string }{
			{"traceId", "trace id"},
			{"correlationId", "correlation id"},
		} {
			if value := model.ObjectString(status, field.key); value != "" && terms[value] {
				return field.name
			}
		}
	}
	return "trace id"
}

// EntityFromRequest derives the "in choice Asset.Withdraw" hint from the
// submitted command, which names the template and choice directly.
// Ports _entity_from_request.
func EntityFromRequest(request map[string]any) (kind, name string) {
	commands, _ := request["commands"].([]any)
	for _, command := range commands {
		obj, ok := command.(map[string]any)
		if !ok {
			continue
		}
		body, ok := commandBody(obj, "ExerciseCommand", "exerciseCommand")
		if !ok {
			continue
		}
		template := ShortTemplate(fmt.Sprint(body["templateId"]))
		choice := fmt.Sprint(body["choice"])
		if template != "" && choice != "" && choice != "<nil>" {
			return "choice", template + "." + choice
		}
	}
	return "", ""
}
