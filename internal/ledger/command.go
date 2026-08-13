package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// CommandSpec is the flag-level description of a command to prepare or submit.
type CommandSpec struct {
	Template     string
	ContractID   string
	Choice       string
	Args         []string // key=value assignments
	ArgsJSON     string
	ArgsFile     string
	CommandsFile string
	CommandJSON  string
}

// Commands turns the flags into the command list the API expects.
// Ports prepare_commands.
func (s CommandSpec) Commands() ([]any, error) {
	if s.CommandsFile != "" && s.CommandJSON != "" {
		return nil, fmt.Errorf("use only one of --commands or --command-json")
	}
	if s.CommandsFile != "" {
		data, err := os.ReadFile(s.CommandsFile)
		if err != nil {
			return nil, err
		}
		return normalizeCommandsJSON(data, s.CommandsFile)
	}
	if s.CommandJSON != "" {
		return normalizeCommandsJSON([]byte(s.CommandJSON), "--command-json")
	}
	if s.Template != "" {
		command, err := s.explicitCommand()
		if err != nil {
			return nil, err
		}
		return []any{command}, nil
	}
	return nil, fmt.Errorf("prepare needs --commands, --command-json, or --template")
}

// explicitCommand builds a create or exercise from the individual flags.
// Ports explicit_js_command.
func (s CommandSpec) explicitCommand() (map[string]any, error) {
	arguments, err := s.arguments()
	if err != nil {
		return nil, err
	}
	if s.ContractID != "" || s.Choice != "" {
		if s.ContractID == "" {
			return nil, fmt.Errorf("--contract-id is required for an exercise simulation")
		}
		if s.Choice == "" {
			return nil, fmt.Errorf("--choice is required for an exercise simulation")
		}
		return map[string]any{
			"ExerciseCommand": map[string]any{
				"templateId":     s.Template,
				"contractId":     s.ContractID,
				"choice":         s.Choice,
				"choiceArgument": arguments,
			},
		}, nil
	}
	return map[string]any{
		"CreateCommand": map[string]any{
			"templateId":      s.Template,
			"createArguments": arguments,
		},
	}, nil
}

// arguments merges --args-json/--args-file with --arg assignments.
// Ports command_arguments.
func (s CommandSpec) arguments() (any, error) {
	if s.ArgsJSON != "" && s.ArgsFile != "" {
		return nil, fmt.Errorf("use only one of --args-json or --args-file")
	}

	var base any = model.NewObject()
	switch {
	case s.ArgsJSON != "":
		decoded, err := model.Decode([]byte(s.ArgsJSON))
		if err != nil {
			return nil, fmt.Errorf("invalid JSON in --args-json: %w", err)
		}
		base = decoded
	case s.ArgsFile != "":
		data, err := os.ReadFile(s.ArgsFile)
		if err != nil {
			return nil, err
		}
		decoded, err := model.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON in %s: %w", s.ArgsFile, err)
		}
		base = decoded
	}

	assignments, err := ParseArgAssignments(s.Args)
	if err != nil {
		return nil, err
	}
	if assignments.Len() == 0 {
		return base, nil
	}

	object, ok := base.(*model.Object)
	if !ok {
		return nil, fmt.Errorf("--arg can only be used when arguments are a JSON object")
	}
	for _, key := range assignments.Keys() {
		value, _ := assignments.Get(key)
		object.Set(key, value)
	}
	return object, nil
}

// ParseArgAssignments parses repeated key=value flags, preserving flag order.
// Ports parse_arg_assignments; order is observable because the failure view
// prints the first three arguments as given.
func ParseArgAssignments(values []string) (*model.Object, error) {
	result := model.NewObject()
	for _, raw := range values {
		key, value, found := strings.Cut(raw, "=")
		if !found {
			return nil, fmt.Errorf("--arg must use key=value syntax: %q", raw)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("--arg key cannot be empty: %q", raw)
		}
		result.Set(key, ParseScalar(strings.TrimSpace(value)))
	}
	return result, nil
}

// ParseScalar infers a JSON type for an --arg value. Ports parse_scalar.
//
// Numbers stay strings on purpose. Every numeric field in Daml LF is Int64 or
// Numeric, and the JSON Ledger API encodes both as JSON strings (Int64 does not
// survive a JavaScript number). Canton 3.5 rejects a JSON number with
// "Expected ujson.Str", so --arg quantity=100 must send "100".
func ParseScalar(value string) any {
	switch value {
	case "null":
		return nil
	case "true":
		return true
	case "false":
		return false
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			return decoded
		}
	}
	return value
}

func normalizeCommandsJSON(data []byte, source string) ([]any, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", source, err)
	}
	switch value := raw.(type) {
	case map[string]any:
		if commands, ok := value["commands"].([]any); ok {
			return commands, nil
		}
		return []any{value}, nil
	case []any:
		return value, nil
	}
	return nil, fmt.Errorf("commands JSON must be an object, an array, or an object with a commands field")
}

// UserID resolves the submitting user. Ports prepare_user_id, including the
// warning: falling back to participant_admin against a non-local participant is
// almost always wrong, so say so rather than failing silently.
func UserID(explicit, ledgerURL, token, tokenFile string) string {
	if explicit != "" {
		return explicit
	}
	if token != "" || tokenFile != "" {
		return ""
	}
	if parsed, err := url.Parse(ledgerURL); err == nil {
		host := parsed.Hostname()
		switch host {
		case "", "localhost", "127.0.0.1", "::1":
		default:
			fmt.Fprintf(os.Stderr,
				"warning: using default user ID with non-local participant %q; pass --user-id or --token.\n", host)
		}
	}
	return "participant_admin"
}

// GenerateCommandID mirrors the uuid4().hex[:12] suffix cli.py uses. Go has no
// stdlib UUID, and only the hex suffix is observable, so crypto/rand supplies it.
func GenerateCommandID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "000000000000"
	}
	return prefix + hex.EncodeToString(buf)
}

// ParseParties splits repeated and comma-separated party flags.
// Ports parse_parties, which deliberately does not deduplicate: the request
// carries the parties as given, and inventing a dedup here would change the
// submitted payload.
func ParseParties(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}
