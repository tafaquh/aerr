// Package aerr provides structured errors that carry an error code, a
// message, arbitrary key/value attributes, and an optional stack trace.
// Errors are built with a fluent *Builder API and implement slog.LogValuer
// so they integrate directly with log/slog and the optional zerolog
// adapter in the github.com/tafaquh/aerr/zerolog package.
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
