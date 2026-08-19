package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newPackage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	content := "sdk-version: 3.4.11\nname: demo-app\nsource: daml\nversion: 1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "daml.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInitWritesTheLayout(t *testing.T) {
	root := newPackage(t)
	result, err := Init(&bytes.Buffer{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"itests/lit.cfg.py",
		"itests/example.test",
		"unittests/daml.yaml",
		"unittests/daml/Example.daml",
		".github/workflows/dpm-trace.yml",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s not written", rel)
		}
	}
	if len(result.Created) != 5 {
		t.Errorf("created %d files, want 5: %v", len(result.Created), result.Created)
	}
}

// --init is expected to be re-runnable: existing files are kept, never
// overwritten, so a user's edits survive.
func TestInitKeepsExistingFiles(t *testing.T) {
	root := newPackage(t)
	if _, err := Init(&bytes.Buffer{}, Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "itests", "example.test")
	if err := os.WriteFile(marker, []byte("# edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Init(&bytes.Buffer{}, Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != 0 {
		t.Errorf("second run created %v, want nothing", result.Created)
	}
	if len(result.Kept) != 5 {
		t.Errorf("kept %d, want 5", len(result.Kept))
	}
	data, _ := os.ReadFile(marker)
	if string(data) != "# edited by hand\n" {
		t.Error("an existing file was overwritten")
	}
}

func TestInitSkipsOptionalParts(t *testing.T) {
	root := newPackage(t)
	if _, err := Init(&bytes.Buffer{}, Options{Root: root, NoCI: true, NoUnittests: true}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".github/workflows/dpm-trace.yml", "unittests/daml.yaml"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("%s was written despite being disabled", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "itests", "lit.cfg.py")); err != nil {
		t.Error("itests should still be written")
	}
}

// The package name and sdk-version come from the target package's daml.yaml.
func TestInitSubstitutesPackageFields(t *testing.T) {
	root := newPackage(t)
	if _, err := Init(&bytes.Buffer{}, Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "unittests", "daml.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "sdk-version: 3.4.11") {
		t.Errorf("sdk-version not substituted:\n%s", text)
	}
	if !strings.Contains(text, "demo-app-unittests") {
		t.Errorf("package name not substituted:\n%s", text)
	}
	if strings.Contains(text, "{{") {
		t.Errorf("unrendered template field left in output:\n%s", text)
	}
}

func TestInitRejectsNonPackage(t *testing.T) {
	if _, err := Init(&bytes.Buffer{}, Options{Root: t.TempDir()}); err == nil {
		t.Fatal("expected an error")
	}
}

// Every embedded template must render without leftover fields.
func TestAllTemplatesRender(t *testing.T) {
	entries, err := templates.ReadDir("templates")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no templates embedded")
	}
	data := fields{PackageName: "p", SDKVersion: "1.0.0", ITestsDir: "itests", UnitTestsDir: "unittests", WithUnittests: true}
	for _, entry := range entries {
		rendered, err := render(entry.Name(), data)
		if err != nil {
			t.Errorf("%s: %v", entry.Name(), err)
			continue
		}
		if bytes.Contains(rendered, []byte("{{")) {
			t.Errorf("%s has an unrendered field", entry.Name())
		}
	}
}
