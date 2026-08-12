package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The committed HTML fixture is what `daml test` actually writes.
func TestTransactionHTMLToTextMatchesPython(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests/fixtures/transaction-testTransfer.html"))
	if err != nil {
		t.Fatal(err)
	}
	text := TransactionHTMLToText(string(data))

	if strings.Contains(text, "<") || strings.Contains(text, "&lt;") {
		t.Errorf("markup survived decoding:\n%s", firstLines(text, 5))
	}
	if strings.HasPrefix(text, "\n") || strings.HasSuffix(text, "\n") {
		t.Error("leading or trailing blank lines were not trimmed")
	}
	if !strings.Contains(text, "TX") {
		t.Errorf("no transaction lines decoded:\n%s", firstLines(text, 5))
	}
}

func TestTransactionStatsCountsTree(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests/fixtures/transaction-testTransfer.html"))
	if err != nil {
		t.Fatal(err)
	}
	stats := TransactionStats(TransactionHTMLToText(string(data)))
	if stats["transactions"] == 0 {
		t.Errorf("no transactions counted: %v", stats)
	}
	for _, key := range []string{"transactions", "creates", "exercises", "archives", "expectedFailures"} {
		if _, ok := stats[key]; !ok {
			t.Errorf("stats missing %q", key)
		}
	}
}

func TestParseJUnitStatuses(t *testing.T) {
	xml := `<?xml version="1.0"?>
<testsuites>
  <testsuite name="Test">
    <testcase name="ok" classname="Test" time="0.5"/>
    <testcase name="bad" classname="Test"><failure message="boom">detail</failure></testcase>
    <testcase name="broken" classname="Test"><error message="crash"/></testcase>
    <testcase name="gone" classname="Test"><skipped/></testcase>
  </testsuite>
</testsuites>`
	path := filepath.Join(t.TempDir(), "results.xml")
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}

	cases, err := ParseJUnit(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 4 {
		t.Fatalf("cases = %d, want 4", len(cases))
	}
	want := []string{StatusPassed, StatusFailed, StatusError, StatusSkipped}
	for i, status := range want {
		if cases[i].Status != status {
			t.Errorf("case %d status = %q, want %q", i, cases[i].Status, status)
		}
	}
	if cases[1].Message != "boom" {
		t.Errorf("message = %q, want the failure attribute", cases[1].Message)
	}
	if cases[0].Time == nil || *cases[0].Time != 0.5 {
		t.Errorf("time = %v, want 0.5", cases[0].Time)
	}
	// The CI gate depends on this: only failed and error count against it.
	if cases[0].Failed() || cases[3].Failed() {
		t.Error("passed and skipped must not count as failures")
	}
	if !cases[1].Failed() || !cases[2].Failed() {
		t.Error("failed and error must count as failures")
	}
}

// A failure with no message attribute falls back to the element text.
func TestParseJUnitFallsBackToElementText(t *testing.T) {
	xml := `<testsuites><testsuite name="T"><testcase name="a"><failure>  text detail  </failure></testcase></testsuite></testsuites>`
	path := filepath.Join(t.TempDir(), "r.xml")
	if err := os.WriteFile(path, []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	cases, err := ParseJUnit(path)
	if err != nil {
		t.Fatal(err)
	}
	if cases[0].Message != "text detail" {
		t.Errorf("message = %q", cases[0].Message)
	}
}

func TestTransactionLocationsDeduplicatesInOrder(t *testing.T) {
	text := "foo (Test:12:3) bar (Asset:40:5) baz (Test:12:3)"
	got := TransactionLocations(text)
	want := []string{"Test:12:3", "Asset:40:5"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func firstLines(text string, n int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
