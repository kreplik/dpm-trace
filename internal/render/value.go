package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/walnuthq/dpm-trace/internal/model"
)

// SimplifyLFValue collapses the Ledger API's tagged value encoding into plain
// data. Ports simplify_lf_value.
func SimplifyLFValue(value any) any {
	switch v := value.(type) {
	case *model.Object:
		return simplifyObject(v)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, SimplifyLFValue(item))
		}
		return out
	default:
		return value
	}
}

func simplifyObject(obj *model.Object) any {
	keys := obj.Keys()
	get := func(key string) any {
		value, _ := obj.Get(key)
		return value
	}
	has := func(key string) bool {
		_, ok := obj.Get(key)
		return ok
	}

	if has("sum") && len(keys) == 1 {
		return SimplifyLFValue(get("sum"))
	}

	if fields, ok := get("fields").([]any); ok {
		out := model.NewObject()
		for index, field := range fields {
			fieldObj, isObject := field.(*model.Object)
			if !isObject {
				continue
			}
			label, hasLabel := fieldObj.Get("label")
			name := fmt.Sprint(index)
			if hasLabel {
				name = fmt.Sprint(label)
			}
			fieldValue, _ := fieldObj.Get("value")
			out.Set(name, SimplifyLFValue(fieldValue))
		}
		return out
	}

	if record, ok := get("record").(*model.Object); ok {
		return SimplifyLFValue(record)
	}

	for _, scalarKey := range []string{"party", "int64", "numeric", "text", "contract_id", "contractId", "timestamp", "date", "bool"} {
		if has(scalarKey) && len(keys) == 1 {
			return get(scalarKey)
		}
	}

	if list, ok := get("list").(*model.Object); ok {
		elements, _ := list.Get("elements")
		if elements == nil {
			elements = []any{}
		}
		return SimplifyLFValue(elements)
	}

	if optional, ok := get("optional").(*model.Object); ok {
		inner, present := optional.Get("value")
		if !present {
			return nil
		}
		return SimplifyLFValue(inner)
	}

	if variant, ok := get("variant").(*model.Object); ok {
		constructor := "variant"
		for _, key := range []string{"constructor", "variant"} {
			if value, present := variant.Get(key); present && fmt.Sprint(value) != "" {
				constructor = fmt.Sprint(value)
				break
			}
		}
		inner, _ := variant.Get("value")
		out := model.NewObject()
		out.Set(constructor, SimplifyLFValue(inner))
		return out
	}

	if enum, ok := get("enum").(*model.Object); ok {
		for _, key := range []string{"constructor", "value"} {
			if value, present := enum.Get(key); present && value != nil {
				return value
			}
		}
		return enum
	}

	out := model.NewObject()
	for _, key := range keys {
		if key == "record_id" || key == "recordId" {
			continue
		}
		out.Set(key, SimplifyLFValue(get(key)))
	}
	return out
}

// RenderPrettyValue formats a value for display: inline when it is short,
// pretty-printed JSON when it is not. Ports render_pretty_value.
func RenderPrettyValue(value any, ctx *Context) string {
	simplified := SimplifyLFValue(value)
	if ctx != nil {
		simplified = ctx.RenderValue(simplified)
	}

	if obj, ok := simplified.(*model.Object); ok {
		if obj.Len() == 0 {
			return "{}"
		}
		// Document order, matching Python dict iteration.
		parts := make([]string, 0, obj.Len())
		multiline := false
		for _, key := range obj.Keys() {
			item, _ := obj.Get(key)
			parts = append(parts, key+": "+FormatScalar(item, ctx))
			if strings.Contains(fmt.Sprint(item), "\n") {
				multiline = true
			}
		}
		items := strings.Join(parts, ", ")
		if len(items) <= 100 && !multiline {
			return "{ " + items + " }"
		}
	}

	if list, ok := simplified.([]any); ok {
		parts := make([]string, 0, len(list))
		for _, item := range list {
			parts = append(parts, FormatScalar(item, ctx))
		}
		items := strings.Join(parts, ", ")
		if len(items) <= 100 {
			return "[" + items + "]"
		}
	}

	if !isContainer(simplified) {
		return FormatScalar(simplified, ctx)
	}

	encoded, err := model.Encode(simplified)
	if err != nil {
		return FormatScalar(simplified, ctx)
	}
	return string(encoded)
}

// FormatScalar renders one value the way Python's format_scalar does.
func FormatScalar(value any, ctx *Context) string {
	switch v := value.(type) {
	case string:
		if ctx != nil {
			return ctx.Party(v)
		}
		return v
	case nil:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case json.Number:
		return v.String()
	}
	if !isContainer(value) {
		return fmt.Sprint(value)
	}
	encoded, err := compactSorted(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return encoded
}

// compactSorted mirrors json.dumps(value, sort_keys=True): compact separators,
// sorted keys. Python emits ", " and ": " separators in compact mode.
func compactSorted(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := indentLikePython(&buf, encoded); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// indentLikePython rewrites Go's compact JSON (no spaces) into Python's default
// separators: ", " between items and ": " after keys.
func indentLikePython(out *strings.Builder, data []byte) error {
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		c := data[i]
		out.WriteByte(c)
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case !inString && (c == ',' || c == ':'):
			out.WriteByte(' ')
		}
	}
	return nil
}

func isContainer(value any) bool {
	switch value.(type) {
	case *model.Object, []any, map[string]any:
		return true
	}
	return false
}

// Short truncates with an ellipsis, rendering empty as "-". Ports short().
func Short(value string, maxLen int) string {
	if value == "" {
		return "-"
	}
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}

// ShortTemplate drops the package id from a package:module:entity template.
// Ports short_template.
func ShortTemplate(template string) string {
	if template == "" {
		return ""
	}
	parts := strings.Split(template, ":")
	if len(parts) >= 3 {
		return strings.Join(parts[1:], ":")
	}
	return template
}

// PackageFromTemplate returns the package id portion of a template, if any.
func PackageFromTemplate(template string) string {
	if template == "" {
		return ""
	}
	parts := strings.Split(template, ":")
	if len(parts) >= 3 {
		return parts[0]
	}
	return ""
}

// IndentText indents every line by two spaces, matching textwrap.indent.
func IndentText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}
