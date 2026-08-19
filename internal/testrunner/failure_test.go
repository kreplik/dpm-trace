package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/source"
)

// The summary parser is the fallback when transaction HTML is unavailable, so
// it has to survive the exact shape `daml test` prints.
func TestParseSummaryReadsPerTestStats(t *testing.T) {
	stdout := strings.Join([]string{
		"Test Summary",
		"daml/Test.daml:testIssue: ok, 1 active contracts, 1 transactions.",
		"daml/Test.daml:testSplit: ok, 3 active contracts, 2 transactions.",
		"daml/Test.daml:testTransfer: ok, 2 active contracts, 2 transactions.",
	}, "\n")

	got := ParseSummary(stdout)
	if len(got) != 3 {
		t.Fatalf("parsed %d tests, want 3: %v", len(got), got)
	}
	if got["testSplit"]["transactions"] != 2 || got["testSplit"]["activeContracts"] != 3 {
		t.Errorf("testSplit = %v, want 2 transactions / 3 active contracts", got["testSplit"])
	}
}

// Singular "1 transaction" and quoted names both occur in real output.
func TestParseSummaryHandlesSingularAndQuotedNames(t *testing.T) {
	got := ParseSummary("daml/Test.daml:test'prime: ok, 1 active contract, 1 transaction.")
	if got["test'prime"]["transactions"] != 1 {
		t.Errorf("got %v, want 1 transaction for test'prime", got)
	}
}

// Anything that is not a summary line must be ignored rather than guessed at.
func TestParseSummaryIgnoresOtherOutput(t *testing.T) {
	for _, line := range []string{
		"", "Test Summary", "daml/Test.daml:testFails: failed",
		"Compiling Test.daml", "daml/Test.daml:testIssue: ok",
	} {
		if got := ParseSummary(line); len(got) != 0 {
			t.Errorf("ParseSummary(%q) = %v, want no entries", line, got)
		}
	}
}

// The decoration strippers exist so a red test shows the assertMsg text rather
// than Canton's wrapper. Each marker keeps only what follows the LAST one.
func TestStripErrorDecoration(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{
			// "AssertionFailed:" occurs twice, so the cut lands on the second
			// one and the category prefix survives. Verified identical in
			// cli.py's strip_canton_error_decoration.
			"UNHANDLED_EXCEPTION/DA.Exception.AssertionFailed:AssertionFailed (error category 9): Insufficient balance",
			"AssertionFailed (error category 9): Insufficient balance",
		},
		{"Aborted: Withdrawal amount must be positive", "Withdrawal amount must be positive"},
		{"Failed with status: DUPLICATE_CONTRACT_KEY", "DUPLICATE_CONTRACT_KEY"},
		{"plain message", "plain message"},
		{"", ""},
	} {
		if got := StripErrorDecoration(tc.in); got != tc.want {
			t.Errorf("StripErrorDecoration(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ChildEnv is a compatibility contract from the port: DPM_RESOLUTION_FILE must
// be dropped so the child resolves the target package rather than dpm-trace's
// own plugin context, and the locale must be UTF-8 or Unicode trees break.
func TestChildEnvDropsResolutionFileAndForcesUTF8(t *testing.T) {
	t.Setenv("DPM_RESOLUTION_FILE", "/some/resolution.json")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "C")

	env := envMap(ChildEnv(nil))

	if _, present := env["DPM_RESOLUTION_FILE"]; present {
		t.Error("DPM_RESOLUTION_FILE must not reach the child")
	}
	if env["LC_ALL"] != "C.UTF-8" || env["LANG"] != "C.UTF-8" {
		t.Errorf("locale = LC_ALL=%q LANG=%q, want C.UTF-8", env["LC_ALL"], env["LANG"])
	}
}

// An existing UTF-8 locale is left alone: overriding it would change the
// child's behaviour for no reason.
func TestChildEnvKeepsExistingUTF8Locale(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	env := envMap(ChildEnv(nil))
	if env["LC_ALL"] != "en_US.UTF-8" {
		t.Errorf("LC_ALL = %q, want it untouched", env["LC_ALL"])
	}
}

// Explicit extras win over the inherited environment.
func TestChildEnvExtrasOverride(t *testing.T) {
	t.Setenv("DPM_TRACE_IT_LEDGER", "http://inherited")
	env := envMap(ChildEnv(map[string]string{"DPM_TRACE_IT_LEDGER": "http://explicit"}))
	if env["DPM_TRACE_IT_LEDGER"] != "http://explicit" {
		t.Errorf("got %q, want the explicit value", env["DPM_TRACE_IT_LEDGER"])
	}
}

func envMap(entries []string) map[string]string {
	out := map[string]string{}
	for _, entry := range entries {
		if key, value, found := strings.Cut(entry, "="); found {
			out[key] = value
		}
	}
	return out
}

var _ = os.Environ

// FailureLocations turns a failed test's message into source coordinates: the
// Module:line:col Daml stamps at the submit call site (the "where"), plus the
// assertMsg literal matched back into the contract (the "why").
func TestFailureLocationsResolvesCoordinates(t *testing.T) {
	index := source.NewIndex()
	index.LoadDamlYAML(filepath.Join("..", "..", "tests/fixtures/source-pkg/daml.yaml"))

	message := "Test.FailureDemo:20:18: Aborted: Insufficient balance"
	locations, capped := FailureLocations(message, index, 5)
	if capped {
		t.Error("five locations should not cap this message")
	}
	if len(locations) == 0 {
		t.Fatalf("no locations resolved from %q", message)
	}
	first := locations[0]
	if !strings.HasSuffix(first.Path, "FailureDemo.daml") {
		t.Errorf("resolved %q, want FailureDemo.daml", first.Path)
	}
	if first.Line != 20 || first.Column != 18 {
		t.Errorf("resolved %d:%d, want 20:18", first.Line, first.Column)
	}
}

// maxLocations caps the list and reports that it did, so the caller can say
// "showing 1 of many" rather than implying the list is exhaustive.
func TestFailureLocationsCaps(t *testing.T) {
	index := source.NewIndex()
	index.LoadDamlYAML(filepath.Join("..", "..", "tests/fixtures/source-pkg/daml.yaml"))

	message := "FailureDemo:8:1: and FailureDemo:14:3: and FailureDemo:20:18:"
	locations, capped := FailureLocations(message, index, 1)
	if len(locations) > 1 {
		t.Errorf("got %d locations, want at most 1", len(locations))
	}
	if len(locations) == 1 && !capped {
		t.Error("capped must be true when candidates were dropped")
	}
}

// A message with no coordinates and no matching source resolves to nothing
// rather than guessing.
func TestFailureLocationsWithoutMatchesIsEmpty(t *testing.T) {
	index := source.NewIndex()
	locations, capped := FailureLocations("something went wrong", index, 5)
	if len(locations) != 0 || capped {
		t.Errorf("got %v (capped=%v), want none", locations, capped)
	}
}
