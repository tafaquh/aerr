package aerr

import (
	"log/slog"
	"strconv"
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
// When the error wraps an aggregate (see [Join]), its members follow under
// errors, each rendered the same way. slog has no array kind for groups, so
// errors is a group keyed by position ("0", "1", ...), which keeps every
// member value visible to LogValuer resolution and to a
// HandlerOptions.ReplaceAttr hook, at group paths such as
// ["err", "errors", "0", "attributes"]. At most 32 members are expanded,
// counted across nesting; the rest are counted under errors_omitted.
func (e *Error) LogValue() slog.Value {
	if e == nil {
		return slog.Value{}
	}
	v, _ := logValue(e, maxMembers)
	return v
}

// logValue renders err as a slog group by the same rules as
// appendErrorJSON: the message err itself reports; code, attributes, and
// stacktrace from the nearest *Error on its single-cause chain; then the
// members of an aggregate on that chain. budget is the member allowance left
// (see maxMembers); what remains of it is returned.
func logValue(err error, budget int) (slog.Value, int) {
	e, members := inspect(err)
	out := make([]slog.Attr, 0, 4)
	if msg := errorText(err); msg != "" {
		out = append(out, slog.String("message", msg))
	}
	if code := e.Code(); code != "" {
		out = append(out, slog.String("code", code))
	}
	if e != nil && len(e.attrs) > 0 {
		sub := make([]slog.Attr, len(e.attrs))
		for i, a := range e.attrs {
			sub[i] = slog.Any(a.key, a.val)
		}
		out = append(out, slog.Attr{Key: "attributes", Value: slog.GroupValue(sub...)})
	}
	if traces := e.Traces(); len(traces) > 0 {
		out = append(out, slog.Any("stacktrace", traces))
	}
	if len(members) > 0 {
		shown := min(len(members), budget)
		budget -= shown
		if shown > 0 {
			group := make([]slog.Attr, shown)
			for i, m := range members[:shown] {
				var v slog.Value
				v, budget = logValue(m, budget)
				if len(v.Group()) == 0 {
					// slog elides an empty group, which would drop this
					// member and leave a gap in the positional keys.
					v = slog.GroupValue(slog.String("message", ""))
				}
				group[i] = slog.Attr{Key: strconv.Itoa(i), Value: v}
			}
			out = append(out, slog.Attr{Key: "errors", Value: slog.GroupValue(group...)})
		}
		if omitted := len(members) - shown; omitted > 0 {
			out = append(out, slog.Int("errors_omitted", omitted))
		}
	}
	return slog.GroupValue(out...), budget
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

// maxMembers caps how many aggregate members one rendered error expands,
// counted across every level of nesting; the rest are counted under
// "errors_omitted". Within each aggregate, all direct members are allotted
// before any of them expands its own, and nested aggregates then draw on
// what remains, in order. The cap keeps a large batch — or an aggregate that
// contains itself — from producing an unbounded record. The logging adapters
// apply the same cap.
const maxMembers = 32

// maxLinks bounds how many single-cause links inspect follows. Real chains
// are a handful of links deep; the bound only matters for one that loops —
// an Unwrap that returns its own receiver or an earlier link — which would
// otherwise hang the renderer.
const maxLinks = 1024

// inspect walks err's single-cause chain (Unwrap() error links) and returns
// the nearest *Error on it, plus the members of the aggregate
// (Unwrap() []error) the walk ends at, if any. It drives every structured
// rendering. Unlike AsAerr it never descends into an aggregate: members are
// separate failures, each rendered on its own under "errors", so none is
// presented as the whole. A link whose Unwrap panics — typically a typed-nil
// wrapper that dereferences its receiver — ends the walk instead of the
// rendering, as does a chain longer than maxLinks; whatever was found before
// that point is still rendered.
func inspect(err error) (e *Error, members []error) {
	defer func() { _ = recover() }()
	for i := 0; err != nil && i < maxLinks; i++ {
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
