package ledger

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestJoinURL(t *testing.T) {
	for _, tc := range []struct{ base, path, want string }{
		{"http://host:7575", "/v2/updates", "http://host:7575/v2/updates"},
		{"http://host:7575/", "/v2/updates", "http://host:7575/v2/updates"},
		{"http://host:7575/", "v2/updates", "http://host:7575/v2/updates"},
	} {
		if got := JoinURL(tc.base, tc.path); got != tc.want {
			t.Errorf("JoinURL(%q, %q) = %q, want %q", tc.base, tc.path, got, tc.want)
		}
	}
}

// 5xx is retried; the request must eventually succeed.
func TestRetriesTransientServerErrors(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"ok": true}`)
	}))
	defer server.Close()

	client := New(server.URL, "")
	obj, err := client.JSON("GET", server.URL, nil, true)
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if value, _ := obj.Get("ok"); value != true {
		t.Errorf("body = %v", value)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// 4xx is a client error: retrying cannot fix it and would multiply the failure.
func TestDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := New(server.URL, "").JSON("GET", server.URL, nil, true); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (4xx must not be retried)", calls)
	}
}

// submit-and-wait is not idempotent: a retry could double-submit.
func TestRetryDisabledMakesOneAttempt(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := New(server.URL, "").JSON("POST", server.URL, map[string]any{}, false); err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (retry disabled)", calls)
	}
}

func TestSendsBearerToken(t *testing.T) {
	var got string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	if _, err := New(server.URL, "  secret  ").JSON("GET", server.URL, nil, true); err != nil {
		t.Fatalf("request: %v", err)
	}
	if got != "Bearer secret" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer secret")
	}
}

// The reassignment request must ask for reassignments, or an unassign/assign
// update comes back empty.
func TestUpdateByIDBodyRequestsReassignments(t *testing.T) {
	body := UpdateByIDBody("update-1", []string{"Alice::1220ab"})
	format, ok := body["updateFormat"].(map[string]any)
	if !ok {
		t.Fatal("no updateFormat")
	}
	if _, present := format["includeReassignments"]; !present {
		t.Error("includeReassignments missing: reassignment updates would return empty")
	}
	transactions, ok := format["includeTransactions"].(map[string]any)
	if !ok {
		t.Fatal("no includeTransactions")
	}
	if shape := transactions["transactionShape"]; shape != "TRANSACTION_SHAPE_LEDGER_EFFECTS" {
		t.Errorf("transactionShape = %v", shape)
	}
}

func TestLoadUpdateRequiresParties(t *testing.T) {
	if _, _, _, err := New("http://host", "").LoadUpdate("update-1", nil); err == nil {
		t.Error("expected an error: a participant fetch needs read-as parties")
	}
}
