package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/scaffold"
	"github.com/walnuthq/dpm-trace/internal/source"
	"github.com/walnuthq/dpm-trace/internal/testrunner"
)

func plain() Color { return Color{Enabled: false} }

// TestReport is the whole `dpm trace test` output. Its banner is what a
// developer reads to decide whether the run is green, so the counts and the
// per-case lines both have to be right.
func TestTestReportRendersResults(t *testing.T) {
	elapsed := 0.25
	cases := []testrunner.Case{
		{Name: "testIssue", Classname: "Test", Status: testrunner.StatusPassed, Time: &elapsed,
			Stats: map[string]int{"transactions": 1, "creates": 1}},
		{Name: "testFails", Classname: "Test", Status: testrunner.StatusFailed,
			Message: "Insufficient balance"},
	}

	var buf bytes.Buffer
	TestReport(&buf, "/pkg", []string{"daml", "test"}, cases, plain(), source.NewIndex(), false)
	out := buf.String()

	for _, want := range []string{"DPM trace test", "/pkg", "daml test", "testIssue", "testFails", "Insufficient balance"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// One of two failed, so the banner must not claim everything passed.
	if strings.Contains(out, "all 2 passed") {
		t.Errorf("banner claims success despite a failure:\n%s", out)
	}
}

func TestTestReportAllPassedBanner(t *testing.T) {
	cases := []testrunner.Case{
		{Name: "a", Status: testrunner.StatusPassed},
		{Name: "b", Status: testrunner.StatusPassed},
	}
	var buf bytes.Buffer
	TestReport(&buf, "/pkg", []string{"daml", "test"}, cases, plain(), source.NewIndex(), false)
	if !strings.Contains(buf.String(), "all 2 passed") {
		t.Errorf("banner = %q, want an all-passed summary", buf.String())
	}
}

func TestTestStatusIconAndStats(t *testing.T) {
	for _, status := range []string{testrunner.StatusPassed, testrunner.StatusFailed, testrunner.StatusError} {
		if icon := testStatusIcon(status, plain()); icon == "" {
			t.Errorf("no icon for %q", status)
		}
	}
	// Zero counts are omitted so a simple test does not render a row of zeroes.
	if got := testStatsText(map[string]int{"transactions": 0}, plain()); strings.Contains(got, "0 tx") {
		t.Errorf("zero counts should be omitted, got %q", got)
	}
	if got := testStatsText(map[string]int{"transactions": 2, "creates": 1}, plain()); got == "" {
		t.Error("non-zero stats rendered nothing")
	}
}

func TestTestResultBanner(t *testing.T) {
	if got := testResultBanner(3, 0, 3, plain()); !strings.Contains(got, "3") {
		t.Errorf("banner = %q", got)
	}
	failed := testResultBanner(1, 2, 3, plain())
	if !strings.Contains(failed, "2") {
		t.Errorf("failure banner does not report the count: %q", failed)
	}
}

// The displayed command is shortened for readability; it must keep the
// arguments a reader needs to reproduce the run.
func TestDisplayCommand(t *testing.T) {
	got := DisplayCommand([]string{"/long/path/to/daml", "test", "--junit", "/tmp/x.xml"})
	if got[0] != "daml" {
		t.Errorf("executable = %q, want the base name", got[0])
	}
	if strings.Join(got, " ") == "" {
		t.Error("empty display command")
	}
}

func TestBaseNameAndIndentBy(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/a/b/c.daml", "c.daml"}, {"c.daml", "c.daml"}, {"", ""},
	} {
		if got := baseName(tc.in); got != tc.want {
			t.Errorf("baseName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	got := indentBy("one\ntwo", "  ")
	if !strings.HasPrefix(got, "  one") || !strings.Contains(got, "\n  two") {
		t.Errorf("indentBy = %q", got)
	}
}

// The scaffolder report distinguishes created from kept, so re-running --init
// visibly does not clobber existing files.
func TestScaffoldReport(t *testing.T) {
	var buf bytes.Buffer
	result := scaffold.Result{
		Created: []string{"itests/lit.cfg.py"},
		Kept:    []string{"itests/example.test"},
	}
	ScaffoldReport(&buf, "/pkg", result, plain(), "itests", "unittests")
	out := buf.String()

	if !strings.Contains(out, "created") || !strings.Contains(out, "itests/lit.cfg.py") {
		t.Errorf("created files missing from:\n%s", out)
	}
	if !strings.Contains(out, "kept") || !strings.Contains(out, "already exists") {
		t.Errorf("kept files missing from:\n%s", out)
	}
}
