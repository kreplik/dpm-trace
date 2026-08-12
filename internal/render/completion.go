package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// CompletionTrace writes the completion view of a submission.
// Ports print_completion_trace's non-compact path. The compact path is used by
// `dpm trace submit --allow-fail`, which is not ported yet, and source
// diagnostics need internal/source.
func CompletionTrace(w io.Writer, c *model.Completion, color Color) {
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
	printLogMatches(w, c.Get("logMatches"), color)
}

// PreparedCompletionComparison writes a prepared-vs-completion comparison.
// Ports print_prepared_completion_comparison, minus source diagnostics.
func PreparedCompletionComparison(w io.Writer, c *model.CompletionComparison, color Color, compact bool) {
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
		offset := orDash(scalarText(completion.Get("offset")))
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
	fmt.Fprintf(w, "  offset:     %s\n", orDash(scalarText(completion.Get("offset"))))
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
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + escaped + `"`
	}
	escaped = strings.ReplaceAll(escaped, "'", `\'`)
	return "'" + escaped + "'"
}
