package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Decode parses a Ledger API response into the untyped, order-preserving form
// the rest of this package works with.
//
// Objects become *Object rather than map[string]any so payload key order
// survives to the renderer, and numbers stay json.Number so large offsets are
// neither rounded nor reformatted.
func Decode(data []byte) (*Object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if err := ensureEOF(dec); err != nil {
		return nil, err
	}
	obj, ok := asObject(value)
	if !ok {
		return nil, fmt.Errorf("expected a JSON object, got %T", value)
	}
	return obj, nil
}

// Encode renders v the way the Python implementation does: two-space indent,
// sorted keys, and no HTML escaping.
//
// Objects sort themselves via MarshalJSON, matching json.dumps(sort_keys=True);
// Go maps are sorted by encoding/json already. Structs are not -- encoding/json
// emits their fields in declaration order -- so anything whose key order must
// match Python is built as an Object or a map, never a struct.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode always appends a newline; callers decide their own trailing bytes.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// pick returns the first key present in obj, mirroring cli.py's pick(). The
// Ledger API spells the same field several ways depending on shape and version,
// so probing a list of names is the tolerance, not a shortcut.
func pick(obj *Object, keys ...string) any {
	if obj == nil {
		return nil
	}
	for _, key := range keys {
		if value, ok := obj.Get(key); ok {
			return value
		}
	}
	return nil
}

// pickObject is pick restricted to object values.
func pickObject(obj *Object, keys ...string) (*Object, bool) {
	return asObject(pick(obj, keys...))
}

// pickString renders the first present key as a string. Absent and JSON null
// both yield "".
func pickString(obj *Object, keys ...string) string {
	return toString(pick(obj, keys...))
}

// toString mirrors Python's str() for the scalar shapes that reach it.
func toString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprint(v)
	}
}

// listString mirrors cli.py's list_str: a list becomes its elements stringified,
// nil becomes empty, and a scalar becomes a single-element list.
func listString(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, toString(item))
		}
		return out
	default:
		return []string{toString(value)}
	}
}

// asInt mirrors cli.py's as_int: anything that does not parse as an integer
// yields no value.
func asInt(value any) *int64 {
	switch v := value.(type) {
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return &n
		}
	case string:
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return &n
		}
	case float64:
		n := int64(v)
		return &n
	}
	return nil
}

// asBool returns a pointer so "absent" stays distinct from "false"; consuming=false
// is meaningful on an exercise event.
func asBool(value any) *bool {
	if b, ok := value.(bool); ok {
		return &b
	}
	return nil
}

// templateName mirrors cli.py's template_name: a string passes through, an
// identifier object joins the parts it has.
func templateName(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case *Object:
		var parts []string
		for _, keys := range [][]string{
			{"package_id", "packageId", "packageName"},
			{"module_name", "moduleName"},
			{"entity_name", "entityName"},
		} {
			if part := pickString(v, keys...); part != "" {
				parts = append(parts, part)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ":")
		}
		if encoded, err := v.MarshalJSON(); err == nil {
			return string(encoded)
		}
		return ""
	default:
		return toString(value)
	}
}
