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
