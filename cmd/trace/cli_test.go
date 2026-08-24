package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These drive the subcommand entry points in-process, so the shipping CLI
// surface is gated by `go test` alone. The golden harness covers the same
// ground byte-for-byte, but it exists to prove parity with the Python
// implementation; these must keep working once that oracle is retired.

// capture runs fn with stdout and stderr redirected, returning what was written
// to each and the exit code fn reported.
func capture(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	realOut, realErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	outCh, errCh := make(chan string, 1), make(chan string, 1)
	go func() { data, _ := io.ReadAll(outR); outCh <- string(data) }()
	go func() { data, _ := io.ReadAll(errR); errCh <- string(data) }()

	func() {
		defer func() {
			os.Stdout, os.Stderr = realOut, realErr
			outW.Close()
			errW.Close()
		}()
		code = fn()
	}()

	return <-outCh, <-errCh, code
}

func repoPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func TestOpenRendersAnArtifact(t *testing.T) {
	stdout, stderr, code := capture(t, func() int {
		return runOpen([]string{repoPath("examples", "create.trace.json"), "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Trace artifact", "CREATE Asset:Asset", "signatories:", "quantity: 100"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}

func TestOpenPrintJSONEmitsTheArtifact(t *testing.T) {
	stdout, _, code := capture(t, func() int {
		return runOpen([]string{repoPath("examples", "create.trace.json"), "--print-json"})
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var artifact map[string]any
	if err := json.Unmarshal([]byte(stdout), &artifact); err != nil {
		t.Fatalf("--print-json did not emit valid JSON: %v", err)
	}
	if artifact["schema"] != "dpm-trace/trace-artifact/v0" {
		t.Errorf("schema = %v", artifact["schema"])
	}
}

// A missing file and a file that is not an artifact must both fail with a
// message on stderr, not a panic and not a silent success.
func TestOpenRejectsBadInput(t *testing.T) {
	for _, tc := range []struct{ name, path string }{
		{"missing", repoPath("examples", "does-not-exist.json")},
		{"not an artifact", repoPath("tests", "fixtures", "compare", "completion-fail.json")},
	} {
		stdout, stderr, code := capture(t, func() int {
			return runOpen([]string{tc.path, "--color", "never"})
		})
		if code == 0 {
			t.Errorf("%s: exit 0, want failure (stdout: %s)", tc.name, stdout)
		}
		if !strings.Contains(stderr, "error:") {
			t.Errorf("%s: stderr = %q, want an error", tc.name, stderr)
		}
	}
}

func TestCompareUpdateVsUpdate(t *testing.T) {
	stdout, stderr, code := capture(t, func() int {
		return runCompare([]string{
			repoPath("tests", "fixtures", "compare", "trace-a.json"),
			repoPath("tests", "fixtures", "compare", "trace-b.json"),
			"--full", "--color", "never",
		})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"DPM trace comparison", "update-vs-update", "Event counts", "Root events"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}

func TestComparePreparedVsUpdate(t *testing.T) {
	stdout, stderr, code := capture(t, func() int {
		return runCompare([]string{
			"--prepared", repoPath("examples", "transfer.prepared.json"),
			"--update", repoPath("examples", "transfer.trace.json"),
			"--full", "--color", "never",
		})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"prepared-vs-update", "Operation", "Field diff", "prep hash"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}

func TestComparePreparedVsCompletion(t *testing.T) {
	stdout, _, code := capture(t, func() int {
		return runCompare([]string{
			"--prepared", repoPath("tests", "fixtures", "compare", "prepared.json"),
			"--completion-file", repoPath("tests", "fixtures", "compare", "completion-fail.json"),
			"--color", "never",
		})
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout, "prepared-vs-completion") {
		t.Errorf("wrong comparison kind:\n%s", stdout)
	}
}

// A completion is not a committed transaction: the failed-submission workflow
// must render from completion data with no update id.
func TestTraceFromCompletionFile(t *testing.T) {
	stdout, stderr, code := capture(t, func() int {
		return runTrace([]string{
			"--completion-file", repoPath("examples", "failed-withdraw.completion.json"),
			"--daml-yaml", repoPath("examples", "asset", "daml.yaml"),
			"--color", "never",
		})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"completion failed", "Insufficient balance", "Source diagnostics", "Asset.daml"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}

// Argument validation must fail before any network call is attempted.
func TestSubcommandsRejectMissingArguments(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func() int
		want string
	}{
		{"trace without a target", func() int { return runTrace([]string{"--color", "never"}) },
			"update id"},
		{"compare without operands", func() int { return runCompare([]string{"--color", "never"}) },
			"error:"},
		{"prepare without a ledger", func() int { return runPrepare([]string{"--act-as", "Alice", "--template", "T:T"}) },
			"error:"},
		{"submit without a ledger", func() int { return runSubmit([]string{"--act-as", "Alice", "--template", "T:T"}) },
			"error:"},
	} {
		_, stderr, code := capture(t, tc.run)
		if code == 0 {
			t.Errorf("%s: exit 0, want failure", tc.name)
		}
		if !strings.Contains(stderr, tc.want) {
			t.Errorf("%s: stderr = %q, want it to mention %q", tc.name, stderr, tc.want)
		}
	}
}

// An unknown flag must be reported rather than ignored, or a typo silently
// changes what the command does.
func TestUnknownFlagIsRejected(t *testing.T) {
	_, stderr, code := capture(t, func() int {
		return runTrace([]string{"1220abcd", "--not-a-flag"})
	})
	if code == 0 {
		t.Fatal("exit 0 for an unknown flag")
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("stderr = %q, want an unknown-flag error", stderr)
	}
}

// install-plugin writes the component and registers it in the SDK manifest.
// Both halves matter: without the manifest entry `dpm trace` does not resolve.
func TestInstallPluginRegistersTheComponent(t *testing.T) {
	home := t.TempDir()
	manifestDir := filepath.Join(home, "cache", "sdk", "open-source")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(manifestDir, "3.5.1.yaml")
	content := "apiVersion: digitalasset.com/v1\nkind: SdkManifest\nspec:\n  components:\n  assistant:\n    version: 1.0.0\n  version: 3.5.1\n  edition: open-source\n"
	if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := capture(t, func() int {
		return runInstallPlugin([]string{"--dpm-home", home, "--sdk-version", "3.5.1", "--component-version", "0.1.0"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Registered") {
		t.Errorf("stdout = %q", stdout)
	}

	component := filepath.Join(home, "cache", "components", "dpm-trace", "0.1.0")
	if _, err := os.Stat(filepath.Join(component, "component.yaml")); err != nil {
		t.Errorf("component.yaml not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(component, "bin", "dpm-trace")); err != nil {
		t.Errorf("binary not installed: %v", err)
	}
	updated, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), "dpm-trace:") {
		t.Errorf("manifest not updated:\n%s", updated)
	}
}

// --help must succeed for every subcommand: a non-zero exit here breaks
// `dpm trace <cmd> --help` for users.
func TestSubcommandHelpExits0(t *testing.T) {
	for name, run := range map[string]func() int{
		"open":           func() int { return runOpen([]string{"--help"}) },
		"compare":        func() int { return runCompare([]string{"--help"}) },
		"prepare":        func() int { return runPrepare([]string{"--help"}) },
		"submit":         func() int { return runSubmit([]string{"--help"}) },
		"test":           func() int { return runTest([]string{"--help"}) },
		"install-plugin": func() int { return runInstallPlugin([]string{"--help"}) },
	} {
		stdout, stderr, code := capture(t, run)
		if code != 0 {
			t.Errorf("%s --help exited %d (stderr: %s)", name, code, stderr)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Errorf("%s --help printed no usage:\n%s", name, stdout)
		}
	}
}

// A CantonScan update URL is accepted wherever an update id is, so a user can
// paste a link straight from the explorer.
func TestExtractUpdateID(t *testing.T) {
	const id = "1220c8f9d9b2ca3aa72aae3eee228cb84e30e559dfdcfc2b7189def41a3b4589b6d5"
	for _, in := range []string{
		id,
		"https://scan.example.com/update/" + id,
		"https://scan.example.com/update/" + id + "?tab=events",
		"https://scan.example.com/update/" + id + "#events",
	} {
		if got := extractUpdateID(in); got != id {
			t.Errorf("extractUpdateID(%q) = %q", in, got)
		}
	}
	// Anything unrecognised passes through, so the ledger reports the error.
	if got := extractUpdateID("not-an-id"); got != "not-an-id" {
		t.Errorf("got %q", got)
	}
}

func TestSmallHelpers(t *testing.T) {
	if got := orDefaultString("", "daml"); got != "daml" {
		t.Errorf("orDefaultString(empty) = %q", got)
	}
	if got := orDefaultString("damlc", "daml"); got != "damlc" {
		t.Errorf("orDefaultString(set) = %q", got)
	}
	if got := firstOrEmpty(nil); got != "" {
		t.Errorf("firstOrEmpty(nil) = %q", got)
	}
	if got := firstOrEmpty([]string{"a", "b"}); got != "a" {
		t.Errorf("firstOrEmpty = %q", got)
	}
}

// resolvePath must make a path absolute: the test runner spawns daml from a
// temporary working directory where a relative path would not resolve.
func TestResolvePath(t *testing.T) {
	got, err := resolvePath(".")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolvePath(.) = %q, want absolute", got)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if got, err := resolvePath("~"); err != nil || got == "~" {
			t.Errorf("tilde not expanded: %q (%v)", got, err)
		} else if !strings.HasPrefix(got, filepath.Dir(home)) && got != home {
			t.Logf("resolvePath(~) = %q (symlinks resolved)", got)
		}
	}
}

// An empty party value means the caller's variable was unset. Dropping it
// silently gave a narrower projection than was asked for, with nothing in the
// output naming the parties that were requested rather than used -- so a
// mistyped $ALICE produced a plausible answer to a different question.
func TestPartyFlagsRejectAnEmptyValue(t *testing.T) {
	artifact := repoPath("examples", "create.trace.json")

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"trace read-as", []string{"some-update-id", "--submitter", "http://127.0.0.1:1", "--read-as", ""}},
		{"trace party", []string{"some-update-id", "--submitter", "http://127.0.0.1:1", "--party", ""}},
		// The mixed case is the one that bit: one good party hides one empty.
		{"trace mixed", []string{"some-update-id", "--submitter", "http://127.0.0.1:1", "--read-as", "", "--read-as", "Alice::1220aa"}},
		{"compare read-as", []string{"--update", artifact, "--submitter", "http://127.0.0.1:1", "--read-as", ""}},
		{"compare act-as", []string{"--update", artifact, "--submitter", "http://127.0.0.1:1", "--act-as", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			var stderr string
			if strings.HasPrefix(tc.name, "compare") {
				_, stderr, code = capture(t, func() int { return runCompare(tc.args) })
			} else {
				_, stderr, code = capture(t, func() int { return runTrace(tc.args) })
			}
			if code == 0 {
				t.Errorf("empty party accepted, exit 0")
			}
			if !strings.Contains(stderr, "requires a party id") {
				t.Errorf("stderr = %q, want a party-id error", stderr)
			}
		})
	}
}
