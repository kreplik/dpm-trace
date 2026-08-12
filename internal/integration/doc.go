// Package integration runs an lit suite against a managed local Canton node.
//
// Ports from src/dpm_trace/cli.py: run_integration_tests, canton_config_text,
// canton_bootstrap_text, find_free_ports, parse_party_placements,
// wait_for_parties, build_dar, daml_child_env.
//
// lit and FileCheck stay external tools we invoke; they are not ported. The
// child environment must still drop DPM_RESOLUTION_FILE and force a UTF-8
// locale, or spawned daml/damlc resolve against this component's plugin
// context instead of the target package.
package integration
