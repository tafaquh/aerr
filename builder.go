package aerr

import (
	"errors"
	"fmt"
)

// Builder fluently configures an *Error. Each setter mutates the receiver
// and returns it so calls can be chained. Finalizing (Err / ErrMsg / Wrap)
// copies the builder's state into the issued *Error, so a Builder may be
// kept and reused as a template for further errors. A Builder is not safe
// for concurrent use.
type Builder struct {
	code         string
	msg          string
	attrs        []attr
	captureStack bool
}

// attr is an ordered key/value pair. Using a slice instead of a map keeps
// insertion order deterministic and avoids the map header allocation for
// the common case of a handful of attributes.
type attr struct {
	key string
	val any
}

// Code begins a new chain with the given error code.
func Code(code string) *Builder {
	return &Builder{code: code}
}

// Message begins a new chain with the given message.
func Message(msg string) *Builder {
	return &Builder{msg: msg}
}

// StackTrace begins a new chain with stack capture enabled.
func StackTrace() *Builder {
	return &Builder{captureStack: true}
}

// Messagef begins a new chain with a printf-style message.
func Messagef(format string, args ...any) *Builder {
	return &Builder{msg: fmt.Sprintf(format, args...)}
}

// ErrMsg is a shortcut for Message(msg).Err(nil) that skips Builder
// allocation entirely.
func ErrMsg(msg string) error {
	return &Error{msg: msg}
}

// Errorf is a printf-style shortcut for Messagef(...).Err(nil).
func Errorf(format string, args ...any) error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

// Wrapf wraps err with a printf-style message, following the same merge
// rules as (*Builder).Wrap. Returns nil when err is nil.
func Wrapf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	b := Builder{msg: fmt.Sprintf(format, args...)}
	return b.finalize(err, finalizeSkip)
}

// Code sets the error code.
func (b *Builder) Code(code string) *Builder {
	b.code = code
	return b
}

// Message sets the message.
func (b *Builder) Message(msg string) *Builder {
	b.msg = msg
	return b
}

// Messagef sets a printf-style message.
func (b *Builder) Messagef(format string, args ...any) *Builder {
	b.msg = fmt.Sprintf(format, args...)
	return b
}

// StackTrace enables stack capture on the next Err / ErrMsg / Wrap.
func (b *Builder) StackTrace() *Builder {
	b.captureStack = true
	return b
}

// With adds a key/value attribute. When the key is already present its
// value is overwritten in place; insertion order is preserved.
func (b *Builder) With(key string, value any) *Builder {
	for i := range b.attrs {
		if b.attrs[i].key == key {
			b.attrs[i].val = value
			return b
		}
	}
	if b.attrs == nil {
		b.attrs = make([]attr, 0, 4)
	}
	b.attrs = append(b.attrs, attr{key: key, val: value})
	return b
}

// Err finalizes the builder. When cause is non-nil it is recorded as the
// underlying error and its message is appended to the builder's message
// with ": " as separator. When the cause chain contains an *Error (even
// behind non-aerr wrappers such as fmt.Errorf with %w), its code is
// inherited when the builder has none, its attributes merge under the
// outer-wins rule, and its stack trace is inherited.
func (b *Builder) Err(cause error) error {
	return b.finalize(cause, finalizeSkip)
}

// ErrMsg finalizes the builder using msg as a plain-text cause.
// ErrMsg("") is equivalent to Err(nil).
func (b *Builder) ErrMsg(msg string) error {
	if msg == "" {
		return b.finalize(nil, finalizeSkip)
	}
	return b.finalize(errors.New(msg), finalizeSkip)
}

// Wrap finalizes the builder wrapping err, following the same merge rules
// as Err. Returns nil when err is nil.
//
// The deepest stack wins: when the wrapped chain already carries a stack
// trace it is inherited and an outer StackTrace() request is a no-op, so
// each chain captures at most once and traces always point at the origin.
func (b *Builder) Wrap(err error) error {
	if err == nil {
		return nil
	}
	return b.finalize(err, finalizeSkip)
}

// finalizeSkip is the number of frames between runtime.Callers and the
// user call site on the finalize path: runtime.Callers, captureStack,
// finalize, and the exported finalizer (Err / ErrMsg / Wrap).
const finalizeSkip = 4

// finalize builds the immutable *Error from the builder's state. The
// builder's attribute slice is copied so the issued error owns its memory
// and later reuse of the builder cannot mutate it. skip is forwarded to
// captureStack and must count the frames between runtime.Callers and the
// user call site.
func (b *Builder) finalize(cause error, skip int) *Error {
	e := &Error{
		code:  b.code,
		msg:   b.msg,
		cause: cause,
	}
	var inner *Error
	if cause != nil {
		e.msg = joinMsg(e.msg, cause.Error())
		inner, _ = AsAerr(cause)
	}
	extra := 0
	if inner != nil {
		extra = len(inner.attrs)
	}
	if n := len(b.attrs); n+extra > 0 {
		attrs := make([]attr, n, n+extra)
		copy(attrs, b.attrs)
		e.attrs = attrs
	}
	if inner != nil {
		if e.code == "" {
			e.code = inner.code
		}
		e.attrs = mergeAttrs(e.attrs, inner.attrs)
		e.pcs = inner.pcs
	}
	if b.captureStack && len(e.pcs) == 0 {
		e.pcs = captureStack(skip)
	}
	return e
}

// joinMsg returns left + ": " + right, dropping the separator when either
// side is empty.
func joinMsg(left, right string) string {
	switch {
	case right == "":
		return left
	case left == "":
		return right
	}
	return left + ": " + right
}

// mergeAttrs returns dst extended with entries from src whose keys are not
// already present, preserving src order.
func mergeAttrs(dst, src []attr) []attr {
	if len(src) == 0 {
		return dst
	}
	if cap(dst)-len(dst) < len(src) {
		grown := make([]attr, len(dst), len(dst)+len(src))
		copy(grown, dst)
		dst = grown
	}
next:
	for _, a := range src {
		for i := range dst {
			if dst[i].key == a.key {
				continue next
			}
		}
		dst = append(dst, a)
	}
	return dst
}
