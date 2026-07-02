package aerr_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// logJSON captures a single slog JSON log line emitted by fn and returns it
// parsed, with the non-deterministic "time" field removed so callers can
// compare canonical structures.
func logJSON(t *testing.T, fn func(*slog.Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	fn(logger)
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, buf.String())
	}
	delete(line, "time")
	return line
}

// errField extracts the structured "err" group object from a parsed line.
func errField(t *testing.T, line map[string]any) map[string]any {
	t.Helper()
	obj, ok := line["err"].(map[string]any)
	if !ok {
		t.Fatalf(`log line has no structured "err" object: %v`, line)
	}
	return obj
}

func TestBasicError(t *testing.T) {
	err := aerr.Message("something went wrong").StackTrace().Err(nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "something went wrong" {
		t.Errorf("expected message 'something went wrong', got %q", err.Error())
	}
}

func TestErrorWithCode(t *testing.T) {
	err := aerr.Code("TEST_ERROR").
		Message("test error occurred").
		StackTrace().
		Err(nil)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "test error occurred" {
		t.Errorf("expected message 'test error occurred', got %q", err.Error())
	}

	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected *aerr.Error in chain")
	}
	if e.Code() != "TEST_ERROR" {
		t.Errorf("Code() = %q, want %q", e.Code(), "TEST_ERROR")
	}
}

func TestWrapError(t *testing.T) {
	original := errors.New("original error")
	wrapped := aerr.Message("wrapped error").StackTrace().Wrap(original)

	if wrapped == nil {
		t.Fatal("expected error, got nil")
	}
	if wrapped.Error() != "wrapped error: original error" {
		t.Errorf("expected message 'wrapped error: original error', got %q", wrapped.Error())
	}

	if unwrapped := errors.Unwrap(wrapped); unwrapped != original {
		t.Errorf("expected unwrapped error to be original, got %v", unwrapped)
	}
	if !errors.Is(wrapped, original) {
		t.Error("errors.Is must reach the wrapped cause")
	}
}

func TestWrapNil(t *testing.T) {
	if err := aerr.Message("this should be nil").Wrap(nil); err != nil {
		t.Errorf("expected nil when wrapping nil, got %v", err)
	}
}

func TestLog(t *testing.T) {
	err := aerr.Message("test error").StackTrace().Err(nil)
	line := logJSON(t, func(l *slog.Logger) {
		l.Error("error occurred", slog.Any("err", err))
	})

	if line["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", line["level"])
	}
	if line["msg"] != "error occurred" {
		t.Errorf("msg = %v, want 'error occurred'", line["msg"])
	}

	eo := errField(t, line)
	if eo["message"] != "test error" {
		t.Errorf("err.message = %v, want 'test error'", eo["message"])
	}
	traces, ok := eo["stacktrace"].([]any)
	if !ok || len(traces) == 0 {
		t.Fatalf("err.stacktrace missing or empty: %v", eo)
	}
	first, _ := traces[0].(string)
	if !strings.Contains(first, "TestLog") {
		t.Errorf("first frame = %q, want it to point at this test", first)
	}
}

func TestLogWithCause(t *testing.T) {
	original := errors.New("original error")
	wrapped := aerr.Message("wrapped error").StackTrace().Wrap(original)
	line := logJSON(t, func(l *slog.Logger) {
		l.Error("error occurred", slog.Any("err", wrapped))
	})

	eo := errField(t, line)
	if eo["message"] != "wrapped error: original error" {
		t.Errorf("err.message = %v, want full chain text", eo["message"])
	}
	if _, ok := eo["code"]; ok {
		t.Errorf("err.code must be omitted when unset, got %v", eo["code"])
	}
	if _, ok := eo["attributes"]; ok {
		t.Errorf("err.attributes must be omitted when empty, got %v", eo["attributes"])
	}
}

func TestLogNil(t *testing.T) {
	// Untyped nil must render as a null err field without panicking.
	line := logJSON(t, func(l *slog.Logger) {
		l.Error("test", slog.Any("err", nil))
	})
	if v, ok := line["err"]; !ok || v != nil {
		t.Errorf(`err = %v, want JSON null`, v)
	}

	// Typed-nil *Error resolves through LogValue's nil guard to null too.
	var typedNil *aerr.Error
	line = logJSON(t, func(l *slog.Logger) {
		l.Error("test", slog.Any("err", typedNil))
	})
	if v, ok := line["err"]; !ok || v != nil {
		t.Errorf(`typed-nil err = %v, want JSON null`, v)
	}
}

func TestErrorChain(t *testing.T) {
	err1 := errors.New("base error")
	err2 := aerr.Message("wrapped once").Wrap(err1)
	err3 := aerr.Message("wrapped twice").Wrap(err2)

	if !errors.Is(err3, err1) {
		t.Error("expected errors.Is to reach the base error through the chain")
	}
	if !errors.Is(err3, err2) {
		t.Error("expected errors.Is to reach the intermediate layer")
	}

	var ae *aerr.Error
	if !errors.As(err3, &ae) {
		t.Fatal("expected errors.As to find an *aerr.Error")
	}
	if ae.Error() != "wrapped twice: wrapped once: base error" {
		t.Errorf("outermost message = %q, want full chain text", ae.Error())
	}
}

