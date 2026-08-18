// Package source maps failures back to Daml source: line, column and caret.
//
// Ported from the original Python implementation: SourceIndex (daml.yaml source discovery and
// damlc inspect integration), completion_source_needles,
// render_source_diagnostic, test_failure_locations, read_daml_yaml_field,
// format_source_path.
//
// Keep the precedence rule: prefer damlc inspect plus project metadata, with
// local source text matching only as a fallback.
package source
