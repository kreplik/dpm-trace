package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Completion is a submission outcome read from the Ledger API or a captured
// file. A failed submission has no update id, so completion data is all there
// is -- the tool must never present it as a committed transaction.
type Completion struct {
	Raw *Object
}

// LoadCompletion reads a captured completion response from disk.
func LoadCompletion(path string) (*Completion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := Decode(data)
	if err != nil {
		// Keep the decoder's message: "must be a JSON object" is a guess that
		// hides a syntax error, which is the more common cause.
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Completion{Raw: NormalizeCompletion(raw)}, nil
}

// NormalizeCompletion unwraps the envelopes a completion arrives in.
// Ports normalize_completion.
func NormalizeCompletion(raw *Object) *Object {
	if raw == nil {
		return NewObject()
	}
	completion := raw
	// Python reassigns through every matching key in order, not just the first.
	for _, key := range []string{"completionResponse", "Completion", "completion", "value"} {
		if inner, ok := pickObject(completion, key); ok {
			completion = inner
		}
	}
	if inner, ok := pickObject(completion, "Completion"); ok {
		return NormalizeCompletion(inner)
	}
	if _, empty := completion.Get("Empty"); empty {
		return NewObject()
	}
	if _, checkpoint := completion.Get("OffsetCheckpoint"); checkpoint {
		return NewObject()
	}
	return completion
}

// CommandID returns the submitted command id.
//
// A rejection -- the shape a failed submission actually arrives in -- carries
// no top-level commandId: the participant records the submission under
// `context.commands`, formatted rather than structured. Reading only the top
// level reported no command id for exactly the submissions this tool exists to
// explain, and left a prepared/completion comparison unable to say the two
// were the same command.
func (c *Completion) CommandID() string {
	if id := c.String("commandId", "command_id"); id != "" {
		return id
	}
	return commandFieldFromContext(c.Raw, "commandId")
}

// SubmissionID returns the submission id, with the same fallback.
func (c *Completion) SubmissionID() string {
	if id := c.String("submissionId", "submission_id"); id != "" {
		return id
	}
	return commandFieldFromContext(c.Raw, "submissionId")
}

// commandFieldFromContext reads one field out of the formatted
// `context.commands` blob a rejection carries.
func commandFieldFromContext(raw *Object, field string) string {
	context, ok := ObjectField(raw, "context")
	if !ok {
		return ""
	}
	pattern, err := regexp.Compile(regexp.QuoteMeta(field) + `:\s*'?([^,}']+)`)
	if err != nil {
		return ""
	}
	match := pattern.FindStringSubmatch(ObjectString(context, "commands"))
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

// Get exposes a field of the underlying completion object.
func (c *Completion) Get(keys ...string) any { return pick(c.Raw, keys...) }

// String renders a field as a string.
func (c *Completion) String(keys ...string) string { return pickString(c.Raw, keys...) }

// StatusFields returns the status code and message, probing the several shapes
// they arrive in. Ports completion_status_fields.
func (c *Completion) StatusFields() (code any, message any) {
	if status, ok := pickObject(c.Raw, "status", "completionStatus"); ok {
		code = pick(status, "code", "grpcCode", "grpcCodeValue")
		message = pick(status, "message", "details")
		if code != nil || message != nil {
			return code, message
		}
	} else if status := pick(c.Raw, "status", "completionStatus"); status != nil && status != "" {
		return nil, status
	}

	code = pick(c.Raw, "code", "grpcCode", "grpcCodeValue", "errorCategory")
	message = pick(c.Raw, "message", "details", "cause")
	if code != nil || message != nil {
		return code, message
	}
	if pick(c.Raw, "updateId", "update_id") != nil {
		return "OK", "committed"
	}
	return nil, nil
}

// Committed reports whether the submission produced an update.
func (c *Completion) Committed() bool {
	return c.String("updateId", "update_id") != ""
}

// Failed reports a non-OK status with no committed update.
func (c *Completion) Failed() bool {
	code, _ := c.StatusFields()
	if c.Committed() {
		return false
	}
	switch toString(code) {
	case "", "OK", "0":
		return false
	}
	return true
}

// CompletionComparison is the result of comparing a prepared command against a
// completion.
type CompletionComparison struct {
	Kind                     string
	Left                     PreparedSummary
	Completion               *Completion
	StatusCode               any
	Message                  any
	CommittedUpdateAvailable bool
	CommandIDMatch           bool
}

// ComparePreparedToCompletion ports compare_prepared_to_completion.
func ComparePreparedToCompletion(prepared *Object, completion *Completion) *CompletionComparison {
	code, message := completion.StatusFields()
	request, _ := pickObject(prepared, "request")
	preparedID := pickString(request, "commandId")
	completionID := completion.CommandID()

	return &CompletionComparison{
		Kind:                     KindPreparedVsCompletion,
		Left:                     SummarizePrepared(prepared),
		Completion:               completion,
		StatusCode:               code,
		Message:                  message,
		CommittedUpdateAvailable: completion.Committed(),
		CommandIDMatch:           preparedID != "" && preparedID == completionID,
	}
}

// JSON renders the comparison for --print-json.
func (c *CompletionComparison) JSON() map[string]any {
	logMatches := c.Completion.Get("logMatches")
	if logMatches == nil {
		logMatches = []any{}
	}
	return map[string]any{
		"kind": c.Kind,
		"left": c.Left.json(),
		"right": map[string]any{
			"commandId":        nilIfBlank(c.Completion.CommandID()),
			"updateId":         nilIfBlank(c.Completion.String("updateId", "update_id")),
			"offset":           c.Completion.Get("offset", "completionOffset"),
			"submissionId":     nilIfBlank(c.Completion.SubmissionID()),
			"statusCode":       c.StatusCode,
			"message":          c.Message,
			"source":           c.Completion.Get("source"),
			"traceContext":     c.Completion.Get("traceContext"),
			"synchronizerTime": c.Completion.Get("synchronizerTime", "synchronizer_time"),
			"logMatches":       logMatches,
		},
		"diff": map[string]any{
			"committedUpdateAvailable": c.CommittedUpdateAvailable,
			"commandIdMatch":           c.CommandIDMatch,
			"logMatches":               logMatches,
		},
	}
}

// AttachLogMatches correlates a completion with operator log lines.
//
// Terms shorter than 6 characters are dropped: a short id would match
// arbitrary log text and the correlation would be noise rather than evidence.
// Ports attach_log_matches.
func AttachLogMatches(completion *Object, logFiles []string) *Object {
	if len(logFiles) == 0 {
		return completion
	}
	terms := CorrelationTerms(completion)
	matches := []any{}

	for _, path := range logFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			entry := NewObject()
			entry.Set("file", path)
			// Python surfaces the OSError text; reproduce the ENOENT form so a
			// missing log file reads the same from either implementation.
			entry.Set("error", osErrorText(err, path))
			matches = append(matches, entry)
			continue
		}
		for i, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			// splitlines() strips the carriage return; splitting on \n alone
			// leaves it, so a CRLF log renders a stray \r at each line end.
			line = strings.TrimSuffix(line, "\r")
			if !containsAnyTerm(line, terms) {
				continue
			}
			text := line
			// Counted in characters, as Python's slicing does.
			if runes := []rune(text); len(runes) > 500 {
				text = string(runes[:500])
			}
			entry := NewObject()
			entry.Set("file", path)
			entry.Set("line", i+1)
			entry.Set("text", text)
			matches = append(matches, entry)
		}
	}

	sorted := make([]string, 0, len(terms))
	for term := range terms {
		sorted = append(sorted, term)
	}
	sortStrings(sorted)

	completion.Set("logMatches", matches)
	completion.Set("logMatchTerms", toAnyList(sorted))
	return completion
}

