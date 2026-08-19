// Package model holds the transaction model and the normalization that turns a
// Ledger API response into it, plus the on-disk artifact encodings.
//
// Ported from the original Python implementation: TraceEvent, NormalizedTrace,
// EVENT_VARIANT_KINDS, normalize_trace, normalize_event, normalize_events_map,
// unwrap_transaction, infer_roots, link_range_children, normalize_completion,
// trace_to_json, trace_from_json, event_to_json, event_from_json,
// load_trace_artifact, load_prepared_artifact.
//
// Porting notes:
//   - The Python code reads responses as untyped dicts and probes several key
//     spellings per field (pick()). Decode into map[string]any rather than
//     structs, or the tolerance is lost. Field spellings are not guesses: see
//     tests/fixtures/reassignment for a case where they differ from the docs.
//   - Decode with json.Decoder.UseNumber. Offsets and reassignment counters are
//     large integers and float64 both loses precision and reformats them.
//   - Artifact JSON is emitted by Python with sort_keys=True. encoding/json
//     sorts map keys but emits struct fields in declaration order, so encode
//     artifacts through maps or declare fields alphabetically.
//   - Encode with SetEscapeHTML(false); Go escapes <, > and & by default and
//     Python does not.
package model
