package aerrzerolog_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

func TestZerologIntegration(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		StackTrace().
		Err(nil)

	logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err)).Msg("test")

	output := buf.String()

	// Check that the log contains expected fields
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

	logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err)).Msg("test")

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

	logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err3)).Msg("request failed")

	output := buf.String()

	// Check that all error messages are in the combined message
	if !strings.Contains(output, "API request failed") {
		t.Errorf("expected log to contain 'API request failed', got:\n%s", output)
	}
	if !strings.Contains(output, "user service failed") {
		t.Errorf("expected log to contain 'user service failed', got:\n%s", output)
	}
	if !strings.Contains(output, "query failed") {
		t.Errorf("expected log to contain 'query failed', got:\n%s", output)
	}

	// Check that only the top-level code is present
	if !strings.Contains(output, "API_ERROR") {
		t.Errorf("expected log to contain 'API_ERROR', got:\n%s", output)
	}
}

func TestZerologStandardAPI(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test with standard API").
		StackTrace().
		With("user_id", "123").
		Err(nil)

	// Standard zerolog API works!
	logger.Error().Stack().Err(err).Msg("operation failed")

	output := buf.String()

	// Check that the log contains expected fields
	if !strings.Contains(output, "TEST_ERROR") {
		t.Errorf("expected log to contain 'TEST_ERROR', got:\n%s", output)
	}
	if !strings.Contains(output, "test with standard API") {
		t.Errorf("expected log to contain 'test with standard API', got:\n%s", output)
	}
	if !strings.Contains(output, "user_id") {
		t.Errorf("expected log to contain 'user_id', got:\n%s", output)
	}
	if !strings.Contains(output, "123") {
		t.Errorf("expected log to contain '123', got:\n%s", output)
	}
	if !strings.Contains(output, "stacktrace") || !strings.Contains(output, "stack") {
		t.Errorf("expected log to contain 'stacktrace' or 'stack', got:\n%s", output)
	}
}
