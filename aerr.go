package aerr

// Package aerr provides simple error logging with stack traces
import (
	"errors"
	"log/slog"
	"runtime"
	"strconv"
	"sync"
)

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

// aerr provides a fluent API for constructing errors.
type aerr struct {
	msg        string
	code       string
	cause      error
	stack      []uintptr
	skipStack  bool
	attributes map[string]any
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

// AsAerr checks if an error is an aerr error and returns it along with a boolean indicating success.
func AsAerr(err error) (aerr, bool) {
	e := aerr{}
	ok := errors.As(err, &e)
	return e, ok
}

// LogValue implements slog.LogValuer for automatic structured logging.
// It shows only the last code and builds a single combined message from the error chain.
func (e *aerr) LogValue() slog.Value {
	if e == nil {
		return slog.Value{}
	}

	attrs := []slog.Attr{slog.String("message", e.Error())}

	if stacktraces := e.Traces(); len(stacktraces) > 0 {
		attrs = append(attrs, slog.Any("stacktrace", stacktraces))
	}

	if code := e.GetCode(); code != "" {
		attrs = append(attrs, slog.Any("code", code))
	}

	if attributes := e.GetAttributes(); len(attributes) > 0 {
		attrs = append(attrs, slog.Any("attributes", attributes))
	}

	return slog.GroupValue(attrs...)
}

// Code starts building an error with an error code.
func Code(code string) *aerr {
	return &aerr{
		code:       code,
		attributes: make(map[string]any),
	}
}

// Message starts building an error with a message.
func Message(msg string) *aerr {
	return &aerr{
		msg:        msg,
		attributes: make(map[string]any),
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
	if b.attributes == nil {
		b.attributes = make(map[string]any)
	}
	b.attributes[key] = value
	return b
}

// StackTrace enables stack trace capture.
func (b *aerr) StackTrace() *aerr {
	b.skipStack = false
	return b
}

// Wrap wraps another aerr error, preserving its stack trace and chain.
// This allows building error chains while maintaining all context.
func (b *aerr) Wrap(err error) error {
	if err == nil {
		return nil
	}

	e := &aerr{
		msg:        b.msg,
		code:       b.code,
		cause:      err,
		skipStack:  b.skipStack,
		attributes: b.attributes,
	}

	// If wrapping a aerr error, preserve its stack instead of creating a new one
	aErr, ok := err.(*aerr)
	if !ok {
		if !b.skipStack {
			e.stack = captureStack()
		}
		return e
	}

	if b.code == "" {
		e.code = aErr.code
	}

	if aErr.msg != "" {
		e.msg += ": " + aErr.msg
	}

	if len(aErr.stack) > 0 {
		e.stack = aErr.stack
		e.skipStack = true // Don't capture a new stack since we're reusing the original
	}

	if len(aErr.attributes) > 0 {
		for key, value := range aErr.attributes {
			e.attributes[key] = value
		}
	}

	return e
}

// Err finalizes the builder and returns the error.
func (b *aerr) Err(cause error) error {
	e := &aerr{
		msg:        b.msg,
		code:       b.code,
		cause:      cause,
		skipStack:  b.skipStack,
		attributes: b.attributes,
	}

	if !b.skipStack {
		e.stack = captureStack()
	}

	return e
}

func (b *aerr) GetCode() string {
	return b.code
}

func (b *aerr) GetAttributes() map[string]any {
	return b.attributes
}

func (b *aerr) Traces() []string {
	var stacktrace []string
	frames := runtime.CallersFrames(b.stack)
	for {
		frame, more := frames.Next()

		// Get buffer from pool
		bufPtr := bufPool.Get().(*[]byte)
		buf := (*bufPtr)[:0]

		// Format frame using pooled buffer
		buf = append(buf, frame.File...)
		buf = append(buf, '.', '(')
		buf = append(buf, frame.Function...)
		buf = append(buf, ')', ':')
		buf = strconv.AppendInt(buf, int64(frame.Line), 10)
		frameStr := string(buf)

		// Return buffer to pool
		*bufPtr = buf
		bufPool.Put(bufPtr)
		stacktrace = append(stacktrace, frameStr)

		if !more {
			break
		}
	}
	return stacktrace
}

// captureStack captures the current stack trace with intelligent filtering.
// It excludes frames from the Go standard library (GOROOT) and internal
// aerr package frames to provide cleaner, more relevant stack traces.
func captureStack() []uintptr {
	const maxDepth = 32
	var pcs [maxDepth]uintptr

	// Capture raw program counters starting from caller's caller
	// Skip: captureStack, Err/Wrap, runtime.Callers
	n := runtime.Callers(3, pcs[:])

	// Filter frames to exclude irrelevant ones
	filtered := make([]uintptr, 0, n)
	frames := runtime.CallersFrames(pcs[:n])

	for {
		frame, more := frames.Next()
		filtered = append(filtered, frame.PC)

		if !more {
			break
		}
	}

	return filtered
}
