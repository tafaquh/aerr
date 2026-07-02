package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

func TestMain(m *testing.M) {
	aerrzerolog.Register()
	os.Exit(m.Run())
}

func TestZerologIntegration(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		StackTrace().
		Err(nil)

	logger.Error().Err(err).Msg("test")

	output := buf.String()

	if !strings.Contains(output, "TEST_ERROR") {
		t.Errorf("expected log to contain 'TEST_ERROR', got:\n%s", output)
	}
	if !strings.Contains(output, "test error") {
		t.Errorf("expected log to contain 'test error', got:\n%s", output)
	}
	if !strings.Contains(output, "stacktrace") {
		t.Errorf("expected log to contain 'stacktrace', got:\n%s", output)
	}
}

func TestZerologWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test with fields").
		With("field1", "value1").
		With("field2", 123).
		StackTrace().
		Err(nil)

	logger.Error().Err(err).Msg("test")

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

func TestZerologErrorChain(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err1 := aerr.Code("DB_ERROR").
		Message("query failed").
		StackTrace().
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

	logger.Error().Err(err3).Msg("request failed")

	output := buf.String()

	if !strings.Contains(output, "API request failed") {
		t.Errorf("expected log to contain 'API request failed', got:\n%s", output)
	}
	if !strings.Contains(output, "user service failed") {
		t.Errorf("expected log to contain 'user service failed', got:\n%s", output)
	}
	if !strings.Contains(output, "query failed") {
		t.Errorf("expected log to contain 'query failed', got:\n%s", output)
	}
	if !strings.Contains(output, "API_ERROR") {
		t.Errorf("expected log to contain 'API_ERROR', got:\n%s", output)
	}
}

// TestZerologStackNotDuplicated asserts that the README-recommended
// .Stack().Err(err) call renders the trace exactly once (inside the error
// object), not additionally as a top-level "stack" field.
func TestZerologStackNotDuplicated(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test with standard API").
		StackTrace().
		With("user_id", "123").
		Err(nil)

	logger.Error().Stack().Err(err).Msg("operation failed")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}

	if _, ok := event["stack"]; ok {
		t.Errorf("stack duplicated as top-level %q field:\n%s", "stack", buf.String())
	}
	errObj, ok := event["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error object, got:\n%s", buf.String())
	}
	traces, ok := errObj["stacktrace"].([]any)
	if !ok || len(traces) == 0 {
		t.Errorf("expected stacktrace inside error object, got:\n%s", buf.String())
	}
}

// TestRegisterChainsPreviousMarshaler asserts that a marshal func active
// before Register keeps handling non-aerr errors.
func TestRegisterChainsPreviousMarshaler(t *testing.T) {
	saved := zerolog.ErrorMarshalFunc
	defer func() { zerolog.ErrorMarshalFunc = saved }()

	zerolog.ErrorMarshalFunc = func(err error) any {
		return "custom:" + err.Error()
	}
	aerrzerolog.Register()

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	// Non-aerr error goes through the previous custom marshaler.
	logger.Error().Err(errors.New("plain")).Msg("x")
	if !strings.Contains(buf.String(), "custom:plain") {
		t.Errorf("previous marshal func not chained for non-aerr error:\n%s", buf.String())
	}

	// aerr error is rendered structurally.
	buf.Reset()
	logger.Error().Err(aerr.Code("C").ErrMsg("boom")).Msg("x")
	if !strings.Contains(buf.String(), `"code":"C"`) {
		t.Errorf("aerr error not rendered structurally:\n%s", buf.String())
	}
}

// TestObjectHelper renders an error without any global registration.
func TestObjectHelper(t *testing.T) {
	saved := zerolog.ErrorMarshalFunc
	defer func() { zerolog.ErrorMarshalFunc = saved }()
	// Simulate a process that never called Register.
	zerolog.ErrorMarshalFunc = func(err error) any { return err }

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("OBJ").Message("via object").With("k", "v").Err(nil)
	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("x")

	out := buf.String()
	if !strings.Contains(out, `"code":"OBJ"`) || !strings.Contains(out, `"k":"v"`) {
		t.Errorf("Object() did not render structured payload:\n%s", out)
	}

	// Non-aerr errors render their message.
	buf.Reset()
	logger.Error().Object("err", aerrzerolog.Object(errors.New("plain"))).Msg("x")
	if !strings.Contains(buf.String(), "plain") {
		t.Errorf("Object() lost plain error message:\n%s", buf.String())
	}
}

func TestZerologNonAerrDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Error().Err(errors.New("ordinary failure")).Msg("x")
	if !strings.Contains(buf.String(), "ordinary failure") {
		t.Errorf("non-aerr error lost its message:\n%s", buf.String())
	}
}
