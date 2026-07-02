package aerrzap_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tafaquh/aerr"
	aerrzap "github.com/tafaquh/aerr/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// newJSONLogger returns a zap logger writing one JSON line per entry into
// the returned buffer.
func newJSONLogger() (*zap.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(buf), zapcore.ErrorLevel)
	return zap.New(core), buf
}

// decodeLine parses the single JSON log line in buf.
func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, buf.String())
	}
	return line
}

func TestFieldRendersStructuredError(t *testing.T) {
	logger, buf := newJSONLogger()

	type payload struct {
		Kind string `json:"kind"`
	}
	err := aerr.Code("TEST_ERROR").
		Message("test error").
		StackTrace().
		With("user_id", "12345").
		With("attempt", 3).
		With("retryable", true).
		With("cause", errors.New("connection timeout")).
		With("payload", payload{Kind: "request"}).
		Err(nil)

	logger.Error("request failed", aerrzap.Field(err))

	line := decodeLine(t, buf)
	errObj, ok := line["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested error object, got:\n%s", buf.String())
	}
	if got := errObj["code"]; got != "TEST_ERROR" {
		t.Errorf("error.code = %v, want TEST_ERROR", got)
	}
	if got := errObj["message"]; got != "test error" {
		t.Errorf("error.message = %v, want %q", got, "test error")
	}

	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested error.attributes object, got:\n%s", buf.String())
	}
	if got := attrs["user_id"]; got != "12345" {
		t.Errorf("error.attributes.user_id = %v, want %q", got, "12345")
	}
	if got := attrs["attempt"]; got != float64(3) {
		t.Errorf("error.attributes.attempt = %v, want 3", got)
	}
	if got := attrs["retryable"]; got != true {
		t.Errorf("error.attributes.retryable = %v, want true", got)
	}
	if got := attrs["cause"]; got != "connection timeout" {
		t.Errorf("error.attributes.cause = %v, want %q", got, "connection timeout")
	}
	reflected, ok := attrs["payload"].(map[string]any)
	if !ok || reflected["kind"] != "request" {
		t.Errorf("error.attributes.payload = %v, want {kind: request}", attrs["payload"])
	}

	traces, ok := errObj["stacktrace"].([]any)
	if !ok || len(traces) == 0 {
		t.Errorf("expected non-empty error.stacktrace array, got:\n%s", buf.String())
	}
}

func TestFieldOmitsEmptySections(t *testing.T) {
	logger, buf := newJSONLogger()

	// No code, no attributes, no stack: only the message survives.
	logger.Error("x", aerrzap.Field(aerr.ErrMsg("boom")))

	line := decodeLine(t, buf)
	errObj, ok := line["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested error object, got:\n%s", buf.String())
	}
	if got := errObj["message"]; got != "boom" {
		t.Errorf("error.message = %v, want %q", got, "boom")
	}
	for _, key := range []string{"code", "attributes", "stacktrace"} {
		if _, present := errObj[key]; present {
			t.Errorf("error.%s should be omitted when empty:\n%s", key, buf.String())
		}
	}
}

func TestFieldNilIsSkip(t *testing.T) {
	if got := aerrzap.Field(nil); !got.Equals(zap.Skip()) {
		t.Errorf("Field(nil) = %+v, want zap.Skip()", got)
	}
}

func TestFieldPlainErrorFallsBack(t *testing.T) {
	plain := errors.New("ordinary failure")
	if got := aerrzap.Field(plain); !got.Equals(zap.Error(plain)) {
		t.Errorf("Field(plain) = %+v, want zap.Error(plain)", got)
	}

	logger, buf := newJSONLogger()
	logger.Error("x", aerrzap.Field(plain))

	line := decodeLine(t, buf)
	if got := line["error"]; got != "ordinary failure" {
		t.Errorf("error = %v, want plain message under key %q", got, "error")
	}
}

func TestObjectPlainError(t *testing.T) {
	logger, buf := newJSONLogger()

	logger.Error("x", zap.Object("err", aerrzap.Object(errors.New("plain"))))

	line := decodeLine(t, buf)
	errObj, ok := line["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested err object, got:\n%s", buf.String())
	}
	if got := errObj["message"]; got != "plain" {
		t.Errorf("err.message = %v, want %q", got, "plain")
	}
}

func TestObjectCustomKey(t *testing.T) {
	logger, buf := newJSONLogger()

	err := aerr.Code("OBJ").Message("via object").With("k", "v").Err(nil)
	logger.Error("x", zap.Object("err", aerrzap.Object(err)))

	line := decodeLine(t, buf)
	errObj, ok := line["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested err object, got:\n%s", buf.String())
	}
	if got := errObj["code"]; got != "OBJ" {
		t.Errorf("err.code = %v, want OBJ", got)
	}
	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok || attrs["k"] != "v" {
		t.Errorf("err.attributes = %v, want {k: v}", errObj["attributes"])
	}
}

func TestTypedNilAerrDoesNotPanic(t *testing.T) {
	var typedNil *aerr.Error

	logger, buf := newJSONLogger()
	logger.Error("x", aerrzap.Field(typedNil))
	decodeLine(t, buf)

	buf.Reset()
	logger.Error("x", zap.Object("err", aerrzap.Object(typedNil)))
	decodeLine(t, buf)
}

func TestFieldTypedNilAttrError(t *testing.T) {
	logger, buf := newJSONLogger()

	err := aerr.Code("NIL_ATTR").With("cause", (*aerr.Error)(nil)).Err(nil)
	logger.Error("x", aerrzap.Field(err))

	line := decodeLine(t, buf)
	errObj := line["error"].(map[string]any)
	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested error.attributes object, got:\n%s", buf.String())
	}
	if got := attrs["cause"]; got != "<nil>" {
		t.Errorf("error.attributes.cause = %v, want %q", got, "<nil>")
	}
}
