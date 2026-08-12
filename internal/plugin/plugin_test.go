package plugin

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifestFixture = `apiVersion: digitalasset.com/v1
kind: SdkManifest
spec:
  components:
    canton-open-source:
      version: 3.5.1
    damlc:
      version: 3.5.1
  assistant:
    version: 1.0.17
  version: 3.5.1
  edition: open-source
`

func writeManifest(t *testing.T, home, sdkVersion string) string {
	t.Helper()
	dir := filepath.Join(home, "cache", "sdk", "open-source")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, sdkVersion+".yaml")
	if err := os.WriteFile(path, []byte(manifestFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The entry must land as the last component, immediately before `assistant:`.
func TestRegisterInManifestInsertsBeforeAssistant(t *testing.T) {
	home := t.TempDir()
	manifest := writeManifest(t, home, "3.5.1")

	if err := RegisterInManifest(manifest, "dpm-trace", "0.1.0"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")

	entry, assistant := -1, -1
	for i, line := range lines {
		if strings.HasPrefix(line, "    dpm-trace:") {
			entry = i
		}
		if strings.HasPrefix(line, "  assistant:") {
			assistant = i
		}
	}
	if entry < 0 {
		t.Fatalf("component not registered:\n%s", data)
	}
	if entry > assistant {
		t.Errorf("entry at %d is after assistant at %d", entry, assistant)
	}
	if lines[entry+1] != "      version: 0.1.0" {
		t.Errorf("version line = %q", lines[entry+1])
	}
}

// Re-running install must not duplicate the entry.
func TestRegisterInManifestIsIdempotent(t *testing.T) {
	home := t.TempDir()
	manifest := writeManifest(t, home, "3.5.1")

	for i := 0; i < 3; i++ {
		if err := RegisterInManifest(manifest, "dpm-trace", "0.1.0"); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(manifest)
	if count := strings.Count(string(data), "    dpm-trace:"); count != 1 {
		t.Errorf("entry appears %d times, want 1", count)
	}
}

func TestInstallWritesComponentAndBinary(t *testing.T) {
	home := t.TempDir()
	writeManifest(t, home, "3.5.1")

	binary := filepath.Join(t.TempDir(), "dpm-trace")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Install(&out, Options{
		DPMHome: home, SDKVersion: "3.5.1", ComponentVersion: "0.1.0", BinaryPath: binary,
	})
	if err != nil {
		t.Fatalf("install: %v", err)
	}

	componentDir := filepath.Join(home, "cache", "components", "dpm-trace", "0.1.0")
	descriptor, err := os.ReadFile(filepath.Join(componentDir, "component.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(descriptor), "name: trace") {
		t.Errorf("component.yaml does not declare the trace command:\n%s", descriptor)
	}

	// The installed binary must be executable, or dpm cannot run it.
	info, err := os.Stat(filepath.Join(componentDir, "bin", "dpm-trace"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("installed binary is not executable: %v", info.Mode())
	}
}

func TestInstallReportsMissingManifest(t *testing.T) {
	home := t.TempDir()
	err := Install(&bytes.Buffer{}, Options{DPMHome: home, SDKVersion: "9.9.9", ComponentVersion: "0.1.0"})
	if err == nil {
		t.Fatal("expected an error for a missing manifest")
	}
	if !strings.Contains(err.Error(), "SDK manifest not found") {
		t.Errorf("error = %v", err)
	}
}

func TestDetectSDKVersionPicksHighestInstalled(t *testing.T) {
	home := t.TempDir()
	writeManifest(t, home, "3.4.11")
	writeManifest(t, home, "3.5.1")
	if got := DetectSDKVersion(home); got != "3.5.1" {
		t.Errorf("DetectSDKVersion = %q, want 3.5.1", got)
	}
}
