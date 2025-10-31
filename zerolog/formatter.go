package aerrzerolog

import (
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
)

var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

func init() {
	// Configure zerolog to use aerr's JSON marshaling
	zerolog.ErrorMarshalFunc = AerrMarshalFunc
	zerolog.ErrorStackMarshaler = AerrStackMarshaler
}

// AerrMarshalFunc returns a marshaller function that converts aerr errors
// into their JSON representation for zerolog logging.
//
// This function is automatically set in init() so you can use standard zerolog API:
//
//	logger.Error().Stack().Err(err).Msg("request failed")
func AerrMarshalFunc(err error) interface{} {
	if typedErr, ok := aerr.AsAerr(err); ok {
		return zerologErrorMarshaller{err: typedErr}
	}
	return err
}

// AerrStackMarshaler returns a marshaller function that extracts stack trace
// information from aerr errors for zerolog logging.
//
// This function is automatically set in init() to work with zerolog's Stack() method.
func AerrStackMarshaler(err error) interface{} {
	if aErr, ok := aerr.AsAerr(err); ok {
		if stack := aerr.GetStack(aErr); len(stack) > 0 {
			var stacktrace []string
			seenFrames := make(map[string]struct{})
			frames := runtime.CallersFrames(stack)
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
			return stacktrace
		}
	}
	return nil
}

// AerrMarshaller wraps aerr errors for zerolog logging.
// Returns a type that implements zerolog.LogObjectMarshaler.
//
// Deprecated: Just import this package and use standard zerolog API with .Err()
//
// Example usage:
//
//	logger.Error().Object("err", aerrzerolog.AerrMarshaller(err)).Msg("operation failed")
func AerrMarshaller(err error) interface{} {
	if typedErr, ok := aerr.AsAerr(err); ok {
		return zerologErrorMarshaller{err: typedErr}
	}
	return zerologErrorMarshaller{err: err}
}

// zerologErrorMarshaller implements zerolog's LogObjectMarshaler interface
// to provide structured serialization of aerr errors.
type zerologErrorMarshaller struct {
	err error
}

// MarshalZerologObject implements zerolog.LogObjectMarshaler for high-performance zerolog integration.
func (m zerologErrorMarshaller) MarshalZerologObject(evt *zerolog.Event) {
	if m.err == nil {
		return
	}

	var messages []string
	var lastCode string
	var allAttributes map[string]any
	seenFrames := make(map[string]struct{})
	var stacktrace []string

	// Walk through the error chain
	current := m.err
	for current != nil {
		if aErr, ok := aerr.AsAerr(current); ok {
			// Collect message
			if msg := aerr.GetMessage(aErr); msg != "" {
				messages = append(messages, msg)
			}

			// Keep the first code we encounter
			if lastCode == "" {
				if code := aerr.GetCode(aErr); code != "" {
					lastCode = code
				}
			}

			// Merge attributes from all errors
			if fields := aerr.GetFields(aErr); len(fields) > 0 {
				if allAttributes == nil {
					allAttributes = make(map[string]any, len(fields))
				}
				for k, v := range fields {
					allAttributes[k] = v
				}
			}

			// Collect stacktraces with deduplication
			if stack := aerr.GetStack(aErr); len(stack) > 0 {
				frames := runtime.CallersFrames(stack)
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

			current = aerr.GetCause(aErr)
		} else {
			// Non-aerr error at the end of the chain
			messages = append(messages, current.Error())
			break
		}
	}

	// Add code if present
	if lastCode != "" {
		evt.Str("code", lastCode)
	}

	// Build combined message
	if len(messages) > 0 {
		if len(messages) == 1 {
			evt.Str("message", messages[0])
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
			evt.Str("message", msgBuilder.String())
		}
	}

	// Add attributes if present
	if len(allAttributes) > 0 {
		dict := zerolog.Dict()
		for k, v := range allAttributes {
			switch vTyped := v.(type) {
			case nil:
				// Skip nil values
			case error:
				dict = dict.Str(k, vTyped.Error())
			default:
				dict = dict.Interface(k, vTyped)
			}
		}
		evt.Dict("attributes", dict)
	}

	// Add stacktrace if present
	if len(stacktrace) > 0 {
		evt.Interface("stacktrace", stacktrace)
	}
}
