package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// The walk stops at the project boundary: a config in an unrelated parent
// workspace must never inject ledger settings into a subproject.
func TestFindStopsAtProjectBoundary(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ConfigFileName), `{"ledgerUrl": "http://parent"}`)
	project := filepath.Join(root, "project")
	write(t, filepath.Join(project, ".git", "HEAD"), "ref: refs/heads/main\n")
	if err := os.MkdirAll(filepath.Join(project, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	chdir(t, filepath.Join(project, "sub"))
	if found := Find(""); found != "" {
		t.Errorf("found %q; the parent config is outside the project boundary", found)
	}
}

func TestFindWalksUpToTheBoundary(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(root, ConfigFileName), `{"ledgerUrl": "http://project"}`)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	chdir(t, filepath.Join(root, "sub"))
	found := Find("")
	if found == "" {
		t.Fatal("expected to find the project config")
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := String("", cfg, "", "ledgerUrl"); got != "http://project" {
		t.Errorf("ledgerUrl = %q", got)
	}
}

// An explicit flag beats the environment, which beats the config file.
func TestPrecedence(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref\n")
	write(t, filepath.Join(root, ConfigFileName), `{"ledgerUrl": "http://from-config"}`)
	chdir(t, root)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := String("http://from-flag", cfg, "DPM_TRACE_LEDGER_URL", "ledgerUrl"); got != "http://from-flag" {
		t.Errorf("flag should win, got %q", got)
	}
	t.Setenv("DPM_TRACE_LEDGER_URL", "http://from-env")
	if got := String("", cfg, "DPM_TRACE_LEDGER_URL", "ledgerUrl"); got != "http://from-env" {
		t.Errorf("env should beat config, got %q", got)
	}
}

// A scalar in the config becomes a single-element list, matching config_values.
func TestStringsAcceptsScalarOrList(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "HEAD"), "ref\n")
	write(t, filepath.Join(root, ConfigFileName), `{"readAs": "Alice::1220ab", "darPaths": ["a.dar", "b.dar"]}`)
	chdir(t, root)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := Strings(nil, cfg, "", "readAs"); len(got) != 1 || got[0] != "Alice::1220ab" {
		t.Errorf("readAs = %v", got)
	}
	if got := Strings(nil, cfg, "", "darPaths"); len(got) != 2 {
		t.Errorf("darPaths = %v", got)
	}
}

func TestExplicitMissingConfigIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("an explicit missing config must be an error")
	}
}
