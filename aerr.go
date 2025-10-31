package aerr

// Package aerr provides simple error logging with stack traces
import (
	"encoding/json"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
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

// AsAerr checks if an error is an aerr error and returns it along with a boolean indicating success.
func AsAerr(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	if aErr, ok := err.(*aerr); ok {
		return aErr, true
	}
	return nil, false
}

// GetMessage returns the message from an aerr error.
// The error must be an *aerr type, otherwise returns empty string.
func GetMessage(err error) string {
	if aErr, ok := err.(*aerr); ok {
		return aErr.msg
	}
	return ""
}

// GetCode returns the code from an aerr error.
// The error must be an *aerr type, otherwise returns empty string.
func GetCode(err error) string {
	if aErr, ok := err.(*aerr); ok {
		return aErr.code
	}
	return ""
}

// GetFields returns the fields/attributes from an aerr error.
// The error must be an *aerr type, otherwise returns nil.
func GetFields(err error) map[string]any {
	if aErr, ok := err.(*aerr); ok {
		return aErr.fields
	}
	return nil
}

// GetStack returns the stack trace from an aerr error.
// The error must be an *aerr type, otherwise returns nil.
func GetStack(err error) []uintptr {
	if aErr, ok := err.(*aerr); ok {
		return aErr.stack
	}
	return nil
}

// GetCause returns the wrapped error from an aerr error.
// The error must be an *aerr type, otherwise returns nil.
func GetCause(err error) error {
	if aErr, ok := err.(*aerr); ok {
		return aErr.cause
	}
	return nil
}

// LogValue implements slog.LogValuer for automatic structured logging.
// It shows only the last code and builds a single combined message from the error chain.
func (e *aerr) LogValue() slog.Value {
	if e == nil {
		return slog.Value{}
	}

	messages := make([]string, 0, 4)
	result := make([]slog.Attr, 0, 4)
	var lastCode string
	var allAttributes map[string]any
	seenFrames := make(map[string]struct{})
	var stacktrace []string

	// Walk through the error chain
	current := error(e)
	for current != nil {
		if aErr, ok := current.(*aerr); ok {
			// Collect message
			if aErr.msg != "" {
				messages = append(messages, aErr.msg)
			}

			// Keep the first code we encounter (the outermost/top-level error code)
			if lastCode == "" && aErr.code != "" {
				lastCode = aErr.code
			}

			// Merge attributes from all errors (lazy init to avoid allocation if no attributes)
			if len(aErr.fields) > 0 {
				if allAttributes == nil {
					allAttributes = make(map[string]any, len(aErr.fields))
				}
				for k, v := range aErr.fields {
					allAttributes[k] = v
				}
			}

			// Collect ALL stacktraces from all errors in the chain with deduplication
			if !aErr.skipStack && len(aErr.stack) > 0 {
				frames := runtime.CallersFrames(aErr.stack)
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

					// Only add if we haven't seen this frame before
					if _, seen := seenFrames[frameStr]; !seen {
						stacktrace = append(stacktrace, frameStr)
						seenFrames[frameStr] = struct{}{}
					}

					if !more {
						break
					}
				}
			}

			current = aErr.cause
		} else {
			// Non-aerr error at the end of the chain
			messages = append(messages, current.Error())
			break
		}
	}

	// Add code if present
	if lastCode != "" {
		result = append(result, slog.String("code", lastCode))
	}

	// Build combined message from all messages in the chain using strings.Builder
	if len(messages) > 0 {
		// Fast path for single message
		if len(messages) == 1 {
			result = append(result, slog.String("message", messages[0]))
		} else {
			// Estimate total length to minimize allocations
			totalLen := 0
			for _, msg := range messages {
				totalLen += len(msg)
			}
			totalLen += (len(messages) - 1) * 2 // Add space for ": " separators

			var msgBuilder strings.Builder
			msgBuilder.Grow(totalLen)
			for i, msg := range messages {
				if i > 0 {
					msgBuilder.WriteString(": ")
				}
				msgBuilder.WriteString(msg)
			}
			result = append(result, slog.String("message", msgBuilder.String()))
		}
	}

	// Add attributes if present
	if len(allAttributes) > 0 {
		result = append(result, slog.Any("attributes", allAttributes))
	}

	// Add stacktrace if present
	if len(stacktrace) > 0 {
		result = append(result, slog.Any("stacktrace", stacktrace))
	}

	return slog.GroupValue(result...)
}

// MarshalJSON implements json.Marshaler for automatic JSON serialization.
// This allows aerr errors to work seamlessly with zerolog and other JSON loggers.
func (e *aerr) MarshalJSON() ([]byte, error) {
	if e == nil {
		return []byte("null"), nil
	}

	var messages []string
	var lastCode string
	var allAttributes map[string]any
	seenFrames := make(map[string]struct{})
	var stacktrace []string

	// Walk through the error chain
	current := error(e)
	for current != nil {
		if aErr, ok := current.(*aerr); ok {
			// Collect message
			if aErr.msg != "" {
				messages = append(messages, aErr.msg)
			}

			// Keep the first code we encounter
			if lastCode == "" && aErr.code != "" {
				lastCode = aErr.code
			}

			// Merge attributes from all errors
			if len(aErr.fields) > 0 {
				if allAttributes == nil {
					allAttributes = make(map[string]any, len(aErr.fields))
				}
				for k, v := range aErr.fields {
					allAttributes[k] = v
				}
			}

			// Collect stacktraces with deduplication
			if !aErr.skipStack && len(aErr.stack) > 0 {
				frames := runtime.CallersFrames(aErr.stack)
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

					// Only add if we haven't seen this frame before
					if _, seen := seenFrames[frameStr]; !seen {
						stacktrace = append(stacktrace, frameStr)
						seenFrames[frameStr] = struct{}{}
					}

					if !more {
						break
					}
				}
			}

			current = aErr.cause
		} else {
			// Non-aerr error at the end of the chain
			messages = append(messages, current.Error())
			break
		}
	}

	// Build the result map
	result := make(map[string]any)

	// Add code if present
	if lastCode != "" {
		result["code"] = lastCode
	}

	// Build combined message
	if len(messages) > 0 {
		if len(messages) == 1 {
			result["message"] = messages[0]
		} else {
			// Estimate total length
			totalLen := 0
			for _, msg := range messages {
				totalLen += len(msg)
			}
			totalLen += (len(messages) - 1) * 2

			var msgBuilder strings.Builder
			msgBuilder.Grow(totalLen)
			for i, msg := range messages {
				if i > 0 {
					msgBuilder.WriteString(": ")
				}
				msgBuilder.WriteString(msg)
			}
			result["message"] = msgBuilder.String()
		}
	}

	// Add attributes if present
	if len(allAttributes) > 0 {
		result["attributes"] = allAttributes
	}

	// Add stacktrace if present
	if len(stacktrace) > 0 {
		result["stacktrace"] = stacktrace
	}

	return json.Marshal(result)
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

	goroot := runtime.GOROOT()

	for {
		frame, more := frames.Next()

		// Skip frames from Go standard library if GOROOT is set
		isGoStd := goroot != "" && strings.HasPrefix(frame.File, goroot)

		// Include frame if it's not from standard library
		if !isGoStd {
			filtered = append(filtered, frame.PC)
		}

		if !more {
			break
		}
	}

	return filtered
}
