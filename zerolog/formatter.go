// Package aerrzerolog wires aerr errors into github.com/rs/zerolog so
// that the standard zerolog API renders aerr errors with their full
// structured payload (code, message, attributes, stacktrace).
//
// The payload is found anywhere in an error's chain, and nothing around it
// is dropped: an aerr error behind a wrapper such as fmt.Errorf with %w is
// rendered with the wrapper's full message, and an aggregate holding one
// (aerr.Join, errors.Join) lists its members under "errors", each with its
// own code, attributes, and stacktrace. At most 32 members are expanded per
// rendered error, counted across nesting; the rest are counted under
// "errors_omitted".
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
// instead of silently replacing them. (A chain that panics while being
// searched is rendered by aerr instead, so the panic cannot escape the log
// call.)
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
		if m := marshaller(err); m != nil {
			return m
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
	if m := marshaller(err); m != nil {
		return m
	}
	return plainMarshaller{err: err}
}

// AerrMarshalFunc returns a zerolog.LogObjectMarshaler when err carries an
// *aerr.Error somewhere in its chain. Non-aerr errors fall through to
// zerolog's default handling (except a chain that panics while being
// searched, which is rendered by aerr so the panic cannot escape the log
// call). It is exported for callers composing their own
// zerolog.ErrorMarshalFunc; most applications should call Register instead.
func AerrMarshalFunc(err error) any {
	if m := marshaller(err); m != nil {
		return m
	}
	return err
}

// marshaller returns the structured renderer for err when an *aerr.Error
// sits anywhere in its chain, and nil otherwise. A bare *aerr.Error — the
// common case — is recognized first, before the guarded chain search, and
// gets aerrMarshaller, which holds only the pointer and so converts to an
// interface without allocating. Anything else — a wrapper such as
// fmt.Errorf with %w, or an aggregate — is rendered from err itself by
// chainMarshaller, so the context it adds survives.
func marshaller(err error) zerolog.LogObjectMarshaler {
	if e, ok := err.(*aerr.Error); ok && e != nil {
		return aerrMarshaller{e: e}
	}
	if !carriesAerr(err) {
		return nil
	}
	return chainMarshaller{err: err}
}

// carriesAerr reports whether an *aerr.Error sits anywhere in err's chain.
// A chain that panics while being searched — a typed-nil wrapper whose
// Unwrap dereferences its receiver — counts as carrying one, so it is
// rendered by marshalError, which degrades the broken link rather than
// letting the panic escape the log call.
func carriesAerr(err error) (found bool) {
	defer func() {
		if recover() != nil {
			found = true
		}
	}()
	_, found = aerr.AsAerr(err)
	return found
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
	marshalError(evt, m.e, nil)
}

// chainMarshaller renders any other error — a wrapper, an aggregate, or an
// aggregate member of any type — by the same rules (see marshalError).
// budget is the member allowance shared across one log line; it is nil at
// the top level, where none has been spent yet.
type chainMarshaller struct {
	err    error
	budget *int
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler.
func (m chainMarshaller) MarshalZerologObject(evt *zerolog.Event) {
	marshalError(evt, m.err, m.budget)
}

// maxMembers caps how many aggregate members one rendered error expands,
// counted across every level of nesting, matching aerr's own JSON and slog
// rendering. The rest are counted under "errors_omitted", which keeps a
// large batch — or an aggregate that contains itself — from producing an
// unbounded line.
const maxMembers = 32

// maxLinks bounds how many single-cause links inspect follows, matching
// aerr's own rendering. Real chains are a handful of links deep; the bound
// only matters for one that loops — an Unwrap that returns its own receiver
// or an earlier link — which would otherwise hang the log call.
const maxLinks = 1024

// marshalError writes err's structured payload. The message is the one err
// itself reports, so context a wrapper adds (fmt.Errorf("op: %w", e)) is
// kept. Code, attributes, and stacktrace come from the nearest *aerr.Error
// on err's single-cause chain (the accessors are nil-safe, so an err with
// none simply omits them). When that chain reaches an aggregate, its
// members are rendered the same way under "errors" (see marshalMembers).
func marshalError(evt *zerolog.Event, err error, budget *int) {
	e, members := inspect(err)
	if code := e.Code(); code != "" {
		evt.Str("code", code)
	}
	var msg string
	if x, ok := err.(*aerr.Error); ok && x != nil {
		msg = x.Error()
	} else {
		msg = errMessage(err)
	}
	if msg != "" {
		evt.Str("message", msg)
	}
	if e.NumAttrs() > 0 {
		dict := zerolog.Dict()
		e.RangeAttrs(func(k string, v any) bool {
			appendAttr(dict, k, v)
			return true
		})
		evt.Dict("attributes", dict)
	}
	if traces := e.Traces(); len(traces) > 0 {
		evt.Strs("stacktrace", traces)
	}
	if len(members) > 0 {
		marshalMembers(evt, members, budget)
	}
}

// marshalMembers writes an aggregate's members under "errors" and the count
// left out by the maxMembers budget under "errors_omitted". All of an
// aggregate's direct members are allotted before any of them expands its
// own, and nested aggregates then draw on what remains, in order. The budget
// is created here, on first use, so a line without an aggregate never
// allocates it.
func marshalMembers(evt *zerolog.Event, members []error, budget *int) {
	if budget == nil {
		n := maxMembers
		budget = &n
	}
	shown := min(len(members), *budget)
	*budget -= shown
	if shown > 0 {
		arr := evt.CreateArray()
		for _, member := range members[:shown] {
			arr.Object(chainMarshaller{err: member, budget: budget})
		}
		evt.Array("errors", arr)
	}
	if omitted := len(members) - shown; omitted > 0 {
		evt.Int("errors_omitted", omitted)
	}
}

// inspect walks err's single-cause chain (Unwrap() error links) and returns
// the nearest *aerr.Error on it, plus the members of the aggregate
// (Unwrap() []error) the walk ends at, if any. Unlike aerr.AsAerr it never
// descends into an aggregate, so a bare aggregate is not rendered as if it
// were its first aerr member: each member is a separate failure and gets
// its own entry under "errors". A link whose Unwrap panics — typically a
// typed-nil wrapper that dereferences its receiver — ends the walk instead
// of the log call, as does a chain longer than maxLinks; whatever was found
// before that point is still rendered.
func inspect(err error) (e *aerr.Error, members []error) {
	defer func() { _ = recover() }()
	for i := 0; err != nil && i < maxLinks; i++ {
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
	// Redacted first so it never reaches Interface: the typed fast path keeps the mask zero-alloc and the plaintext off the encode buffer.
	case aerr.Redacted:
		dict.Str(k, aerr.RedactedText)
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
