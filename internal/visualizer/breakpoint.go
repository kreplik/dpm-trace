// Package visualizer is the interactive --visualize REPL.
//
// Per #7 this is the last piece ported, because the expression replay leans on
// Python evaluation. This file and stepper.go cover navigation, breakpoints and
// event display; the replay is a separate step.
package visualizer

import (
	"strconv"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
	"github.com/walnuthq/dpm-trace/internal/render"
	"github.com/walnuthq/dpm-trace/internal/source"
)

// Breakpoint matches by step number, event id, template/choice, source label or
// file[:line]. Ports Breakpoint.
type Breakpoint struct {
	Spec string
}

// Matches reports whether a breakpoint stops at this step. loc may be nil when
// no source is available; the source-based forms then never match.
// Ports Breakpoint.matches.
func (b Breakpoint) Matches(step int, eventID string, ev *model.Event, loc *source.Location) bool {
	spec := strings.TrimSpace(b.Spec)
	if spec == "" {
		return false
	}
	lowered := strings.ToLower(spec)

	// step is zero-based; users count from 1.
	if spec == eventID || lowered == strings.ToLower("#"+eventID) || spec == strconv.Itoa(step+1) {
		return true
	}
	target := strings.ToLower(render.EventTarget(ev))
	if lowered == target || strings.Contains(target, lowered) {
		return true
	}
	if loc == nil {
		return false
	}
	label := strings.ToLower(loc.Label)
	if lowered == label || strings.Contains(label, lowered) {
		return true
	}

	if filePart, linePart, found := cutLast(spec, ":"); found && isDigits(linePart) {
		line, err := strconv.Atoi(linePart)
		if err != nil || line != loc.Line {
			return false
		}
		filePart = strings.TrimSpace(filePart)
		return filePart == "" || strings.HasSuffix(loc.Path, filePart)
	}
	return strings.HasSuffix(loc.Path, spec)
}

// cutLast splits around the last occurrence of sep, mirroring str.rpartition.
func cutLast(s, sep string) (before, after string, found bool) {
	index := strings.LastIndex(s, sep)
	if index < 0 {
		return "", s, false
	}
	return s[:index], s[index+len(sep):], true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
