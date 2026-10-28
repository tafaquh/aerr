package aerr

import (
	"log/slog"
	"sync"
)

// Error is the immutable error value produced by *Builder.Err and
// *Builder.Wrap. It implements error, slog.LogValuer, and supports
// errors.Is / errors.As via the embedded cause.
//
// Once an *Error has been returned from a *Builder method it must not be
// mutated; the type does not expose any setter.
type Error struct {
	code  string
	msg   string
	cause error
	attrs []attr
	pcs   []uintptr

	// traces caches the rendered stack so repeated logging of the same
	// error symbolizes the PCs only once. Guarded by traceOnce, which
	// keeps the lazy render safe under concurrent LogValue calls.
	traceOnce sync.Once
	traces    []string
}

// Error returns the combined message of the error chain.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// Unwrap returns the wrapped cause, if any.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Code returns the error code, or "" when unset.
func (e *Error) Code() string {
	if e == nil {
		return ""
	}
	return e.code
}

// NumAttrs returns the number of attached attributes.
func (e *Error) NumAttrs() int {
	if e == nil {
		return 0
	}
	return len(e.attrs)
}

// RangeAttrs invokes fn for each attribute in insertion order. Iteration
// stops early if fn returns false.
func (e *Error) RangeAttrs(fn func(key string, value any) bool) {
	if e == nil {
		return
	}
	for _, a := range e.attrs {
		if !fn(a.key, a.val) {
			return
		}
	}
}

// Attributes returns the attributes as a fresh map. Prefer RangeAttrs in
// hot paths to avoid the allocation.
func (e *Error) Attributes() map[string]any {
	if e == nil || len(e.attrs) == 0 {
		return nil
	}
	out := make(map[string]any, len(e.attrs))
	for _, a := range e.attrs {
		out[a.key] = a.val
	}
	return out
}

// Traces returns the formatted stack trace, or nil when none was captured.
// The rendering is computed on first use and cached for the life of the
// error; callers must treat the returned slice as read-only.
func (e *Error) Traces() []string {
	if e == nil || len(e.pcs) == 0 {
		return nil
	}
	e.traceOnce.Do(func() {
		e.traces = renderTraces(e.pcs)
	})
	return e.traces
}

// LogValue implements slog.LogValuer, producing a group with the keys
// message, code, attributes, and stacktrace (each emitted only when set).
// When the error wraps an aggregate (see [Join]), an errors key follows,
// listing every member with its own code, attributes, and stack trace.
// slog has no array-of-groups kind, so the members render through the same
// JSON encoder as [Error.MarshalJSON]; a HandlerOptions.ReplaceAttr hook
// therefore sees this error's own attributes but not a member's — mask
// sensitive member values with [Redact] or [RedactKeys], which apply on
// every path.
func (e *Error) LogValue() slog.Value {
	if e == nil {
		return slog.Value{}
	}
	out := make([]slog.Attr, 0, 4)
	if e.msg != "" {
		out = append(out, slog.String("message", e.msg))
	}
	if e.code != "" {
		out = append(out, slog.String("code", e.code))
	}
	if len(e.attrs) > 0 {
		sub := make([]slog.Attr, len(e.attrs))
		for i, a := range e.attrs {
			sub[i] = slog.Any(a.key, a.val)
		}
		out = append(out, slog.Attr{Key: "attributes", Value: slog.GroupValue(sub...)})
	}
	if traces := e.Traces(); len(traces) > 0 {
		out = append(out, slog.Any("stacktrace", traces))
	}
	if _, members := inspect(e); len(members) > 0 {
		out = append(out, slog.Any("errors", errorList(members)))
	}
	return slog.GroupValue(out...)
}

// HasCode reports whether any *Error in err's chain carries the given
// code, walking both Unwrap() error and Unwrap() []error links. Unlike
// AsAerr followed by Code, it sees codes that outer errors did not
// inherit: every aerr layer of the chain is checked individually.
//
// The empty string never matches: a code of "" is treated as unset, so
// HasCode(err, "") is always false even when the chain contains *Error
// values whose code was never set.
func HasCode(err error, code string) bool {
	if code == "" {
		return false
	}
	for err != nil {
		if e, ok := err.(*Error); ok && e != nil && e.code == code {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, sub := range x.Unwrap() {
				if HasCode(sub, code) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
	return false
}

// inspect walks err's single-cause chain (Unwrap() error links) and returns
// the nearest *Error on it, plus the members of the aggregate
// (Unwrap() []error) the walk ends at, if any. It drives every structured
// rendering. Unlike AsAerr it never descends into an aggregate: members are
// separate failures, each rendered on its own under "errors", so none is
// presented as the whole.
func inspect(err error) (e *Error, members []error) {
	for err != nil {
		switch x := err.(type) {
		case *Error:
			if x == nil {
				return e, nil
			}
			if e == nil {
				e = x
			}
			err = x.cause
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

// AsAerr extracts an *Error from anywhere in err's chain, walking both
// Unwrap() error and Unwrap() []error links without reflection. The first
// return value is non-nil when the second is true; a typed-nil *Error in
// the chain does not count as a match.
func AsAerr(err error) (*Error, bool) {
	for err != nil {
		if e, ok := err.(*Error); ok && e != nil {
			return e, true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
		case interface{ Unwrap() []error }:
			for _, sub := range x.Unwrap() {
				if e, ok := AsAerr(sub); ok {
					return e, true
				}
			}
			return nil, false
		default:
			return nil, false
		}
	}
	return nil, false
}
