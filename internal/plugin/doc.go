// Package plugin registers the binary as a DPM component so it runs as
// `dpm trace`.
//
// Ported from the original Python implementation: install_plugin_main, DPM home discovery
// ($DPM_HOME or ~/.dpm), component.yaml generation and SDK manifest
// registration.
//
// The port simplifies this: the manifest entry points at a single binary
// instead of a bash shim wrapping a Python module.
package plugin
