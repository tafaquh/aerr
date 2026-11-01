package aerr_test

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// The interfaces the error trackers' SDKs look for.
var (
	_ interface{ StackTrace() []uintptr } = (*aerr.Error)(nil)
	_ interface{ ErrorType() string }     = (*aerr.Error)(nil)
)

// originStack captures a stack in a named function, so a test can tell the
// origin frame from the frame of whoever later reads the trace.
func originStack() error {
	return aerr.Code("DB").StackTrace().ErrMsg("timeout")
}

// TestStackTraceFollowsSentryContract replays how sentry-go extracts a stack
// by reflection: it looks up StackFrames first and stops there if the method
// exists, so aerr must not have one; then StackTrace, called with no
// arguments, must return a slice of uintptr-kind elements that
// runtime.CallersFrames resolves to the origin.
func TestStackTraceFollowsSentryContract(t *testing.T) {
	err := aerr.Message("handler failed").Wrap(originStack())
	v := reflect.ValueOf(err)
	if v.MethodByName("StackFrames").IsValid() {
		t.Fatal("a StackFrames method would shadow StackTrace in sentry-go")
	}
	out := v.MethodByName("StackTrace").Call(nil)[0]
	if out.Kind() != reflect.Slice || out.Type().Elem().Kind() != reflect.Uintptr || out.Len() == 0 {
		t.Fatalf("StackTrace() = %v, want a non-empty slice of uintptr", out)
	}
	frame, _ := runtime.CallersFrames(out.Interface().([]uintptr)).Next()
	if !strings.HasSuffix(frame.Function, ".originStack") {
		t.Errorf("first frame = %s, want the origin, originStack", frame.Function)
	}

	inner, _ := aerr.AsAerr(errors.Unwrap(err))
	outer, _ := aerr.AsAerr(err)
	if &outer.StackTrace()[0] != &inner.StackTrace()[0] {
		t.Error("the wrapping layer should return the inherited origin stack")
	}
	if aerr.Code("X").Err(nil).(*aerr.Error).StackTrace() != nil {
		t.Error("StackTrace() should be nil when no stack was captured")
	}
	if (*aerr.Error)(nil).StackTrace() != nil {
		t.Error("StackTrace() on a nil *Error should be nil")
	}
}

// TestErrorTypeIsTheCode pins the method OpenTelemetry's semconv.ErrorType
// reads: the code, found through wrappers by errors.As, and "" when no code
// is set so the caller falls back to the type name.
func TestErrorTypeIsTheCode(t *testing.T) {
	var et interface{ ErrorType() string }
	if !errors.As(fmt.Errorf("handler: %w", aerr.Code("DB_ERROR").ErrMsg("timeout")), &et) || et.ErrorType() != "DB_ERROR" {
		t.Errorf("ErrorType through a wrapper = %v, want DB_ERROR", et)
	}
	if got := aerr.ErrMsg("no code").(*aerr.Error).ErrorType(); got != "" {
		t.Errorf("ErrorType() without a code = %q, want \"\"", got)
	}
	if got := (*aerr.Error)(nil).ErrorType(); got != "" {
		t.Errorf("ErrorType() on a nil *Error = %q, want \"\"", got)
	}
}
