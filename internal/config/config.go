// Package config discovers and applies .dpm-trace.json defaults.
//
// Ports find_config, load_config, apply_config_defaults and their helpers.
// #7 lists ".dpm-trace.json discovery and config-default application" as a
// compatibility contract: a user with a config file must get the same behavior
// from either implementation.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// ConfigFileName is the per-project config file.
const ConfigFileName = ".dpm-trace.json"

// projectBoundaryMarkers bound the upward search. A stray config in an
// unrelated parent workspace must never inject ledger settings into a
// subproject, so the walk stops at the nearest directory holding one of these.
var projectBoundaryMarkers = []string{".git", "daml.yaml", "component.yaml"}

// Find locates the config for the current directory. An explicit path is
// honored as-is. Ports find_config.
func Find(explicit string) string {
	if explicit != "" {
		return explicit
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return ""
	}

	projectRoot := cwd
	for _, dir := range ancestors(cwd) {
		if hasBoundaryMarker(dir) {
			projectRoot = dir
			break
		}
	}

	for _, dir := range ancestors(cwd) {
		candidate := filepath.Join(dir, ConfigFileName)
		if fileExists(candidate) {
			return candidate
		}
		if dir == projectRoot {
			break
		}
	}
	return ""
}

// Load reads the discovered config. A missing implicit config is not an error;
// a missing explicit one is. Ports load_config.
func Load(explicit string) (*model.Object, error) {
	path := Find(explicit)
	if path == "" {
		return model.NewObject(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if explicit != "" {
			return nil, fmt.Errorf("config file not found: %s", explicit)
		}
		return model.NewObject(), nil
	}
	obj, err := model.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON config %s: %w", path, err)
	}
	if explicit == "" && os.Getenv("DPM_TRACE_VERBOSE") != "" {
		fmt.Fprintf(os.Stderr, "dpm trace: using config %s\n", path)
	}
	return obj, nil
}

// String applies the precedence in apply_config_defaults: an explicit flag
// wins, then the environment variable, then the config keys in order.
func String(current string, config *model.Object, env string, keys ...string) string {
	if current != "" {
		return current
	}
	if env != "" {
		if value := os.Getenv(env); value != "" {
			return value
		}
	}
	for _, key := range keys {
		if value := model.ObjectString(config, key); value != "" {
			return value
		}
	}
	return ""
}

// Strings is String for list-valued settings. A scalar in the config becomes a
// single-element list, matching config_values.
func Strings(current []string, config *model.Object, env string, keys ...string) []string {
	if len(current) > 0 {
		return current
	}
	if env != "" {
		if value := os.Getenv(env); value != "" {
			// One value, not a PATH-style list: config_values wraps a scalar
			// as a single-element list, so a path containing a colon stays
			// whole.
			return []string{value}
		}
	}
	for _, key := range keys {
		if values := model.ObjectStrings(config, key); len(values) > 0 {
			return values
		}
	}
	return nil
}

func ancestors(dir string) []string {
	var out []string
	for {
		out = append(out, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			return out
		}
		dir = parent
	}
}

func hasBoundaryMarker(dir string) bool {
	for _, marker := range projectBoundaryMarkers {
		if fileExists(filepath.Join(dir, marker)) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
