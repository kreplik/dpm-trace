package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/ledger"
)

// Everything a `trace`, `submit` or `prepare` against a real participant does
// after argument parsing goes through these paths, and nothing else in the
// suite reaches them: the goldens and lit tests all work from committed files.
// A stub participant covers the fetch, the retry policy and the error handling
// without needing Canton.

// stubLedger serves canned responses and records what it was asked for.
type stubLedger struct {
	server   *httptest.Server
	requests atomic.Int32
}

func newStubLedger(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) *stubLedger {
	t.Helper()
	stub := &stubLedger{}
	mux := http.NewServeMux()
	for path, handler := range routes {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			stub.requests.Add(1)
			handler(w, r)
		})
	}
	stub.server = httptest.NewServer(mux)
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *stubLedger) url() string { return s.server.URL }

// rawUpdate is the shape the JSON Ledger API returns from update-by-id.
func rawUpdate() string {
	return `{"update":{"TransactionTree":{"updateId":"1220abcdef","offset":42,
	  "recordTime":"2026-01-01T00:00:00Z","synchronizerId":"local::1220ff",
	  "eventsById":{"0":{"CreatedEvent":{
	    "nodeId":0,"contractId":"00abc","templateId":"pkg:Asset:Asset",
	    "signatories":["Issuer"],"observers":["Alice"],"witnessParties":["Issuer"],
	    "createArgument":{"issuer":"Issuer","owner":"Alice","name":"GOLD","quantity":"100"}}}},
	  "rootNodeIds":[0]}}}`
}

func serveJSON(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func TestTraceAgainstAParticipant(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.UpdateByIDPath: serveJSON(rawUpdate()),
	})

	stdout, stderr, code := capture(t, func() int {
		return runTrace([]string{"1220abcdef", "--submitter", stub.url(), "--read-as", "Issuer", "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"Canton trace", "1220abcdef", "CREATE Asset:Asset", "signatories: Issuer", "quantity: 100"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
	// The projection caveat must survive a live fetch, not only a replayed file.
	if !strings.Contains(stdout, "participant projection") {
		t.Errorf("no projection label in:\n%s", stdout)
	}
}

// --export writes a portable artifact; --print-json returns before exporting,
// which is the ordering cli.py uses.
func TestTraceExportAndPrintJSON(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.UpdateByIDPath: serveJSON(rawUpdate()),
	})
	dir := t.TempDir()

	exported := filepath.Join(dir, "trace.json")
	_, stderr, code := capture(t, func() int {
		return runTrace([]string{"1220abcdef", "--submitter", stub.url(), "--read-as", "Issuer",
			"--export", exported, "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	data, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("artifact not written: %v", err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("artifact is not JSON: %v", err)
	}
	if artifact["schema"] != "dpm-trace/trace-artifact/v0" {
		t.Errorf("schema = %v", artifact["schema"])
	}

	notWritten := filepath.Join(dir, "skipped.json")
	stdout, _, code := capture(t, func() int {
		return runTrace([]string{"1220abcdef", "--submitter", stub.url(), "--read-as", "Issuer",
			"--print-json", "--export", notWritten})
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if _, err := os.Stat(notWritten); err == nil {
		t.Error("--print-json must return before --export writes, as run_trace does")
	}
	if !strings.Contains(stdout, "eventsById") {
		t.Errorf("--print-json did not emit the trace:\n%s", stdout)
	}
}

// A participant that rejects the call must surface its status and body, not a
// bare failure.
func TestTraceReportsAnHTTPError(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.UpdateByIDPath: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"INVALID_ARGUMENT","cause":"bad update id"}`))
		},
	})

	_, stderr, code := capture(t, func() int {
		return runTrace([]string{"1220abcdef", "--submitter", stub.url(), "--read-as", "Issuer", "--color", "never"})
	})
	if code == 0 {
		t.Fatal("exit 0 for an HTTP 400")
	}
	for _, want := range []string{"400", "INVALID_ARGUMENT"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("missing %q in stderr: %s", want, stderr)
		}
	}
}

// 5xx is transient, so the client retries. A participant that fails once and
// then succeeds must produce a trace, not an error.
func TestTraceRetriesTransientFailures(t *testing.T) {
	var attempts atomic.Int32
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.UpdateByIDPath: func(w http.ResponseWriter, r *http.Request) {
			if attempts.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(rawUpdate()))
		},
	})

	stdout, stderr, code := capture(t, func() int {
		return runTrace([]string{"1220abcdef", "--submitter", stub.url(), "--read-as", "Issuer", "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d after a retryable failure, stderr: %s", code, stderr)
	}
	if got := attempts.Load(); got < 2 {
		t.Errorf("attempts = %d, want the 503 retried", got)
	}
	if !strings.Contains(stdout, "CREATE Asset:Asset") {
		t.Errorf("no trace after retry:\n%s", stdout)
	}
}

// submit-and-wait returns the update id, which is what the command prints so it
// can be piped into `dpm trace`.
func TestSubmitPrintsTheUpdateID(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.SubmitAndWaitPath: serveJSON(`{"updateId":"1220submitted","completionOffset":77}`),
	})

	stdout, stderr, code := capture(t, func() int {
		return runSubmit([]string{
			"--submitter", stub.url(), "--act-as", "Alice::1220aa",
			"--template", "#pkg:Asset:Asset",
			"--arg", "issuer=Alice::1220aa", "--arg", "quantity=100",
		})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) != "1220submitted" {
		t.Errorf("stdout = %q, want just the update id", stdout)
	}
}

// A rejected submission is not a crash: --allow-fail turns it into completion
// data the failure workflows consume.
func TestSubmitAllowFailEmitsCompletionJSON(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.SubmitAndWaitPath: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"DAML_FAILURE","cause":"Insufficient balance"}`))
		},
	})

	stdout, _, _ := capture(t, func() int {
		return runSubmit([]string{
			"--submitter", stub.url(), "--act-as", "Alice::1220aa",
			"--template", "#pkg:Asset:Asset", "--choice", "Withdraw",
			"--contract-id", "00abc", "--arg", "amount=500",
			"--allow-fail", "--print-json",
		})
	})
	var completion map[string]any
	if err := json.Unmarshal([]byte(stdout), &completion); err != nil {
		t.Fatalf("--allow-fail --print-json did not emit JSON: %v\n%s", err, stdout)
	}
	if completion["code"] != "DAML_FAILURE" {
		t.Errorf("completion = %v", completion)
	}
}

