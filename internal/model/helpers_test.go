package model

import (
	"strings"
	"testing"
)

// Correlation terms are what a completion is matched against in operator logs.
// Missing one means a relevant log line is never surfaced; a bogus short one
// would match everything, which is why there is a length floor.
func TestCorrelationTerms(t *testing.T) {
	completion, err := Decode([]byte(`{
	  "commandId": "dpm-trace-submit-abcdef123456",
	  "updateId": "1220aabbccdd",
	  "submissionId": "sub-0001",
	  "traceId": "tid-9999",
	  "status": {"code": "FAILED_PRECONDITION"}
	}`))
	if err != nil {
		t.Fatal(err)
	}

	terms := CorrelationTerms(completion)
	for _, want := range []string{"dpm-trace-submit-abcdef123456", "1220aabbccdd", "sub-0001", "tid-9999"} {
		if !terms[want] {
			t.Errorf("missing correlation term %q (got %v)", want, terms)
		}
	}
	for term := range terms {
		if len(term) < 6 {
			t.Errorf("term %q is too short to be selective", term)
		}
	}
}

func TestContainsAnyTerm(t *testing.T) {
	terms := map[string]bool{"abc123": true, "": true}
	if !containsAnyTerm("log line with abc123 in it", terms) {
		t.Error("did not match a present term")
	}
	if containsAnyTerm("unrelated line", terms) {
		t.Error("matched a line with no term; the empty term must be ignored")
	}
}

func TestObjectAccessors(t *testing.T) {
	obj, err := Decode([]byte(`{
	  "nested": {"inner": "value"},
	  "text": "hello",
	  "list": ["a", "b"],
	  "count": 42,
	  "notnumeric": "abc"
	}`))
	if err != nil {
		t.Fatal(err)
	}

	if inner, ok := ObjectField(obj, "nested"); !ok || ObjectString(inner, "inner") != "value" {
		t.Errorf("ObjectField/ObjectString failed: %v", inner)
	}
	if _, ok := ObjectField(obj, "text"); ok {
		t.Error("ObjectField returned a non-object")
	}
	if got := ObjectString(obj, "text"); got != "hello" {
		t.Errorf("ObjectString = %q", got)
	}
	if got := ObjectString(obj, "absent"); got != "" {
		t.Errorf("absent key = %q, want empty", got)
	}
	if got := ObjectStrings(obj, "list"); len(got) != 2 || got[0] != "a" {
		t.Errorf("ObjectStrings = %v", got)
	}
	if got := ObjectValue(obj, "text"); got != "hello" {
		t.Errorf("ObjectValue = %v", got)
	}
	if got := ObjectInt(obj, "count"); got == nil || *got != 42 {
		t.Errorf("ObjectInt = %v", got)
	}
	if got := ObjectInt(obj, "notnumeric"); got != nil {
		t.Errorf("ObjectInt on a non-number = %v, want nil", got)
	}
	if got := ObjectInt(obj, "absent"); got != nil {
		t.Errorf("ObjectInt on an absent key = %v, want nil", got)
	}
}

// DecodeValue accepts a bare array or scalar, where Decode requires an object.
func TestDecodeValue(t *testing.T) {
	value, err := DecodeValue([]byte(`[1, "two", {"three": 3}]`))
	if err != nil {
		t.Fatal(err)
	}
	list, ok := value.([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("decoded %#v", value)
	}
	if _, err := DecodeValue([]byte(`not json`)); err == nil {
		t.Error("invalid JSON decoded without error")
	}
}

// The prepared artifact is a published shape, so its schema and request echo
// have to survive a round trip.
func TestNewPreparedArtifact(t *testing.T) {
	request := map[string]any{"commands": []any{"c"}, "actAs": []any{"Alice"}}
	response, err := Decode([]byte(`{"preparedTransactionHash": "abc123"}`))
	if err != nil {
		t.Fatal(err)
	}

	artifact := NewPreparedArtifact(request, response, "http://ledger", "cmd-1", nil, nil)
	encoded, err := Encode(artifact)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{"prepared", "abc123", "cmd-1", "http://ledger"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}
