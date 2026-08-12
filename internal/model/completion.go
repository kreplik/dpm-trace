package model

import (
	"fmt"
	"os"
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
		return nil, fmt.Errorf("completion file must be a JSON object: %s", path)
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
	completionID := completion.String("commandId", "command_id")

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
			"commandId":        nilIfBlank(c.Completion.String("commandId", "command_id")),
			"updateId":         nilIfBlank(c.Completion.String("updateId", "update_id")),
			"offset":           c.Completion.Get("offset"),
			"submissionId":     nilIfBlank(c.Completion.String("submissionId", "submission_id")),
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
