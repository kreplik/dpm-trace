// Package testrunner implements `dpm trace test`: it wraps `daml test`, decodes
// the per-script transaction trees and renders a report that gates CI.
//
// Ports from src/dpm_trace/cli.py: test_main, run_test, daml_test_command,
// parse_junit, transaction_html_to_text, transaction_stats, print_test_report,
// test_report_json.
//
// Contracts that must not change: a non-zero exit on any test failure, and the
// shapes of the dpm-trace/test-report/v0 JSON and the JUnit XML, which
// downstream consumers depend on.
//
// Porting notes: encoding/xml covers the JUnit parsing. The HTML decoding needs
// only html.UnescapeString plus the existing regexes -- no HTML parser and so
// no dependency outside the standard library.
package testrunner
