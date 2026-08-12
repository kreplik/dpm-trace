package testrunner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/source"
)

var (
	// Module:line[:col] coordinates Daml stamps into failure messages.
	coordinatePattern = regexp.MustCompile(`([A-Za-z0-9_.']+):(\d+)(?::(\d+))?`)
	cantonCategory    = regexp.MustCompile(`\s+Using Canton Error Category.*$`)
)

// FailureLocations resolves a failed test's message to source.
//
// Daml stamps the submit call site as Module:line:col -- the "where". The
// message body usually also carries the assertMsg/abort literal that caused the
// rejection, which matches back into the contract -- the "why". Both are
// surfaced, call sites first, because a red test needs both to be actionable.
//
// capped reports that more candidates existed than were returned, so the caller
// can say the list was truncated rather than exhaustive.
// Ports test_failure_locations.
func FailureLocations(message string, index *source.Index, maxLocations int) (locations []source.Location, capped bool) {
	seen := map[string]bool{}
	add := func(loc source.Location) {
		key := fmt.Sprintf("%s\x00%d\x00%d", loc.Path, loc.Line, loc.Column)
		if seen[key] {
			return
		}
		seen[key] = true
		if len(locations) >= maxLocations {
			capped = true
			return
		}
		locations = append(locations, loc)
	}

	for _, match := range coordinatePattern.FindAllStringSubmatch(message, -1) {
		module := match[1]
		files := index.ModuleFiles(module)
		if len(files) == 0 {
			continue
		}
		line, err := strconv.Atoi(match[2])
		if err != nil {
			continue
		}
		column := 1
		if match[3] != "" {
			if parsed, err := strconv.Atoi(match[3]); err == nil {
				column = parsed
			}
		}
		for _, path := range files {
			add(source.Location{Path: path, Line: line, Label: "daml test: " + module, Column: column})
		}
	}

	for _, needle := range source.Needles(StripErrorDecoration(message)) {
		for _, loc := range index.FindFailureText(needle, 5) {
			add(loc)
		}
	}
	return locations, capped
}

// StripErrorDecoration reduces a Daml/Canton failure message to the
// user-authored text, so the source search matches the literal in the contract
// rather than the surrounding error machinery.
// Ports strip_canton_error_decoration.
func StripErrorDecoration(message string) string {
	text := strings.TrimSpace(cantonCategory.ReplaceAllString(message, ""))
	for _, marker := range []string{"AssertionFailed:", "Aborted:", "Failed with status:"} {
		if index := strings.LastIndex(text, marker); index >= 0 {
			text = strings.TrimSpace(text[index+len(marker):])
		}
	}
	return text
}

var summaryPattern = regexp.MustCompile(
	`^\S*:([A-Za-z0-9_']+):\s*ok,\s*(\d+)\s+active contracts?,\s*(\d+)\s+transactions?`)

// ParseSummary reads per-test stats from `daml test`'s plain-text summary.
//
// This is the fallback when the transaction HTML could not be written: the
// summary is ASCII, so it survives environments where the Unicode trees do not.
// Ports parse_test_summary.
func ParseSummary(stdout string) map[string]map[string]int {
	result := map[string]map[string]int{}
	for _, line := range strings.Split(stdout, "\n") {
		match := summaryPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		active, _ := strconv.Atoi(match[2])
		transactions, _ := strconv.Atoi(match[3])
		result[match[1]] = map[string]int{
			"transactions":    transactions,
			"activeContracts": active,
		}
	}
	return result
}
