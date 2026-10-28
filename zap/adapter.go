// Package aerrzap renders aerr errors into go.uber.org/zap log lines
// with their full structured payload (code, message, attributes,
// stacktrace).
//
// The payload is found anywhere in an error's chain, and nothing around it
// is dropped: an aerr error behind a wrapper such as fmt.Errorf with %w is
// rendered with the wrapper's full message, and an aggregate (aerr.Join,
// errors.Join) lists every member under "errors", each with its own code,
// attributes, and stacktrace.
//
// zap has no process-global error marshaler to register, so the idiomatic
// integration is a field constructor: pass Field wherever a zap.Error
// field would go, no registration needed:
//
//	logger.Error("request failed", aerrzap.Field(err))
//
// Code that wants to choose its own key can compose the raw marshaler
// with zap.Object instead:
//
//	logger.Error("request failed", zap.Object("err", aerrzap.Object(err)))
package aerrzap

import (
	"fmt"
	"reflect"
	"time"

	"github.com/tafaquh/aerr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field renders err under the key "error". When err carries an
// *aerr.Error anywhere in its chain the field is a nested object with
// code, message, attributes, and stacktrace; otherwise it falls back to
// zap.Error, so Field is always a safe drop-in replacement. A nil err
// produces a no-op field, matching zap.Error's behavior.
func Field(err error) zap.Field {
	if err == nil {
		return zap.Skip()
	}
	if m := marshaler(err); m != nil {
		return zap.Object("error", m)
	}
	return zap.Error(err)
}

// Object renders err as a zapcore.ObjectMarshaler with aerr's structured
// payload, for callers composing fields themselves under a key of their
// choosing:
//
//	logger.Error("request failed", zap.Object("err", aerrzap.Object(err)))
//
// When err carries no *aerr.Error the object contains only the error
// message.
func Object(err error) zapcore.ObjectMarshaler {
	if m := marshaler(err); m != nil {
		return m
	}
	return plainMarshaler{err: err}
}

// marshaler returns the structured renderer for err when an *aerr.Error
// sits anywhere in its chain, and nil otherwise.
func marshaler(err error) zapcore.ObjectMarshaler {
	// The e != nil guard tolerates aerr v1.0.0, whose AsAerr returns
	// (nil, true) for a typed-nil *aerr.Error; falling through renders the
	// value safely instead of as an empty object.
	if e, ok := aerr.AsAerr(err); !ok || e == nil {
		return nil
	}
	return objectFor(err)
}

// objectFor returns the renderer for err. A bare *aerr.Error gets
// aerrMarshaler, which holds only the pointer and so converts to an
// interface without allocating. Anything else — a wrapper such as
// fmt.Errorf with %w, an aggregate, or a plain aggregate member — is
// rendered from err itself by chainMarshaler, so the context it adds
// survives.
func objectFor(err error) zapcore.ObjectMarshaler {
	if e, ok := err.(*aerr.Error); ok && e != nil {
		return aerrMarshaler{e: e}
	}
	return chainMarshaler{err: err}
}

// aerrMarshaler renders an *aerr.Error directly into a zapcore encoder,
// avoiding the map/reflection path of zap.Any.
type aerrMarshaler struct {
	e *aerr.Error
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
func (m aerrMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m.e == nil {
		return nil
	}
	return marshalError(enc, m.e)
}

// chainMarshaler renders an error that is not itself an *aerr.Error by the
// same rules as aerrMarshaler (see marshalError).
type chainMarshaler struct {
	err error
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
func (m chainMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	return marshalError(enc, m.err)
}

// marshalError writes err's structured payload. The message is the one err
// itself reports, so context a wrapper adds (fmt.Errorf("op: %w", e)) is
// kept. Code, attributes, and stacktrace come from the nearest *aerr.Error
// on err's single-cause chain (the accessors are nil-safe, so an err with
// none simply omits them). When that chain reaches an aggregate, every
// member is rendered the same way under "errors".
func marshalError(enc zapcore.ObjectEncoder, err error) error {
	e, members := inspect(err)
	if code := e.Code(); code != "" {
		enc.AddString("code", code)
	}
	var msg string
	if x, ok := err.(*aerr.Error); ok && x != nil {
		msg = x.Error()
	} else {
		msg = errMessage(err)
	}
	if msg != "" {
		enc.AddString("message", msg)
	}
	if e.NumAttrs() > 0 {
		encErr := enc.AddObject("attributes", zapcore.ObjectMarshalerFunc(func(dict zapcore.ObjectEncoder) error {
			var addErr error
			e.RangeAttrs(func(k string, v any) bool {
				if addErr = addAttr(dict, k, v); addErr != nil {
					return false
				}
				return true
			})
			return addErr
		}))
		if encErr != nil {
			return encErr
		}
	}
	if traces := e.Traces(); len(traces) > 0 {
		encErr := enc.AddArray("stacktrace", zapcore.ArrayMarshalerFunc(func(arr zapcore.ArrayEncoder) error {
			for i := 0; i < len(traces); i++ {
				arr.AppendString(traces[i])
			}
			return nil
		}))
		if encErr != nil {
			return encErr
		}
	}
	if len(members) > 0 {
		return enc.AddArray("errors", errorArray(members))
	}
	return nil
}

// inspect walks err's single-cause chain (Unwrap() error links) and returns
// the nearest *aerr.Error on it, plus the members of the aggregate
// (Unwrap() []error) the walk ends at, if any. Unlike aerr.AsAerr it never
// descends into an aggregate, so a bare aggregate is not rendered as if it
// were its first aerr member: each member is a separate failure and gets
// its own entry under "errors".
func inspect(err error) (e *aerr.Error, members []error) {
	for err != nil {
		switch x := err.(type) {
		case *aerr.Error:
			if x == nil {
				return e, nil
			}
			if e == nil {
				e = x
			}
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			return e, x.Unwrap()
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		default:
			return e, nil
		}
	}
	return e, nil
}

// errorArray adapts an aggregate's members to zapcore.ArrayMarshaler,
// rendering each one as an object by the same rules as its parent.
type errorArray []error

// MarshalLogArray implements zapcore.ArrayMarshaler.
func (es errorArray) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for _, err := range es {
		if encErr := arr.AppendObject(objectFor(err)); encErr != nil {
			return encErr
		}
	}
	return nil
}

// plainMarshaler renders a non-aerr error for Object.
type plainMarshaler struct {
	err error
}

// MarshalLogObject implements zapcore.ObjectMarshaler.
//
// A genuinely-nil error yields an empty object. For any non-nil error the
// message is rendered through errMessage, so a typed-nil or panicking
// Error implementation cannot crash the logger.
func (m plainMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m.err == nil {
		return nil
	}
	enc.AddString("message", errMessage(m.err))
	return nil
}

// addAttr writes one attribute through zapcore's typed appenders, falling
// back to AddReflected (reflection + encoding/json) only for types
// without a fast path. A value AddReflected cannot encode (e.g. a channel
// or func the JSON encoder rejects) degrades to its fmt form via AddString
// rather than propagating the error: returning an error here aborts the
// whole MarshalLogObject, dropping every later attribute AND the
// stacktrace, not just the offending value. Degrading matches the core
// JSON and zerolog renderers, which keep going past an unencodable value.
func addAttr(enc zapcore.ObjectEncoder, k string, v any) error {
	switch val := v.(type) {
	// Redacted first so it never reaches AddReflected: the typed fast path keeps the mask zero-alloc and the plaintext off the encode buffer.
	case aerr.Redacted:
		enc.AddString(k, aerr.RedactedText)
	case string:
		enc.AddString(k, val)
	case int:
		enc.AddInt(k, val)
	case int64:
		enc.AddInt64(k, val)
	case uint64:
		enc.AddUint64(k, val)
	case bool:
		enc.AddBool(k, val)
	case float64:
		enc.AddFloat64(k, val)
	case float32:
		enc.AddFloat32(k, val)
	case time.Time:
		enc.AddTime(k, val)
	case time.Duration:
		enc.AddDuration(k, val)
	case []string:
		return enc.AddArray(k, stringArray(val))
	case []byte:
		enc.AddByteString(k, val)
	case error:
		enc.AddString(k, errMessage(val))
	default:
		if err := enc.AddReflected(k, val); err != nil {
			// AddReflected fails (writing nothing) for values the JSON
			// encoder cannot handle. Degrade to the fmt form and keep going
			// so the remaining attributes and the stacktrace still render.
			enc.AddString(k, fmt.Sprint(val))
		}
	}
	return nil
}

// stringArray adapts a []string to zapcore.ArrayMarshaler without the
// per-element interface boxing of zap.Strings.
type stringArray []string

// MarshalLogArray implements zapcore.ArrayMarshaler.
func (ss stringArray) MarshalLogArray(arr zapcore.ArrayEncoder) error {
	for i := 0; i < len(ss); i++ {
		arr.AppendString(ss[i])
	}
	return nil
}

// errMessage returns err's message, tolerating typed-nil errors and
// Error implementations that panic; a logging path must never crash the
// process it is observing. zapcore recovers such panics only in its
// ErrorType/StringerType encoders, not on the ObjectMarshaler paths this
// adapter uses, so the recover below is what actually protects us. A
// value-receiver error whose Error() dereferences a nil field has
// reflect.Kind Struct, slipping past both nil-interface and pointer-nil
// guards; the switch covers every nilable kind, and any residual panic is
// rendered as "<panic: ...>". Nil-ish values render as "<nil>", the same
// convention zapcore's error encoder uses.
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
