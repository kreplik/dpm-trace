// Package testrunner implements `dpm trace test`: it wraps `daml test`, decodes
// the per-script transaction trees and renders a report that gates CI.
//
// Two contracts must not change, per #7: a non-zero exit on any test failure,
// and the shape of the dpm-trace/test-report/v0 JSON and JUnit XML, which
// downstream consumers depend on.
package testrunner

import (
	"encoding/xml"
	"html"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/source"
)

// Case is one Daml Script test result.
type Case struct {
	Name              string
	Classname         string
	Status            string
	Message           string
	Time              *float64
	TransactionsText  string
	Stats             map[string]int
	TouchedLocations  []string
	Diagnostics       []source.Location
	DiagnosticsCapped bool
}

// Status values.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusError   = "error"
	StatusSkipped = "skipped"
)

// Failed reports whether the case counts against the CI gate.
func (c Case) Failed() bool { return c.Status == StatusFailed || c.Status == StatusError }

// junitSuites mirrors the JUnit XML `daml test` writes: a <testsuites> wrapper
// around one or more <testsuite> elements.
type junitSuites struct {
	Suites []junitSuite `xml:"testsuite"`
}

type junitSuite struct {
	Name  string      `xml:"name,attr"`
	Cases []junitCase `xml:"testcase"`
}

type junitCase struct {
	Name      string       `xml:"name,attr"`
	Classname string       `xml:"classname,attr"`
	Time      string       `xml:"time,attr"`
	Failure   *junitDetail `xml:"failure"`
	Error     *junitDetail `xml:"error"`
	Skipped   *struct{}    `xml:"skipped"`
}

type junitDetail struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// ParseJUnit reads the JUnit XML produced by `daml test`. Ports parse_junit,
// whose `iter("testsuite")` matches the root element as well as nested ones.
//
// Both document shapes are legal and both occur: `daml test` writes a
// <testsuites> wrapper, but other producers emit a bare <testsuite> root.
// Reading only the nested form yields zero cases for the latter, which would
// make a failing run exit 0 -- the CI gate silently inverted.
func ParseJUnit(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc junitSuites
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	suites := doc.Suites
	if len(suites) == 0 {
		// A bare <testsuite> root: unmarshal again against that shape.
		var root junitSuite
		if err := xml.Unmarshal(data, &root); err != nil {
			return nil, err
		}
		if len(root.Cases) > 0 {
			suites = []junitSuite{root}
		}
	}

	var results []Case
	for _, suite := range suites {
		for _, testcase := range suite.Cases {
			status := StatusPassed
			var detail *junitDetail
			switch {
			case testcase.Failure != nil:
				status, detail = StatusFailed, testcase.Failure
			case testcase.Error != nil:
				status, detail = StatusError, testcase.Error
			case testcase.Skipped != nil:
				status = StatusSkipped
			}

			message := ""
			if detail != nil {
				message = strings.TrimSpace(detail.Message)
				if message == "" {
					message = strings.TrimSpace(detail.Text)
				}
			}

			name := testcase.Name
			if name == "" {
				name = "?"
			}
			classname := testcase.Classname
			if classname == "" {
				classname = suite.Name
			}

			var seconds *float64
			if testcase.Time != "" {
				if parsed, err := strconv.ParseFloat(testcase.Time, 64); err == nil {
					seconds = &parsed
				}
			}

			results = append(results, Case{
				Name: name, Classname: classname, Status: status,
				Message: message, Time: seconds,
			})
		}
	}
	return results, nil
}

var (
	stylePattern = regexp.MustCompile(`(?s)<style.*?</style>`)
	brPattern    = regexp.MustCompile(`(?i)<br\s*/?>`)
	tagPattern   = regexp.MustCompile(`<[^>]+>`)
)

// TransactionHTMLToText decodes the IDE-only transaction HTML `daml test`
// writes. Ports transaction_html_to_text.
//
// Regex, not a parser: the input is `daml test`'s own generated markup, and the
// Python implementation is regex-based, so this matches it rather than being
// more correct and diverging.
func TransactionHTMLToText(htmlText string) string {
	text := stylePattern.ReplaceAllString(htmlText, "")
	text = brPattern.ReplaceAllString(text, "\n")
	text = tagPattern.ReplaceAllString(text, "")
	text = html.UnescapeString(text)

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t\r")
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

var (
	txPattern        = regexp.MustCompile(`(?m)^\s*TX\s+\d+`)
	createsPattern   = regexp.MustCompile(`\bcreates\b`)
	exercisesPattern = regexp.MustCompile(`\bexercises\b`)
	archivesPattern  = regexp.MustCompile(`consumed by:`)
	mustFailPattern  = regexp.MustCompile(`\bmustFailAt\b`)
	locationPattern  = regexp.MustCompile(`\(([A-Za-z0-9_.']+:\d+:\d+)\)`)
)

// TransactionStats counts what a decoded tree contains. Ports transaction_stats.
func TransactionStats(text string) map[string]int {
	return map[string]int{
		"transactions":     len(txPattern.FindAllString(text, -1)),
		"creates":          len(createsPattern.FindAllString(text, -1)),
		"exercises":        len(exercisesPattern.FindAllString(text, -1)),
		"archives":         len(archivesPattern.FindAllString(text, -1)),
		"expectedFailures": len(mustFailPattern.FindAllString(text, -1)),
	}
}

// TransactionLocations returns the source positions a tree mentions, in order
// of first appearance. Ports transaction_locations.
func TransactionLocations(text string) []string {
	var seen []string
	for _, match := range locationPattern.FindAllStringSubmatch(text, -1) {
		location := match[1]
		if !containsString(seen, location) {
			seen = append(seen, location)
		}
	}
	return seen
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
