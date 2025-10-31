package aerr

// Package aerr provides simple error logging with stack traces
import (
	"fmt"
	"log/slog"
	"runtime"
)

// aerr provides a fluent API for constructing errors.
type aerr struct {
	msg       string
	code      string
	cause     error
	stack     []uintptr
	skipStack bool
	fields    map[string]any
}

// Error implements the error interface.
func (e *aerr) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// Unwrap implements error unwrapping.
func (e *aerr) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// LogValue implements slog.LogValuer for automatic structured logging.
// It flattens nested errors into an array instead of deeply nested objects.
func (e *aerr) LogValue() slog.Value {
	if e == nil {
		return slog.Value{}
	}

	// Collect all errors in the chain
	var errors []map[string]any

	// Walk through the error chain
	current := error(e)
	for current != nil {
		if aErr, ok := current.(*aerr); ok {
			errMap := make(map[string]any)
			errMap["message"] = aErr.msg

			if aErr.code != "" {
				errMap["code"] = aErr.code
			}

			if len(aErr.fields) > 0 {
				errMap["data"] = aErr.fields
			}

			if !aErr.skipStack && len(aErr.stack) > 0 {
				stackStrs := make([]string, 0)
				frames := runtime.CallersFrames(aErr.stack)
				for {
					frame, more := frames.Next()
					stackStrs = append(stackStrs, fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line))
					if !more {
						break
					}
				}
				errMap["stacktrace"] = stackStrs
			}

			errors = append(errors, errMap)
			current = aErr.cause
		} else {
			// Non-aerr error at the end of the chain
			errors = append(errors, map[string]any{"error": current.Error()})
			break
		}
	}

	// Return as an array of errors
	if len(errors) == 1 {
		// Single error - return it directly as attributes
		attrs := make([]slog.Attr, 0, len(errors[0]))
		for k, v := range errors[0] {
			attrs = append(attrs, slog.Any(k, v))
		}
		return slog.GroupValue(attrs...)
	}

	// Multiple errors - return as array
	return slog.AnyValue(map[string]any{"errors": errors})
}

// Code starts building an error with an error code.
func Code(code string) *aerr {
	return &aerr{
		code:   code,
		fields: make(map[string]any),
	}
}

// Message starts building an error with a message.
func Message(msg string) *aerr {
	return &aerr{
		msg:    msg,
		fields: make(map[string]any),
	}
}

// Code sets the error code.
func (b *aerr) Code(code string) *aerr {
	b.code = code
	return b
}

// Message sets the error message.
func (b *aerr) Message(msg string) *aerr {
	b.msg = msg
	return b
}

// With adds a key-value field to the error.
func (b *aerr) With(key string, value any) *aerr {
	if b.fields == nil {
		b.fields = make(map[string]any)
	}
	b.fields[key] = value
	return b
}

// StackTrace enables stack trace capture.
func (b *aerr) StackTrace() *aerr {
	b.skipStack = false
	return b
}

// WithoutStack disables stack trace capture.
func (b *aerr) WithoutStack() *aerr {
	b.skipStack = true
	return b
}

// Wrap wraps another aerr error, preserving its stack trace and chain.
// This allows building error chains while maintaining all context.
func (b *aerr) Wrap(err error) error {
	if err == nil {
		return nil
	}

	e := &aerr{
		msg:       b.msg,
		code:      b.code,
		cause:     err,
		skipStack: b.skipStack,
		fields:    b.fields,
	}

	// If wrapping a aerr error, preserve its stack instead of creating a new one
	if aErr, ok := err.(*aerr); ok && len(aErr.stack) > 0 {
		e.stack = aErr.stack
		e.skipStack = true // Don't capture a new stack since we're reusing the original
	} else if !b.skipStack {
		e.stack = captureStack()
	}

	return e
}

// Err finalizes the builder and returns the error.
func (b *aerr) Err(cause error) error {
	e := &aerr{
		msg:       b.msg,
		code:      b.code,
		cause:     cause,
		skipStack: b.skipStack,
		fields:    b.fields,
	}

	if !b.skipStack {
		e.stack = captureStack()
	}

	return e
}

// captureStack captures the current stack trace.
func captureStack() []uintptr {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(3, pcs[:]) // Skip captureStack, New/Wrap/Err, and runtime.Callers
	stack := make([]uintptr, n)
	copy(stack, pcs[:n])
	return stack
}
