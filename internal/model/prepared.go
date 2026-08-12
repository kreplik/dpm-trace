package model

import (
	"fmt"
	"os"
	"time"
)

// PreparedArtifactSchema is the schema string prepared artifacts carry.
const PreparedArtifactSchema = "dpm-trace/prepared-artifact/v0"

// LoadPreparedArtifact reads and validates a prepared-command artifact.
// Ports load_prepared_artifact.
func LoadPreparedArtifact(path string) (*Object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	artifact, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("prepared artifact must be a JSON object: %s", path)
	}
	if schema := pickString(artifact, "schema"); schema != PreparedArtifactSchema {
		return nil, fmt.Errorf("unsupported prepared artifact schema in %s: %s", path, quoteOrNone(schema))
	}
	return artifact, nil
}

// PreparedSummary is the prepared side of a comparison header.
// Ports prepared_summary_for_compare.
type PreparedSummary struct {
	CommandID               string
	ActAs                   []string
	ReadAs                  []string
	Commands                int
	PreparedTransactionHash string
	Committed               bool
}

// SummarizePrepared ports prepared_summary_for_compare.
func SummarizePrepared(prepared *Object) PreparedSummary {
	request, _ := pickObject(prepared, "request")
	response, _ := pickObject(prepared, "response")
	commands, _ := pick(request, "commands").([]any)
	return PreparedSummary{
		CommandID:               pickString(request, "commandId"),
		ActAs:                   listString(pick(request, "actAs")),
		ReadAs:                  listString(pick(request, "readAs")),
		Commands:                len(commands),
		PreparedTransactionHash: pickString(response, "preparedTransactionHash"),
		Committed:               false,
	}
}

// CommandRow is one submitted command, shaped for comparison against an event.
// Ports command_compare_row.
type CommandRow struct {
	Kind       string
	Template   string
	Choice     string
	ContractID string
	Value      any
	ValueLabel string
	Keys       []string // populated for unknown command shapes
}

// CommandCompareRow ports command_compare_row.
func CommandCompareRow(command *Object) CommandRow {
	if body, ok := pickObject(command, "CreateCommand"); ok {
		return CommandRow{
			Kind:       KindCreate,
			Template:   pickString(body, "templateId"),
			Value:      pick(body, "createArguments"),
			ValueLabel: "arguments",
		}
	}
	if body, ok := pickObject(command, "ExerciseCommand"); ok {
		return CommandRow{
			Kind:       KindExercise,
			Template:   pickString(body, "templateId"),
			Choice:     pickString(body, "choice"),
			ContractID: shortValue(pickString(body, "contractId"), 48),
			Value:      pick(body, "choiceArgument"),
			ValueLabel: "choice argument",
		}
	}
	return CommandRow{Kind: "unknown", Keys: sortedKeys(command)}
}

// PreparedComparison is the result of comparing a prepared command to a
// committed update.
type PreparedComparison struct {
	Kind        string
	Left        PreparedSummary
	Right       TraceSummary
	Commands    []CommandRow
	RootEvents  []CompareRow
	StateDiff   map[string]int
	PreparedLen int
	RootLen     int
}

// ComparePreparedToTrace ports compare_prepared_to_trace.
func ComparePreparedToTrace(prepared *Object, trace *Trace) *PreparedComparison {
	request, _ := pickObject(prepared, "request")
	rawCommands, _ := pick(request, "commands").([]any)

	commands := []CommandRow{}
	for _, raw := range rawCommands {
		if obj, ok := asObject(raw); ok {
			commands = append(commands, CommandCompareRow(obj))
		}
	}

	roots := []CompareRow{}
	for _, id := range trace.RootEventIDs {
		if ev, ok := trace.EventsByID[id]; ok {
			// Python passes no events_by_id here, so children are not inlined.
			roots = append(roots, EventCompareRow(ev, nil))
		}
	}

	return &PreparedComparison{
		Kind:        KindPreparedVsUpdate,
		Left:        SummarizePrepared(prepared),
		Right:       SummarizeTrace(trace),
		Commands:    commands,
		RootEvents:  roots,
		StateDiff:   StateDiffCounts(trace),
		PreparedLen: len(rawCommands),
		RootLen:     len(roots),
	}
}

// JSON renders the comparison for --print-json.
func (c *PreparedComparison) JSON() map[string]any {
	commands := make([]any, 0, len(c.Commands))
	for _, row := range c.Commands {
		commands = append(commands, row.json())
	}
	return map[string]any{
		"kind":  c.Kind,
		"left":  c.Left.json(),
		"right": c.Right.json(),
		"diff": map[string]any{
			"commandCount": map[string]any{
				"prepared":         c.PreparedLen,
				"updateRootEvents": c.RootLen,
			},
			"rootEvents": rowsJSON(c.RootEvents),
			"commands":   commands,
			"stateDiff":  intMap(c.StateDiff),
			"notes": []string{
				"Prepared data is not committed.",
				"The comparison checks visible command/event shape; it is not proof of semantic equivalence.",
			},
		},
	}
}

func (s PreparedSummary) json() map[string]any {
	return map[string]any{
		"commandId":               nilIfBlank(s.CommandID),
		"actAs":                   stringsJSON(s.ActAs),
		"readAs":                  stringsJSON(s.ReadAs),
		"commands":                s.Commands,
		"preparedTransactionHash": nilIfBlank(s.PreparedTransactionHash),
		"committed":               s.Committed,
	}
}

func (r CommandRow) json() map[string]any {
	if r.Kind == "unknown" {
		return map[string]any{"kind": "unknown", "keys": stringsJSON(r.Keys)}
	}
	out := map[string]any{
		"kind":       r.Kind,
		"template":   nilIfBlank(r.Template),
		"value":      r.Value,
		"valueLabel": nilIfBlank(r.ValueLabel),
	}
	if r.Kind == KindExercise {
		out["choice"] = nilIfBlank(r.Choice)
		out["contractId"] = r.ContractID
	}
	return out
}

func sortedKeys(obj *Object) []string {
	keys := obj.Keys()
	sortStrings(keys)
	return keys
}

// NewPreparedArtifact wraps a prepare request and response for export.
// Ports create_prepared_artifact.
//
// committed is always false and the privacy note is fixed: this is prepared
// data from a non-committing call, and the artifact must never read as a
// committed transaction.
func NewPreparedArtifact(request map[string]any, response *Object, sourceURL, ledgerURL string, actAs, readAs []string) map[string]any {
	return map[string]any{
		"schema":    PreparedArtifactSchema,
		"createdAt": time.Now().UTC().Format("2006-01-02T15:04:05.000000Z"),
		"kind":      "prepared-command",
		"source":    "ledger-json-api",
		"sourceUrl": sourceURL,
		"participant": map[string]any{
			"ledgerUrl": nilIfBlank(ledgerURL),
			"actAs":     stringsJSON(actAs),
			"readAs":    stringsJSON(readAs),
		},
		"committed": false,
		"request":   request,
		"response":  response,
		"privacy": map[string]any{
			"scope":                    "Prepared command result from an authorized participant endpoint.",
			"missingPrivateDataPolicy": "counterparty-private data outside this authorization context is not present in the artifact",
		},
	}
}
