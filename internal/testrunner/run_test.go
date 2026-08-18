package testrunner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/source"
)

// The CI gate: `dpm trace test` must exit non-zero when any case did not pass.
// Anything other than "passed" counts, including errored.
func TestResultFailedCountsAnythingNotPassed(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []string
		want     bool
	}{
		{"all passed", []string{StatusPassed, StatusPassed}, false},
		{"one failed", []string{StatusPassed, StatusFailed}, true},
		{"one errored", []string{StatusPassed, StatusError}, true},
		{"no cases", nil, false},
	} {
		var result Result
		for _, status := range tc.statuses {
			result.Cases = append(result.Cases, Case{Status: status})
		}
		if got := result.Failed(); got != tc.want {
			t.Errorf("%s: Failed() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The assistant and damlc take different arguments for the same run: damlc
// needs --package-root, and only the assistant understands the legacy warning
// flag. Getting this wrong makes `test` fail on one toolchain and not the other.
func TestCommandDiffersForDamlcAndAssistant(t *testing.T) {
	opts := Options{Root: "/pkg", Daml: "/sdk/bin/damlc"}
	command := strings.Join(Command(opts, "/tmp/j.xml", "", ""), " ")
	if !strings.Contains(command, "--package-root /pkg") {
		t.Errorf("damlc invocation missing --package-root: %s", command)
	}
	if strings.Contains(command, "--no-legacy-assistant-warning") {
		t.Errorf("damlc must not get the assistant's warning flag: %s", command)
	}

	opts = Options{Root: "/pkg", Daml: "/sdk/bin/daml"}
	command = strings.Join(Command(opts, "/tmp/j.xml", "", ""), " ")
	if !strings.Contains(command, "--no-legacy-assistant-warning") {
		t.Errorf("assistant invocation missing the warning flag: %s", command)
	}
	if strings.Contains(command, "--package-root") {
		t.Errorf("assistant must not get --package-root: %s", command)
	}
}

// Empty optional paths must not produce dangling flags, and empty --files
// entries must be dropped rather than passed as "".
func TestCommandOmitsEmptyOptions(t *testing.T) {
	command := Command(Options{Files: []string{"", "daml/Test.daml", ""}}, "/tmp/j.xml", "", "")
	joined := strings.Join(command, " ")
	for _, unwanted := range []string{"--transactions-output", "--table-output", "--test-pattern"} {
		if strings.Contains(joined, unwanted) {
			t.Errorf("unset option leaked into the command: %s", joined)
		}
	}
	for i, arg := range command {
		if arg == "" {
			t.Errorf("empty argument at %d: %q", i, command)
		}
	}
	if !strings.HasSuffix(joined, "--files daml/Test.daml") {
		t.Errorf("files not appended correctly: %s", joined)
	}
}

// The report is a published artifact (dpm-trace/test-report/v0), so its shape
// is a compatibility contract: absent values are null, not omitted, and lists
// are empty arrays, not null.
func TestReportJSONShape(t *testing.T) {
	elapsed := 1.5
	cases := []Case{
		{
			Name: "testIssue", Classname: "Test", Status: StatusPassed, Time: &elapsed,
			Stats: map[string]int{"transactions": 2}, TouchedLocations: []string{"Asset.daml"},
		},
		{
			Name: "testFails", Classname: "Test", Status: StatusFailed,
			Message:     "Insufficient balance",
			Diagnostics: []source.Location{{Path: "/pkg/Asset.daml", Line: 54, Column: 20, Label: "local source"}},
		},
	}

	report := ReportJSON("/pkg", []string{"daml", "test"}, cases, 5)
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("report does not marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded["schema"] != "dpm-trace/test-report/v0" {
		t.Errorf("schema = %v", decoded["schema"])
	}

	tests, _ := decoded["tests"].([]any)
	if len(tests) != 2 {
		t.Fatalf("got %d tests, want 2", len(tests))
	}

	first, _ := tests[0].(map[string]any)
	if first["time"] != 1.5 {
		t.Errorf("time = %v, want 1.5", first["time"])
	}
	if first["message"] != nil {
		t.Errorf("absent message must be null, got %v", first["message"])
	}

	second, _ := tests[1].(map[string]any)
	if second["time"] != nil {
		t.Errorf("absent time must be null, got %v", second["time"])
	}
	if touched, ok := second["touchedLocations"].([]any); !ok || touched == nil {
		t.Errorf("touchedLocations must be [] when unset, got %v", second["touchedLocations"])
	}
	diagnostics, _ := second["diagnostics"].([]any)
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %v", second["diagnostics"])
	}
	diagnostic, _ := diagnostics[0].(map[string]any)
	if diagnostic["line"] != float64(54) || diagnostic["basis"] != "local source" {
		t.Errorf("diagnostic = %v", diagnostic)
	}
}

// The artifact copy helpers are small but their failure modes are silent: a
// missed file leaves the report without transaction text.
func TestFileExistsAndCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(src, []byte("transaction tree"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !fileExists(src) {
		t.Error("fileExists said an existing file does not exist")
	}
	if fileExists(filepath.Join(dir, "absent.txt")) {
		t.Error("fileExists said a missing file exists")
	}
	// A directory reports true: this mirrors cli.py, which uses Path.exists().
	// Callers only ever ask about paths daml wrote, so the distinction has
	// never mattered -- pinned here so a "fix" is a deliberate divergence.
	if !fileExists(dir) {
		t.Error("fileExists must match Path.exists(), which is true for a directory")
	}

	dst := filepath.Join(dir, "copy.txt")
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "transaction tree" {
		t.Errorf("copied content = %q", content)
	}

	if err := copyFile(filepath.Join(dir, "absent.txt"), filepath.Join(dir, "out.txt")); err == nil {
		t.Error("copyFile from a missing source returned no error")
	}
	// Like shutil.copyfile, the destination's parent must already exist.
	if err := copyFile(src, filepath.Join(dir, "nested", "copy.txt")); err == nil {
		t.Error("copyFile created a missing parent directory; cli.py does not")
	}
}
