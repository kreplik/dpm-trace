package ledger

import (
	"fmt"
	"strconv"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// CompletionLookup describes a completions query.
type CompletionLookup struct {
	CommandID      string
	Parties        []string
	UserID         string
	BeginExclusive string
	Limit          int
	TimeoutMs      int
}

// FetchCompletion finds the completion for a command id.
//
// A failed submission has no update id, so this is the only way to see what
// happened to it. The window is bounded by beginExclusive and limit, and a
// command outside that window is reported as such rather than as "no failure".
// Ports fetch_completion_by_command_id.
func (c *Client) FetchCompletion(lookup CompletionLookup) (*model.Completion, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("--submitter/--participant-url/--ledger-url is required")
	}
	if len(lookup.Parties) == 0 {
		return nil, fmt.Errorf("--act-as, --read-as, or --party is required for completion lookup")
	}

	beginExclusive := int64(0)
	if lookup.BeginExclusive != "" {
		parsed, err := strconv.ParseInt(lookup.BeginExclusive, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("--begin-exclusive must be an integer offset")
		}
		beginExclusive = parsed
	}

	body := map[string]any{
		"parties":        lookup.Parties,
		"beginExclusive": beginExclusive,
	}
	switch {
	case lookup.UserID != "":
		body["userId"] = lookup.UserID
	case c.Token == "":
		// Without a token the participant cannot resolve the user itself.
		body["userId"] = UserID("", c.BaseURL, "", "")
	}

	limit := lookup.Limit
	if limit < 1 {
		limit = 1
	}
	timeout := lookup.TimeoutMs
	if timeout < 1 {
		timeout = 1
	}

	url := JoinURL(c.BaseURL, CompletionsPath)
	query := fmt.Sprintf("?limit=%d&stream_idle_timeout_ms=%d", limit, timeout)
	raw, err := c.JSONValue("POST", url+query, body, true)
	if err != nil {
		return nil, err
	}

	for _, candidate := range NormalizeCompletionList(raw) {
		if model.ObjectString(candidate, "commandId") != lookup.CommandID {
			continue
		}
		candidate.Set("source", "ledger-json-api")
		candidate.Set("sourceUrl", url)

		context := model.NewObject()
		context.Set("commandId", lookup.CommandID)
		context.Set("parties", toAnySlice(lookup.Parties))
		context.Set("beginExclusive", beginExclusive)
		context.Set("limit", lookup.Limit)
		candidate.Set("lookup", context)

		return &model.Completion{Raw: candidate}, nil
	}

	// Single quotes, matching Python's !r formatting of the command id.
	return nil, fmt.Errorf(
		"completion '%s' not found in the queried completion window; "+
			"try --begin-exclusive with an earlier offset or --completion-file with captured JSON",
		lookup.CommandID)
}

// NormalizeCompletionList unwraps the several shapes a completions response
// arrives in. Ports normalize_completion_list.
func NormalizeCompletionList(raw any) []*model.Object {
	var candidates []any
	switch value := raw.(type) {
	case *model.Object:
		for _, key := range []string{"completions", "items", "responses", "completionResponses"} {
			if nested, ok := value.Get(key); ok {
				if list, isList := nested.([]any); isList {
					candidates = list
					break
				}
			}
		}
		if candidates == nil {
			candidates = []any{value}
		}
	case []any:
		candidates = value
	default:
		return nil
	}

	var result []*model.Object
	for _, item := range candidates {
		obj, ok := item.(*model.Object)
		if !ok {
			continue
		}
		if completion := model.NormalizeCompletion(obj); completion.Len() > 0 {
			result = append(result, completion)
		}
	}
	return result
}

func toAnySlice(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
