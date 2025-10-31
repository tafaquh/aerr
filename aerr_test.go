package aerr_test

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

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
}

func TestWrapError(t *testing.T) {
	original := errors.New("original error")
	wrapped := aerr.Message("wrapped error").StackTrace().Wrap(original)

	if wrapped == nil {
		t.Fatal("expected error, got nil")
	}

	if wrapped.Error() != "wrapped error" {
		t.Errorf("expected message 'wrapped error', got %q", wrapped.Error())
	}

	// Check unwrapping
	unwrapped := errors.Unwrap(wrapped)
	if unwrapped != original {
		t.Errorf("expected unwrapped error to be original, got %v", unwrapped)
	}
}

func TestWrapNil(t *testing.T) {
	err := aerr.Message("this should be nil").Wrap(nil)
	if err != nil {
		t.Errorf("expected nil when wrapping nil, got %v", err)
	}
}

func TestLog(t *testing.T) {
	// Create a logger that writes to a string builder
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create an error and log it
	err := aerr.Message("test error").StackTrace().Err(nil)
	logger.Error("error occurred", slog.Any("err", err))

	output := buf.String()

	// Check that the log contains our error message
	if !strings.Contains(output, "test error") {
		t.Errorf("expected log to contain 'test error', got:\n%s", output)
	}

	// Check that the log contains stack trace
	if !strings.Contains(output, "stacktrace") {
		t.Errorf("expected log to contain 'stacktrace', got:\n%s", output)
	}
}

func TestLogWithCause(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	original := errors.New("original error")
	wrapped := aerr.Message("wrapped error").StackTrace().Wrap(original)
	logger.Error("error occurred", slog.Any("err", wrapped))

	output := buf.String()

	// Check both errors are in the log
	if !strings.Contains(output, "wrapped error") {
		t.Errorf("expected log to contain 'wrapped error', got:\n%s", output)
	}
	if !strings.Contains(output, "original error") {
		t.Errorf("expected log to contain 'original error', got:\n%s", output)
	}
}

func TestLogNil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	// Should not panic
	logger.Error("test", slog.Any("err", nil))
}

func TestErrorChain(t *testing.T) {
	err1 := errors.New("base error")
	err2 := aerr.Message("wrapped once").Wrap(err1)
	err3 := aerr.Message("wrapped twice").Wrap(err2)

	// Test errors.Is
	if !errors.Is(err3, err1) {
		t.Error("expected errors.Is to work through chain")
	}
}

func TestWrapaErr(t *testing.T) {
	// Create a aerr error
	err1 := aerr.Code("DB_ERROR").
		Message("database connection failed").
		StackTrace().
		With("host", "localhost").
		Err(errors.New("connection refused"))

	// Wrap it with another aerr
	err2 := aerr.Code("APP_ERROR").
		Message("application startup failed").
		With("component", "database").
		Wrap(err1)

	if err2 == nil {
		t.Fatal("expected error, got nil")
	}

	if err2.Error() != "application startup failed" {
		t.Errorf("expected message 'application startup failed', got %q", err2.Error())
	}

	// Check that we can unwrap to get the original error
	unwrapped := errors.Unwrap(err2)
	if unwrapped != err1 {
		t.Error("expected unwrapped error to be err1")
	}

	// Check that errors.Is works through the chain
	if !errors.Is(err2, err1) {
		t.Error("expected errors.Is to work through aerr chain")
	}
}

func TestLogaErrChain(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Create a chain of aerr errors
	err1 := aerr.Code("DB_ERROR").
		Message("query failed").
		With("query", "SELECT * FROM users").
		Err(errors.New("syntax error"))

	err2 := aerr.Code("SERVICE_ERROR").
		Message("user service failed").
		With("method", "GetUser").
		Wrap(err1)

	err3 := aerr.Code("API_ERROR").
		Message("API request failed").
		With("endpoint", "/api/users").
		Wrap(err2)

	logger.Error("request failed", slog.Any("err", err3))

	output := buf.String()

	// Check that all error messages are in the log
	if !strings.Contains(output, "API request failed") {
		t.Errorf("expected log to contain 'API request failed', got:\n%s", output)
	}
	if !strings.Contains(output, "user service failed") {
		t.Errorf("expected log to contain 'user service failed', got:\n%s", output)
	}
	if !strings.Contains(output, "query failed") {
		t.Errorf("expected log to contain 'query failed', got:\n%s", output)
	}

	// Check that all codes are present
	if !strings.Contains(output, "API_ERROR") {
		t.Errorf("expected log to contain 'API_ERROR', got:\n%s", output)
	}
	if !strings.Contains(output, "SERVICE_ERROR") {
		t.Errorf("expected log to contain 'SERVICE_ERROR', got:\n%s", output)
	}
	if !strings.Contains(output, "DB_ERROR") {
		t.Errorf("expected log to contain 'DB_ERROR', got:\n%s", output)
	}

	// Check that context fields are present
	if !strings.Contains(output, "endpoint") {
		t.Errorf("expected log to contain 'endpoint', got:\n%s", output)
	}
	if !strings.Contains(output, "method") {
		t.Errorf("expected log to contain 'method', got:\n%s", output)
	}
	if !strings.Contains(output, "query") {
		t.Errorf("expected log to contain 'query', got:\n%s", output)
	}

	// Check that errors are flattened into an array
	if !strings.Contains(output, `"errors":[`) {
		t.Errorf("expected log to contain errors array, got:\n%s", output)
	}
}

func TestWithFields(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	err := aerr.Code("TEST_ERROR").
		Message("test with fields").
		With("field1", "value1").
		With("field2", 123).
		Err(nil)

	logger.Error("test", slog.Any("err", err))

	output := buf.String()

	if !strings.Contains(output, "field1") {
		t.Errorf("expected log to contain 'field1', got:\n%s", output)
	}
	if !strings.Contains(output, "value1") {
		t.Errorf("expected log to contain 'value1', got:\n%s", output)
	}
	if !strings.Contains(output, "field2") {
		t.Errorf("expected log to contain 'field2', got:\n%s", output)
	}
}

func TestWithoutStack(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	err := aerr.Code("TEST_ERROR").
		Message("test without stack").
		WithoutStack().
		Err(nil)

	logger.Error("test", slog.Any("err", err))

	output := buf.String()

	if strings.Contains(output, "stacktrace") {
		t.Errorf("expected log to NOT contain 'stacktrace', got:\n%s", output)
	}
}
