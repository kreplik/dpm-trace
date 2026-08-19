// Package render produces every non-interactive line the CLI prints: the trace
// tree, completion/failure output and all comparison views.
//
// Ported from the original Python implementation: print_pretty_trace, print_event_tree,
// event_title, event_target, event_detail_lines, reassignment_detail_lines,
// event_kind_label, event_color, state_diff_summary, state_diff_counts,
// print_completion_trace, print_comparison and its helpers, Color,
// RenderContext, build_party_aliases, simplify_lf_value, render_pretty_value,
// short, short_template.
//
// Porting note: output here is pinned byte-for-byte by tests/golden. Iterating
// a Go map to produce output is a bug -- Python dicts preserve insertion order
// and Go map ranging is randomized. Drive ordered output from slices.
package render
