// Package scaffold implements `dpm trace test --init`, which writes the itests/
// and unittests/ layout and a CI workflow into a user's Daml package.
//
// Ports from src/dpm_trace/cli.py: run_init, integration_lit_cfg_text,
// integration_example_test_text, unit_test_daml_yaml_text,
// unit_test_example_text, ci_workflow_text.
//
// The generated lit.cfg.py stays Python: it configures lit, which is a Python
// tool. Only the code that emits it moves to Go.
package scaffold