// prepare returns the transaction the participant would commit, exported as a
// prepared artifact for later comparison.
func TestPrepareExportsAnArtifact(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.InteractiveSubmissionPath: serveJSON(
			`{"preparedTransactionHash":"aabbcc","preparedTransaction":{"transaction":{}}}`),
	})
	exported := filepath.Join(t.TempDir(), "prepared.json")

	_, stderr, code := capture(t, func() int {
		return runPrepare([]string{
			"--submitter", stub.url(), "--act-as", "Alice::1220aa",
			"--template", "#pkg:Asset:Asset", "--arg", "quantity=1",
			"--export", exported,
		})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	data, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("prepared artifact not written: %v", err)
	}
	if !strings.Contains(string(data), "aabbcc") {
		t.Errorf("preparation hash missing from the artifact:\n%s", data)
	}
}

// compare fetches both sides from the participant when given two update ids.
func TestCompareFetchesBothUpdates(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.UpdateByIDPath: serveJSON(rawUpdate()),
	})

	stdout, stderr, code := capture(t, func() int {
		return runCompare([]string{"1220aaa", "1220bbb",
			"--submitter", stub.url(), "--read-as", "Issuer", "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "update-vs-update") {
		t.Errorf("no comparison rendered:\n%s", stdout)
	}
	if got := stub.requests.Load(); got != 2 {
		t.Errorf("made %d requests, want one per side", got)
	}
}

// --command-id looks a failed submission up in the completion stream.
func TestTraceByCommandID(t *testing.T) {
	stub := newStubLedger(t, map[string]func(http.ResponseWriter, *http.Request){
		ledger.CompletionsPath: serveJSON(`[{"completionResponse":{"Completion":{"value":{
		  "commandId":"cmd-1","status":{"code":9,"message":"Insufficient balance"},
		  "offset":12,"updateId":""}}}}]`),
	})

	stdout, stderr, code := capture(t, func() int {
		return runTrace([]string{"--command-id", "cmd-1",
			"--submitter", stub.url(), "--act-as", "Alice::1220aa", "--color", "never"})
	})
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{"completion", "cmd-1", "Insufficient balance"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("missing %q in:\n%s", want, stdout)
		}
	}
}
