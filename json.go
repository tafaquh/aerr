package aerr

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// MarshalJSON implements json.Marshaler. The key and nesting shape is the
// same across the slog and zerolog integrations:
//
//	{"code": ..., "message": ..., "attributes": {...}, "stacktrace": [...]}
//
// Value encodings, however, follow encoding/json and may differ from an
// adapter's native encoding: here durations serialize as integer
// nanoseconds and []byte as base64, whereas the zerolog integration
// renders durations in its configured DurationFieldUnit (milliseconds)
// and []byte raw. Empty fields are omitted. Attribute values marshal with
// encoding/json; values implementing error (but not json.Marshaler)
// marshal as their message string (or "<panic: ...>" if that call
// panics), and unmarshalable values degrade to their fmt representation
// instead of failing the whole error.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	buf := make([]byte, 0, 128)
	buf = append(buf, '{')
	if e.code != "" {
		buf = appendJSONField(buf, "code", e.code)
	}
	if e.msg != "" {
		buf = appendJSONField(buf, "message", e.msg)
	}
	if len(e.attrs) > 0 {
		if len(buf) > 1 {
			buf = append(buf, ',')
		}
		buf = append(buf, `"attributes":{`...)
		for i, a := range e.attrs {
			if i > 0 {
				buf = append(buf, ',')
			}
			key, _ := json.Marshal(a.key)
			buf = append(buf, key...)
			buf = append(buf, ':')
			buf = append(buf, attrJSON(a.val)...)
		}
		buf = append(buf, '}')
	}
	if traces := e.Traces(); len(traces) > 0 {
		if len(buf) > 1 {
			buf = append(buf, ',')
		}
		buf = append(buf, `"stacktrace":`...)
		val, _ := json.Marshal(traces)
		buf = append(buf, val...)
	}
	buf = append(buf, '}')
	return buf, nil
}

func appendJSONField(buf []byte, key, val string) []byte {
	if len(buf) > 1 {
		buf = append(buf, ',')
	}
	k, _ := json.Marshal(key)
	buf = append(buf, k...)
	buf = append(buf, ':')
	v, _ := json.Marshal(val)
	buf = append(buf, v...)
	return buf
}

// attrJSON marshals one attribute value, never failing and never
// panicking: error values render as their message, and values
// encoding/json rejects fall back to their fmt representation.
func attrJSON(v any) []byte {
	if _, ok := v.(json.Marshaler); !ok {
		if er, ok := v.(error); ok {
			msg, ok := errString(er)
			if !ok {
				return []byte("null")
			}
			out, err := json.Marshal(msg)
			if err == nil {
				return out
			}
		}
	}
	out, err := json.Marshal(v)
	if err != nil {
		out, err = json.Marshal(fmt.Sprint(v))
		if err != nil {
			return []byte(`"<unmarshalable>"`)
		}
	}
	return out
}

// errString returns err's message for JSON attribute encoding, tolerating
// typed-nil errors and Error() implementations that panic; MarshalJSON
// must never crash the caller. A value-receiver error whose Error()
// dereferences a nil field has reflect.Kind Struct, so it slips past
// isNilValue's nilable-kind check — the recover below is what actually
// protects against it. ok is false only for nil-ish errors (the caller
// then emits null); on a recovered panic ok is true with a "<panic: ...>"
// placeholder message.
func errString(err error) (msg string, ok bool) {
	if isNilValue(err) {
		return "", false
	}
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("<panic: %v>", r)
			ok = true
		}
	}()
	return err.Error(), true
}

// isNilValue reports whether v holds a nil pointer/interface/map/slice,
// guarding Error() calls on typed-nil values.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return rv.IsNil()
	}
	return false
}
