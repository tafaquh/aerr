// Package aerrzerolog wires aerr errors into github.com/rs/zerolog so
// that the standard zerolog API renders aerr errors with their full
// structured payload.
//
// Importing this package has a side effect: it overwrites the global
// zerolog.ErrorMarshalFunc and zerolog.ErrorStackMarshaler. Use a blank
// import to enable the integration:
//
//	import _ "github.com/tafaquh/aerr/zerolog"
package aerrzerolog

import (
	"time"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
)

func init() {
	zerolog.ErrorMarshalFunc = AerrMarshalFunc
	zerolog.ErrorStackMarshaler = AerrStackMarshaler
}

// AerrMarshalFunc returns a zerolog.LogObjectMarshaler when err carries an
// *aerr.Error somewhere in its chain. Non-aerr errors fall through to
// zerolog's default handling.
func AerrMarshalFunc(err error) any {
	if e, ok := aerr.AsAerr(err); ok {
		return aerrMarshaller{e: e}
	}
	return err
}

// AerrStackMarshaler exposes aerr's captured stack to zerolog's Stack()
// builder. It returns nil when there is no stack to render so zerolog
// can omit the field entirely.
func AerrStackMarshaler(err error) any {
	if e, ok := aerr.AsAerr(err); ok {
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
		// AnErr handles typed-nil errors via zerolog's isNilValue and
		// renders the message instead of Interface's empty "{}".
		dict.AnErr(k, val)
	default:
		dict.Interface(k, val)
	}
}
