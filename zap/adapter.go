// Package aerrzap renders aerr errors into go.uber.org/zap log lines
// with their full structured payload (code, message, attributes,
// stacktrace).
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
	// The e != nil guard tolerates aerr v1.0.0, whose AsAerr returns
	// (nil, true) for a typed-nil *aerr.Error; falling through renders the
	// value safely instead of as an empty object.
	if e, ok := aerr.AsAerr(err); ok && e != nil {
		return zap.Object("error", aerrMarshaler{e: e})
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
	if e, ok := aerr.AsAerr(err); ok && e != nil {
		return aerrMarshaler{e: e}
	}
	return plainMarshaler{err: err}
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
	if code := m.e.Code(); code != "" {
		enc.AddString("code", code)
	}
	if msg := m.e.Error(); msg != "" {
		enc.AddString("message", msg)
	}
	if m.e.NumAttrs() > 0 {
		err := enc.AddObject("attributes", zapcore.ObjectMarshalerFunc(func(dict zapcore.ObjectEncoder) error {
			var addErr error
			m.e.RangeAttrs(func(k string, v any) bool {
				if addErr = addAttr(dict, k, v); addErr != nil {
					return false
				}
				return true
			})
			return addErr
		}))
		if err != nil {
			return err
		}
	}
	if traces := m.e.Traces(); len(traces) > 0 {
		err := enc.AddArray("stacktrace", zapcore.ArrayMarshalerFunc(func(arr zapcore.ArrayEncoder) error {
			for i := 0; i < len(traces); i++ {
				arr.AppendString(traces[i])
			}
			return nil
		}))
		if err != nil {
			return err
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
// without a fast path. It returns any encoding error so the caller can
// surface it instead of silently dropping the field: an AddReflected
// failure (e.g. a channel the JSON encoder cannot marshal) would
// otherwise leave the attribute out of the log line with no trace.
func addAttr(enc zapcore.ObjectEncoder, k string, v any) error {
	switch val := v.(type) {
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
		return enc.AddReflected(k, val)
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
