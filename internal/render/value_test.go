package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// simplifyObject collapses the Ledger API's tagged value encoding into plain
// data. Each branch below is a real encoding the API emits; an unhandled one
// surfaces to the user as raw JSON noise in the payload.

func simplify(t *testing.T, raw string) any {
	t.Helper()
	value, err := model.DecodeValue([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return SimplifyLFValue(value)
}

func TestSimplifyScalars(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      any
	}{
		{"party", `{"party":"Alice"}`, "Alice"},
		{"int64", `{"int64":"42"}`, "42"},
		{"numeric", `{"numeric":"1.5"}`, "1.5"},
		{"text", `{"text":"hello"}`, "hello"},
		{"contract_id", `{"contract_id":"00abc"}`, "00abc"},
		{"contractId", `{"contractId":"00abc"}`, "00abc"},
		{"timestamp", `{"timestamp":"2026-01-01T00:00:00Z"}`, "2026-01-01T00:00:00Z"},
		{"bool", `{"bool":true}`, true},
	} {
		if got := simplify(t, tc.raw); got != tc.want {
			t.Errorf("%s: got %#v, want %#v", tc.name, got, tc.want)
		}
	}
}

// A single-key "sum" wrapper is unwrapped; more than one key is not, because
// then it is a record whose field happens to be called sum.
func TestSimplifySum(t *testing.T) {
	if got := simplify(t, `{"sum":{"text":"inner"}}`); got != "inner" {
		t.Errorf("got %#v", got)
	}
	got := simplify(t, `{"sum":{"text":"inner"},"other":1}`)
	if _, ok := got.(*model.Object); !ok {
		t.Errorf("a two-key object must stay an object, got %#v", got)
	}
}

// Records arrive as a "fields" list of label/value pairs, and the field order
// is the declaration order a reader expects.
func TestSimplifyRecordFields(t *testing.T) {
	got := simplify(t, `{"fields":[
	  {"label":"issuer","value":{"party":"Issuer"}},
	  {"label":"quantity","value":{"int64":"100"}}]}`)
	obj, ok := got.(*model.Object)
	if !ok {
		t.Fatalf("got %#v, want an object", got)
	}
	if keys := obj.Keys(); len(keys) != 2 || keys[0] != "issuer" || keys[1] != "quantity" {
		t.Errorf("keys = %v, want declaration order", keys)
	}
	if v, _ := obj.Get("quantity"); v != "100" {
		t.Errorf("quantity = %#v", v)
	}
}

// An unlabelled field falls back to its position, so a tuple still renders.
func TestSimplifyRecordWithoutLabels(t *testing.T) {
	got := simplify(t, `{"fields":[{"value":{"text":"first"}},{"value":{"text":"second"}}]}`)
	obj, ok := got.(*model.Object)
	if !ok {
		t.Fatalf("got %#v", got)
	}
	if v, _ := obj.Get("0"); v != "first" {
		t.Errorf("positional field 0 = %#v", v)
	}
}

func TestSimplifyNestedRecord(t *testing.T) {
	got := simplify(t, `{"record":{"fields":[{"label":"name","value":{"text":"GOLD"}}]}}`)
	obj, ok := got.(*model.Object)
	if !ok {
		t.Fatalf("got %#v", got)
	}
	if v, _ := obj.Get("name"); v != "GOLD" {
		t.Errorf("name = %#v", v)
	}
}

func TestSimplifyList(t *testing.T) {
	got := simplify(t, `{"list":{"elements":[{"text":"a"},{"text":"b"}]}}`)
	list, ok := got.([]any)
	if !ok || len(list) != 2 || list[0] != "a" {
		t.Fatalf("got %#v", got)
	}
	// An empty list has no "elements" key at all.
	if got := simplify(t, `{"list":{}}`); len(got.([]any)) != 0 {
		t.Errorf("empty list = %#v", got)
	}
}

// None is an optional with no value; Some unwraps to the inner value.
func TestSimplifyOptional(t *testing.T) {
	if got := simplify(t, `{"optional":{}}`); got != nil {
		t.Errorf("None = %#v, want nil", got)
	}
	if got := simplify(t, `{"optional":{"value":{"text":"present"}}}`); got != "present" {
		t.Errorf("Some = %#v", got)
	}
}

// A variant renders as {constructor: value} so the case name is visible.
func TestSimplifyVariant(t *testing.T) {
	got := simplify(t, `{"variant":{"constructor":"Pending","value":{"text":"waiting"}}}`)
	obj, ok := got.(*model.Object)
	if !ok {
		t.Fatalf("got %#v", got)
	}
	if v, _ := obj.Get("Pending"); v != "waiting" {
		t.Errorf("variant = %#v", got)
	}
	// Without a constructor name it still renders rather than dropping data.
	if got := simplify(t, `{"variant":{"value":{"text":"x"}}}`); got == nil {
		t.Error("nameless variant dropped")
	}
}

func TestSimplifyEnum(t *testing.T) {
	if got := simplify(t, `{"enum":{"constructor":"Red"}}`); got != "Red" {
		t.Errorf("got %#v", got)
	}
	if got := simplify(t, `{"enum":{"value":"Blue"}}`); got != "Blue" {
		t.Errorf("got %#v", got)
	}
}

// record_id is type metadata, not data: it must not appear in a payload.
func TestSimplifyDropsRecordID(t *testing.T) {
	for _, key := range []string{"record_id", "recordId"} {
		got := simplify(t, `{"`+key+`":"pkg:M:E","name":{"text":"GOLD"}}`)
		obj, ok := got.(*model.Object)
		if !ok {
			t.Fatalf("got %#v", got)
		}
		if _, present := obj.Get(key); present {
			t.Errorf("%s leaked into the payload", key)
		}
		if v, _ := obj.Get("name"); v != "GOLD" {
			t.Errorf("name = %#v", v)
		}
	}
}

func TestSimplifyNestedList(t *testing.T) {
	got := simplify(t, `[{"text":"a"},{"party":"Bob"}]`)
	list, ok := got.([]any)
	if !ok || len(list) != 2 || list[1] != "Bob" {
		t.Fatalf("got %#v", got)
	}
}

// Short values render inline; long ones fall back to pretty JSON so a wide
// payload does not run off the screen.
func TestRenderPrettyValueInlineVersusBlock(t *testing.T) {
	short := model.NewObject()
	short.Set("name", "GOLD")
	short.Set("quantity", "100")
	if got := RenderPrettyValue(short, nil); !strings.HasPrefix(got, "{ ") || strings.Contains(got, "\n") {
		t.Errorf("short value not inline: %q", got)
	}

	long := model.NewObject()
	for _, key := range []string{"a", "b", "c", "d", "e"} {
		long.Set(key, strings.Repeat(key, 40))
	}
	if got := RenderPrettyValue(long, nil); !strings.Contains(got, "\n") {
		t.Errorf("long value not blocked: %q", got)
	}

	if got := RenderPrettyValue(model.NewObject(), nil); got != "{}" {
		t.Errorf("empty object = %q", got)
	}
	if got := RenderPrettyValue([]any{"a", "b"}, nil); got != "[a, b]" {
		t.Errorf("list = %q", got)
	}
}

func TestFormatScalar(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, "null"}, {true, "true"}, {false, "false"}, {"text", "text"},
	} {
		if got := FormatScalar(tc.in, nil); got != tc.want {
			t.Errorf("FormatScalar(%#v) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Containers fall back to compact JSON with Python's separators.
	obj := model.NewObject()
	obj.Set("b", "2")
	obj.Set("a", "1")
	got := FormatScalar(obj, nil)
	if !strings.Contains(got, `"a": "1"`) || !strings.Contains(got, ", ") {
		t.Errorf("container = %q, want Python separators", got)
	}
}

func TestShortTemplate(t *testing.T) {
	if got := ShortTemplate("pkg123:Asset:Asset"); got != "Asset:Asset" {
		t.Errorf("got %q", got)
	}
	for _, unchanged := range []string{"Asset:Asset", "bare", ""} {
		if got := ShortTemplate(unchanged); got != unchanged {
			t.Errorf("ShortTemplate(%q) = %q", unchanged, got)
		}
	}
}

// Inline JSON goes through the port's encoder, not encoding/json: the latter
// HTML-escapes and leaves non-ASCII raw, both of which diverge from the
// artifacts and reports this tool emits elsewhere.
func TestFormatScalarEncodingMatchesTheArtifactEncoder(t *testing.T) {
	obj := model.NewObject()
	obj.Set("markup", "a < b")
	obj.Set("unicode", "café")

	got := FormatScalar(obj, nil)
	if !strings.Contains(got, "a < b") {
		t.Errorf("< was HTML-escaped: %s", got)
	}
	if !strings.Contains(got, `\u00e9`) {
		t.Errorf("non-ASCII not escaped as ensure_ascii does: %s", got)
	}
}

// Truncation counts characters: cutting bytes can split a multibyte rune.
func TestShortCutsOnCharacterBoundaries(t *testing.T) {
	got := Short("ééééééééééééé", 8)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("not truncated: %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation split a rune: %q", got)
	}
	if runes := []rune(got); len(runes) != 8 {
		t.Errorf("got %d characters, want 8: %q", len(runes), got)
	}
}
