package aerr

import "errors"

// Builder fluently configures an *Error. Each setter mutates the receiver
// and returns it so calls can be chained. A Builder is not safe for
// concurrent use and should be discarded after Err / Wrap / ErrMsg.
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

// ErrMsg is a shortcut for Message(msg).Err(nil) that skips Builder
// allocation entirely.
func ErrMsg(msg string) error {
	return &Error{msg: msg}
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

// StackTrace enables stack capture on the next Err / Wrap.
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
// with ": " as separator.
func (b *Builder) Err(cause error) error {
	e := &Error{
		code:  b.code,
		msg:   b.msg,
		attrs: b.attrs,
	}
	if cause != nil {
		e.cause = cause
		if ce, ok := cause.(*Error); ok {
			absorb(e, ce)
		} else {
			e.msg = joinMsg(e.msg, cause.Error())
		}
	}
	if b.captureStack && len(e.pcs) == 0 {
		e.pcs = captureStack()
	}
	return e
}

// ErrMsg finalizes the builder using msg as a plain-text cause.
// ErrMsg("") is equivalent to Err(nil).
func (b *Builder) ErrMsg(msg string) error {
	if msg == "" {
		return b.Err(nil)
	}
	return b.Err(errors.New(msg))
}

// Wrap finalizes the builder wrapping err. When err is an *Error the outer
// wins on conflicts: its code is preserved, its attribute values shadow
// inner duplicates, and the inner stack trace is inherited only when the
// outer did not request its own. Returns nil when err is nil.
func (b *Builder) Wrap(err error) error {
	if err == nil {
		return nil
	}
	e := &Error{
		code:  b.code,
		msg:   b.msg,
		attrs: b.attrs,
		cause: err,
	}
	if ce, ok := err.(*Error); ok {
		absorb(e, ce)
		if b.captureStack {
			e.pcs = captureStack()
		}
		return e
	}
	e.msg = joinMsg(e.msg, err.Error())
	if b.captureStack {
		e.pcs = captureStack()
	}
	return e
}

// absorb merges fields from inner into outer following the outer-wins rule.
// Inner stack is inherited only when outer has none.
func absorb(outer, inner *Error) {
	if outer.code == "" {
		outer.code = inner.code
	}
	outer.msg = joinMsg(outer.msg, inner.msg)
	outer.attrs = mergeAttrs(outer.attrs, inner.attrs)
	if len(outer.pcs) == 0 {
		outer.pcs = inner.pcs
	}
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
	out := make([]byte, 0, len(left)+2+len(right))
	out = append(out, left...)
	out = append(out, ':', ' ')
	out = append(out, right...)
	return string(out)
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
