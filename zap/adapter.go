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
	if e, ok := aerr.AsAerr(err); ok {
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
	if e, ok := aerr.AsAerr(err); ok {
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
			m.e.RangeAttrs(func(k string, v any) bool {
				addAttr(dict, k, v)
				return true
			})
			return nil
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
func (m plainMarshaler) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if m.err == nil {
		return nil
	}
	enc.AddString("message", m.err.Error())
	return nil
}

// addAttr writes one attribute through zapcore's typed appenders, falling
// back to AddReflected (reflection + encoding/json) only for types
// without a fast path.
func addAttr(enc zapcore.ObjectEncoder, k string, v any) {
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
		_ = enc.AddArray(k, stringArray(val))
	case []byte:
		enc.AddByteString(k, val)
	case error:
		enc.AddString(k, errMessage(val))
	default:
		_ = enc.AddReflected(k, val)
	}
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

// errMessage returns an error attribute's message, tolerating typed-nil
// values whose Error method would dereference a nil receiver. It renders
// them as "<nil>", the same convention zapcore's error encoder uses.
func errMessage(val error) string {
	if val == nil {
		return "<nil>"
	}
	if rv := reflect.ValueOf(val); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return "<nil>"
	}
	return val.Error()
}
