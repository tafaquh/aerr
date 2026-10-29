package aerr

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
)

// MarshalJSON implements json.Marshaler. The key and nesting shape is the
// same across the slog and zerolog integrations (except that slog keys an
// aggregate's members by position, having no array kind for groups):
//
//	{"code": ..., "message": ..., "attributes": {...}, "stacktrace": [...]}
//
// When the error wraps an aggregate (see [Join]), an "errors" array follows
// with one object of this same shape per member, so every member keeps its
// own code, attributes, and stack trace. At most 32 members are expanded per
// rendered error, counted across nesting; the rest are counted under
// "errors_omitted".
//
// Value encodings, however, follow encoding/json and may differ from an
// adapter's native encoding: here durations serialize as integer
// nanoseconds and []byte as base64, whereas the zerolog integration
// renders durations in its configured DurationFieldUnit (milliseconds)
// and []byte raw. Empty fields are omitted. Attribute values marshal with
// encoding/json; values implementing error (but not json.Marshaler)
// marshal as their message string, unmarshalable values degrade to their
// fmt representation, and any value whose MarshalJSON, String, or Error
// panics degrades to a "<panic: ...>" placeholder — MarshalJSON never
// fails and never panics on an attribute value.
func (e *Error) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}
	buf, _ := appendErrorJSON(make([]byte, 0, 128), e, maxMembers)
	return buf, nil
}

// appendErrorJSON appends err as a JSON object. The message is the one err
// itself reports; code, attributes, and stacktrace come from the nearest
// *Error on err's single-cause chain; and when that chain reaches an
// aggregate, its members are appended the same way under "errors". This is
// the rendering every member of an aggregate gets, whatever its type. budget
// is the member allowance left (see maxMembers); what remains of it is
// returned.
func appendErrorJSON(buf []byte, err error, budget int) ([]byte, int) {
	e, members := inspect(err)
	buf = append(buf, '{')
	open := len(buf) // a field written past this offset needs a comma
	if code := e.Code(); code != "" {
		buf = appendJSONField(buf, open, "code", code)
	}
	if msg := errorText(err); msg != "" {
		buf = appendJSONField(buf, open, "message", msg)
	}
	if e != nil && len(e.attrs) > 0 {
		if len(buf) > open {
			buf = append(buf, ',')
		}
		buf = append(buf, `"attributes":{`...)
		for i, a := range e.attrs {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = appendJSONString(buf, a.key)
			buf = append(buf, ':')
			buf = appendAttrJSON(buf, a.val)
		}
		buf = append(buf, '}')
	}
	if traces := e.Traces(); len(traces) > 0 {
		if len(buf) > open {
			buf = append(buf, ',')
		}
		buf = append(buf, `"stacktrace":[`...)
		for i, frame := range traces {
			if i > 0 {
				buf = append(buf, ',')
			}
			buf = appendJSONString(buf, frame)
		}
		buf = append(buf, ']')
	}
	if len(members) > 0 {
		shown := min(len(members), budget)
		budget -= shown
		if shown > 0 {
			if len(buf) > open {
				buf = append(buf, ',')
			}
			buf = append(buf, `"errors":[`...)
			for i, m := range members[:shown] {
				if i > 0 {
					buf = append(buf, ',')
				}
				buf, budget = appendErrorJSON(buf, m, budget)
			}
			buf = append(buf, ']')
		}
		if omitted := len(members) - shown; omitted > 0 {
			if len(buf) > open {
				buf = append(buf, ',')
			}
			buf = append(buf, `"errors_omitted":`...)
			buf = strconv.AppendInt(buf, int64(omitted), 10)
		}
	}
	return append(buf, '}'), budget
}

// errorText returns the message a rendered error reports: an *Error's own
// message, or Error() of anything else, guarded like an attribute value so a
// nil-ish member reads "<nil>" and a panicking Error() a "<panic: ...>"
// placeholder instead of crashing the encoder.
func errorText(err error) string {
	if e, ok := err.(*Error); ok && e != nil {
		return e.msg
	}
	msg, ok := errString(err)
	if !ok {
		return "<nil>"
	}
	return msg
}

func appendJSONField(buf []byte, open int, key, val string) []byte {
	if len(buf) > open {
		buf = append(buf, ',')
	}
	buf = appendJSONString(buf, key)
	buf = append(buf, ':')
	return appendJSONString(buf, val)
}

// appendJSONString appends s as a JSON string. A string of printable ASCII
// with nothing encoding/json would escape — nearly every code, key,
// message, and stack frame — is copied straight in; anything else goes
// through json.Marshal. The output is byte-identical to json.Marshal(s)
// either way, without boxing s or allocating a result for the common case.
func appendJSONString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			b, _ := json.Marshal(s)
			return append(buf, b...)
		}
	}
	buf = append(buf, '"')
	buf = append(buf, s...)
	return append(buf, '"')
}

// appendAttrJSON appends one attribute value. Plain strings, ints, and
// bools — the bulk of attribute values — are written directly, exactly as
// encoding/json writes them; everything else goes through attrJSON.
func appendAttrJSON(buf []byte, v any) []byte {
	switch x := v.(type) {
	case string:
		return appendJSONString(buf, x)
	case int:
		return strconv.AppendInt(buf, int64(x), 10)
	case bool:
		return strconv.AppendBool(buf, x)
	}
	return append(buf, attrJSON(v)...)
}

// attrJSON marshals one attribute value, never failing and never
// panicking: error values render as their message, values encoding/json
// rejects fall back to their fmt representation, and a value whose
// MarshalJSON (or String/Error) panics degrades to a "<panic: ...>"
// placeholder.
func attrJSON(v any) []byte {
	if _, ok := v.(json.Marshaler); !ok {
		if er, ok := v.(error); ok {
			msg, ok := errString(er)
			if !ok {
				return []byte("null")
			}
			return appendJSONString(nil, msg)
		}
	}
	return marshalValue(v)
}

// marshalValue encodes v with encoding/json, degrading a value json
// rejects to its fmt.Sprint form. A value whose MarshalJSON panics — or whose String/Error panics on the
// fmt fallback — is recovered and rendered as "<panic: ...>", so attrJSON
// upholds its no-panic contract for arbitrary json.Marshaler values
// (encoding/json does not recover panics from a value's own MarshalJSON).
func marshalValue(v any) (out []byte) {
	defer func() {
		if r := recover(); r != nil {
			out = appendJSONString(nil, fmt.Sprintf("<panic: %v>", r))
		}
	}()
	out, err := json.Marshal(v)
	if err != nil {
		out = appendJSONString(nil, fmt.Sprint(v))
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