// CorrelationTerms collects the ids that can tie a completion to a log line.
// Ports completion_correlation_terms.
func CorrelationTerms(completion *Object) map[string]bool {
	terms := map[string]bool{}
	add := func(value any) {
		if text := toString(value); text != "" {
			terms[text] = true
		}
	}
	for _, key := range []string{"commandId", "command_id", "updateId", "update_id",
		"submissionId", "submission_id", "traceId", "correlationId"} {
		add(pick(completion, key))
	}
	// A rejection keeps these inside context.commands, and the command id is
	// the term an operator greps a participant log for.
	add(commandFieldFromContext(completion, "commandId"))
	add(commandFieldFromContext(completion, "submissionId"))
	if status, ok := pickObject(completion, "status"); ok {
		add(pick(status, "traceId"))
		add(pick(status, "correlationId"))
	}
	if traceContext, ok := pickObject(completion, "traceContext"); ok {
		for _, key := range traceContext.Keys() {
			value, _ := traceContext.Get(key)
			add(value)
		}
	}

	// A term under 6 characters matches too much log text to be evidence.
	for term := range terms {
		if len(term) < 6 {
			delete(terms, term)
		}
	}
	return terms
}

func containsAnyTerm(line string, terms map[string]bool) bool {
	for term := range terms {
		if term != "" && strings.Contains(line, term) {
			return true
		}
	}
	return false
}

func toAnyList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

// osErrorText renders a file error the way Python's OSError does.
func osErrorText(err error, path string) string {
	if os.IsNotExist(err) {
		return fmt.Sprintf("[Errno 2] No such file or directory: '%s'", path)
	}
	if os.IsPermission(err) {
		return fmt.Sprintf("[Errno 13] Permission denied: '%s'", path)
	}
	return err.Error()
}

// LooksLikeCompletion reports whether a decoded object is completion data:
// either a completion record, or the error body a rejected submission returns.
func LooksLikeCompletion(obj *Object) bool {
	if obj == nil {
		return false
	}
	for _, key := range []string{"completionResponse", "completion", "status", "errorCategory", "grpcCodeValue"} {
		if _, ok := obj.Get(key); ok {
			return true
		}
	}
	if _, ok := obj.Get("code"); ok {
		if _, ok := obj.Get("cause"); ok {
			return true
		}
	}
	// A successful completion carries no status: it is identified by a command
	// id alongside the fields the ledger stamps on it.
	if _, ok := obj.Get("commandId"); ok {
		for _, key := range []string{"offset", "submissionId", "synchronizerTime", "updateId"} {
			if _, ok := obj.Get(key); ok {
				return true
			}
		}
	}
	return false
}
