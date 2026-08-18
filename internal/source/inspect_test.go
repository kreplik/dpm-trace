package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The inspect invocation differs by executable: damlc takes the subcommand
// directly, the assistant needs "damlc" inserted first. Getting this wrong
// fails on one toolchain only.
func TestDamlcInspectCommand(t *testing.T) {
	got := DamlcInspectCommand("/sdk/bin/damlc", "/pkg/app.dar")
	want := []string{"/sdk/bin/damlc", "inspect", "/pkg/app.dar", "--detail", "2"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("damlc: got %v, want %v", got, want)
	}

	got = DamlcInspectCommand("/sdk/bin/daml", "/pkg/app.dar")
	want = []string{"/sdk/bin/daml", "damlc", "inspect", "/pkg/app.dar", "--detail", "2"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("assistant: got %v, want %v", got, want)
	}
}

// A tilde must expand, or the spawned process gets a path that does not exist.
func TestDamlcInspectCommandExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got := DamlcInspectCommand("~/.daml/bin/damlc", "/pkg/app.dar")
	if got[0] != filepath.Join(home, ".daml/bin/damlc") {
		t.Errorf("executable = %q, want the tilde expanded", got[0])
	}
	// A bare name must stay bare so PATH lookup still works.
	if got := DamlcInspectCommand("daml", "/pkg/app.dar"); got[0] != "daml" {
		t.Errorf("executable = %q, want it left on PATH", got[0])
	}
}

// Same contract as the test runner's ChildEnv: the inspect subprocess must not
// inherit dpm-trace's own package resolution, and needs a UTF-8 locale.
func TestChildEnvDropsResolutionFileAndForcesUTF8(t *testing.T) {
	t.Setenv("DPM_RESOLUTION_FILE", "/some/resolution.json")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "")
	t.Setenv("LANG", "C")

	env := map[string]string{}
	for _, entry := range childEnv() {
		if key, value, found := strings.Cut(entry, "="); found {
			env[key] = value
		}
	}

	if _, present := env["DPM_RESOLUTION_FILE"]; present {
		t.Error("DPM_RESOLUTION_FILE must not reach damlc")
	}
	if env["LC_ALL"] != "C.UTF-8" {
		t.Errorf("LC_ALL = %q, want C.UTF-8", env["LC_ALL"])
	}
}

func TestChildEnvKeepsExistingUTF8Locale(t *testing.T) {
	t.Setenv("LC_ALL", "en_US.UTF-8")
	for _, entry := range childEnv() {
		if key, value, found := strings.Cut(entry, "="); found && key == "LC_ALL" && value != "en_US.UTF-8" {
			t.Errorf("LC_ALL = %q, want it untouched", value)
		}
	}
}

// ModuleFiles backs failure resolution: a message naming a module has to find
// the files declaring it, and an unknown module must return nothing rather
// than every file.
func TestModuleFilesAndLines(t *testing.T) {
	ix := NewIndex()
	ix.LoadDamlYAML(filepath.Join("..", "..", "tests/fixtures/source-pkg/daml.yaml"))

	files := ix.ModuleFiles("FailureDemo")
	if len(files) == 0 {
		t.Fatal("FailureDemo resolved to no files")
	}
	if !strings.HasSuffix(files[0], "FailureDemo.daml") {
		t.Errorf("resolved %q", files[0])
	}
	if got := ix.ModuleFiles("NoSuchModule"); len(got) != 0 {
		t.Errorf("unknown module resolved to %v", got)
	}

	lines, ok := ix.Lines(files[0])
	if !ok || len(lines) == 0 {
		t.Fatalf("Lines(%q) returned no content", files[0])
	}
	if _, ok := ix.Lines("/not/loaded.daml"); ok {
		t.Error("Lines reported content for a file that was never loaded")
	}
}
