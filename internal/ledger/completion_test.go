package ledger

import (
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

func decode(t *testing.T, text string) any {
	t.Helper()
	value, err := model.DecodeValue([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// A real participant returns offset checkpoints interleaved with completions.
// They carry no submission outcome and must not be mistaken for one.
func TestNormalizeCompletionListDropsOffsetCheckpoints(t *testing.T) {
	raw := decode(t, `[
	  {"completionResponse":{"OffsetCheckpoint":{"value":{"offset":35}}}},
	  {"completionResponse":{"Completion":{"value":{"commandId":"cmd-1","offset":36}}}}
	]`)
	got := NormalizeCompletionList(raw)
	if len(got) != 1 {
		t.Fatalf("got %d completions, want 1 (the checkpoint must be dropped)", len(got))
	}
	if id := model.ObjectString(got[0], "commandId"); id != "cmd-1" {
		t.Errorf("commandId = %q", id)
	}
}

func TestNormalizeCompletionListAcceptsWrapperShapes(t *testing.T) {
	for _, body := range []string{
		`{"completions":[{"commandId":"cmd-1"}]}`,
		`{"items":[{"commandId":"cmd-1"}]}`,
		`{"responses":[{"commandId":"cmd-1"}]}`,
		`{"completionResponses":[{"commandId":"cmd-1"}]}`,
		`[{"commandId":"cmd-1"}]`,
		`{"commandId":"cmd-1"}`,
	} {
		got := NormalizeCompletionList(decode(t, body))
		if len(got) != 1 {
			t.Errorf("%s -> %d completions, want 1", body, len(got))
			continue
		}
		if id := model.ObjectString(got[0], "commandId"); id != "cmd-1" {
			t.Errorf("%s -> commandId %q", body, id)
		}
	}
}

func TestFetchCompletionRequiresPartiesAndURL(t *testing.T) {
	if _, err := New("", "").FetchCompletion(CompletionLookup{CommandID: "c", Parties: []string{"A"}}); err == nil {
		t.Error("expected an error without a ledger url")
	}
	if _, err := New("http://host", "").FetchCompletion(CompletionLookup{CommandID: "c"}); err == nil {
		t.Error("expected an error without parties")
	}
}

func TestFetchCompletionRejectsNonNumericOffset(t *testing.T) {
	_, err := New("http://host", "").FetchCompletion(CompletionLookup{
		CommandID: "c", Parties: []string{"A"}, BeginExclusive: "abc",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if err.Error() != "--begin-exclusive must be an integer offset" {
		t.Errorf("error = %v", err)
	}
}
