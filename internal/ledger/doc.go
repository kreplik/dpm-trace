// Package ledger is the JSON Ledger API client: update lookup, completions,
// prepare and submit-and-wait, plus bearer-token resolution.
//
// Ported from the original Python implementation: http_json (including the bounded-backoff
// retry on transient failures), ledger_update_by_id_body, load_update,
// fetch_completion_by_command_id, run_prepare, run_submit, join_url, and the
// token precedence in apply_config_defaults (flag, then env, then config file).
//
// The tool speaks JSON over HTTP only -- no gRPC, no protobuf dependency.
package ledger
