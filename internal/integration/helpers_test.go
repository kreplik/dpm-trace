package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ReadTail surfaces the end of a Canton log when startup fails, so it must
// return something useful for a short file and never blow up on a missing one.
func TestReadTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "canton.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := ReadTail(path, 2); got != "four\nfive" {
		t.Errorf("ReadTail(2) = %q, want the last two lines", got)
	}
	// Asking for more lines than exist returns the whole file, not padding.
	if got := ReadTail(path, 50); got != "one\ntwo\nthree\nfour\nfive" {
		t.Errorf("ReadTail(50) = %q, want the whole file", got)
	}
	// A missing file is not an error: the caller is already reporting a failure.
	if got := ReadTail(filepath.Join(dir, "absent.log"), 5); got != "" {
		t.Errorf("ReadTail(missing) = %q, want empty", got)
	}
}

// The readiness line abbreviates party ids; a value that is not a party id, or
// whose fingerprint is too short to abbreviate, must pass through untouched.
func TestShortParty(t *testing.T) {
	long := "Alice::1220dc30efe8a73c80a00c43f0485d3cb96577da9bbd47fa3fca9fd5837ee9bb4b83"
	got := shortParty(long)
	if got != "Alice::1220dc30...bb4b83" {
		t.Errorf("shortParty = %q", got)
	}
	for _, unchanged := range []string{"Alice", "", "Alice::short"} {
		if got := shortParty(unchanged); got != unchanged {
			t.Errorf("shortParty(%q) = %q, want it unchanged", unchanged, got)
		}
	}
}

func TestOrDefault(t *testing.T) {
	if got := orDefault("", "daml"); got != "daml" {
		t.Errorf("orDefault(empty) = %q, want the fallback", got)
	}
	if got := orDefault("/sdk/daml", "daml"); got != "/sdk/daml" {
		t.Errorf("orDefault(set) = %q, want the value", got)
	}
}

// A tilde must expand, and a path without one must be left exactly as given --
// notably not made absolute, which would change how a child process resolves it.
func TestExpandUser(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	if got := expandUser("~/canton.jar"); got != filepath.Join(home, "canton.jar") {
		t.Errorf("expandUser(~) = %q", got)
	}
	for _, unchanged := range []string{"canton.jar", "/abs/canton.jar", ""} {
		if got := expandUser(unchanged); got != unchanged {
			t.Errorf("expandUser(%q) = %q, want it unchanged", unchanged, got)
		}
	}
}

// resolve differs from expandUser: it also makes the path absolute, because the
// generated Canton config and bootstrap are read from a temporary working
// directory where a relative path would not resolve.
func TestResolveMakesPathsAbsolute(t *testing.T) {
	got, err := resolve("relative/app.dar")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("resolve = %q, want an absolute path", got)
	}
	if !strings.HasSuffix(got, filepath.Join("relative", "app.dar")) {
		t.Errorf("resolve = %q, want it to keep the relative tail", got)
	}
}
