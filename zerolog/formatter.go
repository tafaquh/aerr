// Package aerrzerolog wires aerr errors into github.com/rs/zerolog so
// that the standard zerolog API renders aerr errors with their full
// structured payload (code, message, attributes, stacktrace).
//
// There are two ways to use it. Applications that want every
// logger.Err(err) / Error().Err(err) call to render aerr errors
// structurally should call Register once in main, following the same
// convention as zerolog's own pkgerrors helper:
//
//	func main() {
//		aerrzerolog.Register()
//		...
//	}
//
// Code that prefers no process-global configuration can render a single
// error explicitly with Object:
//
//	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("failed")
package aerrzerolog

import (
	"fmt"
	"reflect"
	"time"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
)

// errMessage returns err's message, tolerating typed-nil errors and
// Error implementations that panic; a logging path must never crash
// the process it is observing. A value-receiver error whose Error()
// dereferences a nil field has reflect.Kind Struct, so it slips past
// both nil-interface and pointer-nil guards; the recover below is what
// actually protects against it.
func errMessage(err error) (msg string) {
	if err == nil {
		return "<nil>"
	}
	defer func() {
		if r := recover(); r != nil {
			msg = fmt.Sprintf("<panic: %v>", r)
		}
	}()
	rv := reflect.ValueOf(err)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		if rv.IsNil() {
			return "<nil>"
		}
	}
	return err.Error()
}

// Register installs aerr rendering into zerolog's process-wide
// ErrorMarshalFunc. Errors that do not carry an *aerr.Error anywhere in
// their chain are delegated to the marshal func that was active before
// Register was called, so aerr coexists with other error customizations
// instead of silently replacing them.
//
// Register leaves zerolog.ErrorStackMarshaler untouched: an aerr error's
// stack trace is always rendered inside the error object itself, so
// calling .Stack() is unnecessary (and harmless) for aerr errors, and any
// stack marshaler configured for other error types keeps working.
//
// Register mutates zerolog package state; call it once from main, not
// from library code.
func Register() {
	prev := zerolog.ErrorMarshalFunc
	zerolog.ErrorMarshalFunc = func(err error) any {
		// The e != nil guards tolerate aerr v1.0.0, whose AsAerr returns
		// (nil, true) for a typed-nil *aerr.Error; falling through renders the
		// value safely instead of as an empty object.
		if e, ok := aerr.AsAerr(err); ok && e != nil {
			return aerrMarshaller{e: e}
		}
		if prev != nil {
			return prev(err)
		}
		return err
	}
}

// Object renders err as a zerolog.LogObjectMarshaler with aerr's
// structured payload, without touching any global state:
//
//	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("failed")
//
// When err carries no *aerr.Error the object contains only the error
// message.
func Object(err error) zerolog.LogObjectMarshaler {
	if e, ok := aerr.AsAerr(err); ok && e != nil {
		return aerrMarshaller{e: e}
	}
	return plainMarshaller{err: err}
}

// AerrMarshalFunc returns a zerolog.LogObjectMarshaler when err carries an
// *aerr.Error somewhere in its chain. Non-aerr errors fall through to
// zerolog's default handling. It is exported for callers composing their
// own zerolog.ErrorMarshalFunc; most applications should call Register
// instead.
func AerrMarshalFunc(err error) any {
	if e, ok := aerr.AsAerr(err); ok && e != nil {
		return aerrMarshaller{e: e}
	}
	return err
}

// AerrStackMarshaler exposes aerr's captured stack to zerolog's Stack()
// builder, for callers who assign zerolog.ErrorStackMarshaler themselves
// and want the trace as a top-level field.
//
// Deprecated: Register no longer installs this. The stack is already
// rendered inside the error object, so installing AerrStackMarshaler and
// calling .Stack() duplicates the trace in every log line.
func AerrStackMarshaler(err error) any {
	if e, ok := aerr.AsAerr(err); ok && e != nil {
		if stack := e.Traces(); len(stack) > 0 {
			return stack
		}
	}
	return nil
}

// aerrMarshaller renders an *aerr.Error directly into a zerolog event,
// avoiding the map/reflection path of zerolog.Event.Interface.
type aerrMarshaller struct {
	e *aerr.Error
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler.
func (m aerrMarshaller) MarshalZerologObject(evt *zerolog.Event) {
	if m.e == nil {
		return
	}
	if code := m.e.Code(); code != "" {
		evt.Str("code", code)
	}
	if msg := m.e.Error(); msg != "" {
		evt.Str("message", msg)
	}
	if m.e.NumAttrs() > 0 {
		dict := zerolog.Dict()
		m.e.RangeAttrs(func(k string, v any) bool {
			appendAttr(dict, k, v)
			return true
		})
		evt.Dict("attributes", dict)
	}
	if traces := m.e.Traces(); len(traces) > 0 {
		evt.Strs("stacktrace", traces)
	}
}

// plainMarshaller renders a non-aerr error for Object.
type plainMarshaller struct {
	err error
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler.
//
// A genuinely-nil error yields an empty object (documented). For any
// non-nil error the message is rendered through errMessage, so a
// typed-nil or panicking Error implementation cannot crash the logger.
func (m plainMarshaller) MarshalZerologObject(evt *zerolog.Event) {
	if m.err == nil {
		return
	}
	evt.Str("message", errMessage(m.err))
}

// appendAttr writes one attribute through zerolog's typed appenders,
// falling back to Interface (reflection + encoding/json) only for types
// without a fast path. The typed paths write zero-allocation; Interface
// costs ~2 allocs per value.
func appendAttr(dict *zerolog.Event, k string, v any) {
	switch val := v.(type) {
	case string:
		dict.Str(k, val)
	case int:
		dict.Int(k, val)
	case int64:
		dict.Int64(k, val)
	case uint64:
		dict.Uint64(k, val)
	case bool:
		dict.Bool(k, val)
	case float64:
		dict.Float64(k, val)
	case float32:
		dict.Float32(k, val)
	case time.Time:
		dict.Time(k, val)
	case time.Duration:
		dict.Dur(k, val)
	case []string:
		dict.Strs(k, val)
	case []byte:
		dict.Bytes(k, val)
	case error:
		// zerolog's AnErr omits the key entirely for nil-ish values and
		// calls Error() without recovering, so a typed-nil or panicking
		// error would either vanish from the log or crash the process.
		// Render the message through errMessage instead: the key is
		// always present, with "<nil>"/"<panic: ...>" for those cases.
		dict.Str(k, errMessage(val))
	default:
		dict.Interface(k, val)
	}
}
