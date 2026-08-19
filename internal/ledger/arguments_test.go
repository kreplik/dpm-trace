package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// arguments() is how --arg, --args-json and --args-file become the payload sent
// to the participant. Precedence matters: --arg overrides a field of the same
// name from JSON, so a user can take a captured payload and tweak one value.

func encode(t *testing.T, value any) string {
	t.Helper()
	data, err := model.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestArgumentsFromAssignments(t *testing.T) {
	spec := CommandSpec{Args: []string{"owner=Alice", "quantity=100"}}
	got, err := spec.arguments()
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := got.(*model.Object)
	if !ok {
		t.Fatalf("got %#v", got)
	}
	// Flag order, not sorted: it is what the user typed and what the failure
	// summary echoes back.
	if keys := obj.Keys(); strings.Join(keys, ",") != "owner,quantity" {
		t.Errorf("keys = %v, want flag order", keys)
	}
}

func TestArgumentsFromJSON(t *testing.T) {
	spec := CommandSpec{ArgsJSON: `{"owner":"Alice","quantity":"100"}`}
	got, err := spec.arguments()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encode(t, got), `"owner": "Alice"`) {
		t.Errorf("got %s", encode(t, got))
	}

	if _, err := (CommandSpec{ArgsJSON: `not json`}).arguments(); err == nil {
		t.Error("invalid --args-json accepted")
	} else if !strings.Contains(err.Error(), "--args-json") {
		t.Errorf("error does not name the flag: %v", err)
	}
}

func TestArgumentsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "args.json")
	if err := os.WriteFile(path, []byte(`{"name":"GOLD"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := (CommandSpec{ArgsFile: path}).arguments()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encode(t, got), "GOLD") {
		t.Errorf("got %s", encode(t, got))
	}

	if _, err := (CommandSpec{ArgsFile: filepath.Join(dir, "absent.json")}).arguments(); err == nil {
		t.Error("a missing --args-file returned no error")
	}

	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (CommandSpec{ArgsFile: bad}).arguments(); err == nil {
		t.Error("malformed --args-file accepted")
	}
}

// The two JSON sources are mutually exclusive; accepting both would make the
// precedence silent.
func TestArgumentsRejectsBothJSONSources(t *testing.T) {
	_, err := (CommandSpec{ArgsJSON: "{}", ArgsFile: "/tmp/x.json"}).arguments()
	if err == nil {
		t.Fatal("both sources accepted")
	}
	if !strings.Contains(err.Error(), "only one of") {
		t.Errorf("error = %v", err)
	}
}

// --arg on top of --args-json is the tweak-a-captured-payload workflow.
func TestArgumentsOverlayAssignmentsOnJSON(t *testing.T) {
	spec := CommandSpec{
		ArgsJSON: `{"owner":"Alice","quantity":"100"}`,
		Args:     []string{"quantity=5"},
	}
	got, err := spec.arguments()
	if err != nil {
		t.Fatal(err)
	}
	obj := got.(*model.Object)
	if v, _ := obj.Get("quantity"); v != "5" {
		t.Errorf("quantity = %#v, want the --arg to win", v)
	}
	if v, _ := obj.Get("owner"); v != "Alice" {
		t.Errorf("owner = %#v, want the JSON field kept", v)
	}
}

func TestArgumentsRejectsBadAssignment(t *testing.T) {
	if _, err := (CommandSpec{Args: []string{"noequals"}}).arguments(); err == nil {
		t.Error("an assignment without = was accepted")
	}
}

// A choice parameter need not be a record: a list, a scalar or null are all
// legal, and encoding/json would also round-trip large integers wrongly.
func TestArgumentsAcceptsNonObjectJSON(t *testing.T) {
	for _, raw := range []string{`[1,2]`, `"just text"`, `null`, `42`} {
		if _, err := (CommandSpec{ArgsJSON: raw}).arguments(); err != nil {
			t.Errorf("--args-json %s rejected: %v", raw, err)
		}
	}
	// Combining a non-object with --arg is the one case that must fail.
	if _, err := (CommandSpec{ArgsJSON: `[1,2]`, Args: []string{"a=b"}}).arguments(); err == nil {
		t.Error("--arg over a JSON array was accepted")
	}
}

// Values must survive the round trip exactly: encoding/json decodes into
// float64, so a large Int64 loses its last digit and 1.0 becomes 1.
func TestArgumentsPreserveNumericForm(t *testing.T) {
	got, err := (CommandSpec{ArgsJSON: `{"big":9007199254740993,"exact":1.0}`}).arguments()
	if err != nil {
		t.Fatal(err)
	}
	encoded := encode(t, got)
	for _, want := range []string{"9007199254740993", "1.0"} {
		if !strings.Contains(encoded, want) {
			t.Errorf("value not preserved: want %s in %s", want, encoded)
		}
	}
}

// ParseScalar takes the same path for inline JSON in --arg.
func TestParseScalarPreservesNumericForm(t *testing.T) {
	value := ParseScalar(`{"big":9007199254740993}`)
	if !strings.Contains(encode(t, value), "9007199254740993") {
		t.Errorf("got %s", encode(t, value))
	}
}
