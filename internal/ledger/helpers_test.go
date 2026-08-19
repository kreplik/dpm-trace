package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// A bearer token may come from a flag, a file, or the environment. The file
// form exists so the token stays out of shell history and process listings.
func TestResolveToken(t *testing.T) {
	if got, err := ResolveToken("literal", ""); err != nil || got != "literal" {
		t.Errorf("literal token = %q, %v", got, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveToken("", path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-file" {
		t.Errorf("file token = %q, want it trimmed", got)
	}

	// An explicit token wins over the file.
	if got, _ := ResolveToken("literal", path); got != "literal" {
		t.Errorf("explicit token lost to the file: %q", got)
	}

	t.Setenv("DPM_TRACE_TOKEN_FILE", path)
	if got, _ := ResolveToken("", ""); got != "from-file" {
		t.Errorf("env token file ignored: %q", got)
	}

	if _, err := ResolveToken("", filepath.Join(dir, "absent")); err == nil {
		t.Error("a missing token file returned no error")
	}
}

// The error text is what a user sees when a participant rejects a call, so it
// must carry the method, URL, status and body.
func TestHTTPErrorText(t *testing.T) {
	body, _ := model.Decode([]byte(`{"code": "INVALID_ARGUMENT"}`))
	err := &HTTPError{Method: "POST", URL: "http://p/v2/x", StatusCode: 400, Detail: "bad request", Body: body}

	text := err.Error()
	for _, want := range []string{"POST", "http://p/v2/x", "400", "bad request"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in %q", want, text)
		}
	}

	if got := errorBody(err); got == nil {
		t.Error("errorBody dropped the decoded body")
	}
	if got := errorBody(os.ErrNotExist); got != nil {
		t.Errorf("errorBody on a non-HTTP error = %v, want nil", got)
	}
}

// Command ids are correlated against logs and completions, so they must be
// prefixed and unique.
func TestGenerateCommandID(t *testing.T) {
	first := GenerateCommandID("dpm-trace-submit-")
	second := GenerateCommandID("dpm-trace-submit-")

	if !strings.HasPrefix(first, "dpm-trace-submit-") {
		t.Errorf("missing prefix: %q", first)
	}
	if first == second {
		t.Error("two ids collided")
	}
	if len(first) != len("dpm-trace-submit-")+12 {
		t.Errorf("id = %q, want a 12-character suffix", first)
	}
}