func TestWrapAerr(t *testing.T) {
	err1 := aerr.Code("DB_ERROR").
		Message("database connection failed").
		StackTrace().
		With("host", "localhost").
		Err(errors.New("connection refused"))

	err2 := aerr.Code("APP_ERROR").
		Message("application startup failed").
		With("component", "database").
		Wrap(err1)

	if err2 == nil {
		t.Fatal("expected error, got nil")
	}

	expectedMsg := "application startup failed: database connection failed: connection refused"
	if err2.Error() != expectedMsg {
		t.Errorf("expected message %q, got %q", expectedMsg, err2.Error())
	}

	if unwrapped := errors.Unwrap(err2); unwrapped != err1 {
		t.Error("expected unwrapped error to be err1")
	}
	if !errors.Is(err2, err1) {
		t.Error("expected errors.Is to work through aerr chain")
	}

	e2, ok := aerr.AsAerr(err2)
	if !ok {
		t.Fatal("expected *aerr.Error")
	}
	if e2.Code() != "APP_ERROR" {
		t.Errorf("Code() = %q, want APP_ERROR (outer wins)", e2.Code())
	}
	wantAttrs := map[string]any{"component": "database", "host": "localhost"}
	if got := e2.Attributes(); !reflect.DeepEqual(got, wantAttrs) {
		t.Errorf("Attributes() = %v, want %v", got, wantAttrs)
	}
}

func TestLogAerrChain(t *testing.T) {
	err1 := aerr.Code("DB_ERROR").
		Message("query failed").
		With("query", "SELECT * FROM users").
		StackTrace().
		Err(errors.New("syntax error"))

	err2 := aerr.Code("SERVICE_ERROR").
		Message("user service failed").
		With("method", "GetUser").
		Wrap(err1)

	err3 := aerr.Code("API_ERROR").
		Message("API request failed").
		With("endpoint", "/api/users").
		Wrap(err2)

	line := logJSON(t, func(l *slog.Logger) {
		l.Error("request failed", slog.Any("err", err3))
	})

	// The stack was captured at err1's Err call site and inherited upward.
	eo := errField(t, line)
	traces, ok := eo["stacktrace"].([]any)
	if !ok || len(traces) == 0 {
		t.Fatalf("err.stacktrace missing or empty: %v", eo)
	}
	if first, _ := traces[0].(string); !strings.Contains(first, "TestLogAerrChain") {
		t.Errorf("first frame = %q, want the deepest capture site (this test)", first)
	}

	// With the non-deterministic stack removed, the rest of the line must
	// match the canonical structure exactly: full chain message, outermost
	// code, and all attributes merged under err.attributes.
	delete(eo, "stacktrace")
	want := map[string]any{
		"level": "ERROR",
		"msg":   "request failed",
		"err": map[string]any{
			"message": "API request failed: user service failed: query failed: syntax error",
			"code":    "API_ERROR",
			"attributes": map[string]any{
				"endpoint": "/api/users",
				"method":   "GetUser",
				"query":    "SELECT * FROM users",
			},
		},
	}
	if !reflect.DeepEqual(line, want) {
		t.Errorf("log line = %#v\nwant %#v", line, want)
	}
}

func TestWithFields(t *testing.T) {
	err := aerr.Code("TEST_ERROR").
		Message("test with fields").
		With("field1", "value1").
		With("field2", 123).
		Err(nil)

	line := logJSON(t, func(l *slog.Logger) {
		l.Error("test", slog.Any("err", err))
	})

	want := map[string]any{
		"level": "ERROR",
		"msg":   "test",
		"err": map[string]any{
			"message": "test with fields",
			"code":    "TEST_ERROR",
			"attributes": map[string]any{
				"field1": "value1",
				"field2": float64(123),
			},
		},
	}
	if !reflect.DeepEqual(line, want) {
		t.Errorf("log line = %#v\nwant %#v", line, want)
	}
}

func TestWithStackTrace(t *testing.T) {
	err := aerr.Code("TEST_ERROR").
		Message("test with stack").
		StackTrace().
		Err(nil)

	line := logJSON(t, func(l *slog.Logger) {
		l.Error("test", slog.Any("err", err))
	})

	eo := errField(t, line)
	traces, ok := eo["stacktrace"].([]any)
	if !ok || len(traces) == 0 {
		t.Fatalf("err.stacktrace missing or empty: %v", eo)
	}
	first, _ := traces[0].(string)
	if !strings.Contains(first, "TestWithStackTrace") {
		t.Errorf("first frame = %q, want this test's call site", first)
	}
	if !strings.Contains(first, "aerr_test.go") {
		t.Errorf("first frame = %q, want it to point at aerr_test.go", first)
	}
}
