package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Object is a JSON object that remembers the order its keys arrived in.
//
// Order matters twice, in opposite directions, and the Python implementation
// gets both for free from dict semantics:
//
//   - Rendering a payload inline as "{ payer: ..., owner: ... }" iterates the
//     value in document order. A Go map would randomize that, so the same input
//     would render differently on consecutive runs.
//   - Every json.dumps in cli.py passes sort_keys=True, so emitted JSON is
//     sorted regardless of document order.
//
// Keys therefore returns insertion order for the renderer, while MarshalJSON
// sorts, matching the two behaviors exactly.
type Object struct {
	keys   []string
	values map[string]any
}

// NewObject returns an empty Object.
func NewObject() *Object {
	return &Object{values: map[string]any{}}
}

// Get returns the value for key and whether it was present.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	value, ok := o.values[key]
	return value, ok
}

// Set adds or replaces a key, preserving first-insertion position on replace.
func (o *Object) Set(key string, value any) {
	if o.values == nil {
		o.values = map[string]any{}
	}
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// Delete removes a key.
func (o *Object) Delete(key string) {
	if o == nil {
		return
	}
	if _, exists := o.values[key]; !exists {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Keys returns the keys in document order. The returned slice is a copy.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}
	out := make([]string, len(o.keys))
	copy(out, o.keys)
	return out
}

// Len returns the number of keys.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// MarshalJSON emits sorted keys, matching json.dumps(..., sort_keys=True).
func (o *Object) MarshalJSON() ([]byte, error) {
	if o == nil {
		return []byte("null"), nil
	}
	keys := o.Keys()
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encoded, err := marshalNoEscape(key)
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
		buf.WriteByte(':')
		encoded, err = marshalNoEscape(o.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(encoded)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalNoEscape encodes without Go's default HTML escaping, which Python does
// not apply.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// decodeValue reads one value from dec, building Objects for JSON objects so
// key order survives. Numbers stay json.Number: offsets and reassignment
// counters are large integers that float64 would round and reformat.
func decodeValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return decodeFromToken(dec, token)
}

func decodeFromToken(dec *json.Decoder, token json.Token) (any, error) {
	switch t := token.(type) {
	case json.Delim:
		switch t {
		case '{':
			obj := NewObject()
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object key is %T, not a string", keyToken)
				}
				value, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(key, value)
			}
			if _, err := dec.Token(); err != nil { // closing }
				return nil, err
			}
			return obj, nil
		case '[':
			list := []any{}
			for dec.More() {
				value, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				list = append(list, value)
			}
			if _, err := dec.Token(); err != nil { // closing ]
				return nil, err
			}
			return list, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %v", t)
	default:
		return token, nil
	}
}

// asObject converts a decoded value to an Object when it is one.
func asObject(value any) (*Object, bool) {
	obj, ok := value.(*Object)
	return obj, ok
}

// errUnexpectedEnd is returned when a document holds more than one value.
var errTrailingData = fmt.Errorf("unexpected data after top-level value")

func ensureEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return errTrailingData
		}
		return err
	}
	return nil
}

// ObjectField returns a nested object field, for callers outside this package.
func ObjectField(obj *Object, key string) (*Object, bool) {
	return pickObject(obj, key)
}

// ObjectString returns a field rendered as a string, empty when absent.
func ObjectString(obj *Object, key string) string {
	return pickString(obj, key)
}

// ObjectStrings returns a list field as strings.
func ObjectStrings(obj *Object, key string) []string {
	return listString(pick(obj, key))
}
