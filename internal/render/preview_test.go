package render

import (
	"strings"
	"testing"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// A preview exists for values too big to print. RenderPrettyValue falls back to
// indented JSON at exactly that size, so its first line is "{" -- the preview
// has to keep the leading fields instead.
func TestPreviewValueKeepsFieldsForLargePayloads(t *testing.T) {
	obj := model.NewObject()
	for _, field := range []string{"issuer", "name", "owner", "quantity"} {
		obj.Set(field, strings.Repeat("x", 40))
	}

	pretty := RenderPrettyValue(obj, nil)
	if first, _, _ := strings.Cut(pretty, "\n"); strings.TrimSpace(first) != "{" {
		t.Fatalf("fixture is not multi-line; first line = %q", first)
	}

	preview := PreviewValue(obj, nil, 100)
	if strings.Contains(preview, "\n") {
		t.Errorf("preview is multi-line: %q", preview)
	}
	if !strings.Contains(preview, "issuer") {
		t.Errorf("preview = %q, want the leading field", preview)
	}
	if len(preview) > 100 {
		t.Errorf("preview is %d chars, want <= 100: %q", len(preview), preview)
	}
}

func TestPreviewValueLeavesSmallValuesReadable(t *testing.T) {
	obj := model.NewObject()
	obj.Set("name", "GOLD")
	obj.Set("quantity", int64(60))

	if got, want := PreviewValue(obj, nil, 100), "{ name: GOLD, quantity: 60 }"; got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}
}
