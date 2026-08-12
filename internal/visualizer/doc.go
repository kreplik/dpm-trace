// Package visualizer is the interactive --visualize REPL.
//
// Ports from src/dpm_trace/cli.py: Stepper (order, show_current, show_tree,
// show_source, step_variables, jump), Breakpoint, ExpressionStep,
// eval_daml_expression and _eval_replay.
//
// Port this last. The expression replay is the only part with no direct Go
// analogue -- it leans on Python evaluation and needs a small self-contained
// evaluator with identical observable behavior, including the evaluated=false
// path that marks steps the evaluator could not reduce.
package visualizer
