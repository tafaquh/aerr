package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

// badErr is a value-receiver error whose Error() dereferences a nil
// field. Stored in an error interface its reflect.Kind is Struct, so it
// slips past nil-interface and pointer-nil guards; only a recover keeps
// it from crashing the logger.
type badErr struct{ p *int }

func (b badErr) Error() string { return fmt.Sprintf("v=%d", *b.p) }

// parseLine unmarshals a single zerolog JSON line, failing the test if it
// is not valid JSON.
func parseLine(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal(b, &event); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, b)
	}
	return event
}

// TestObjectTypedNilAerrNoPanic renders a typed-nil *aerr.Error through
// Object: the message key must be present as "<nil>" and no panic occurs.
func TestObjectTypedNilAerrNoPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	var typedNil *aerr.Error
	var err error = typedNil

	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("x")

	event := parseLine(t, buf.Bytes())
	obj, ok := event["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got:\n%s", buf.String())
	}
	if got := obj["message"]; got != "<nil>" {
		t.Errorf("expected message %q, got %q\n%s", "<nil>", got, buf.String())
	}
}

// TestObjectBadErrNoPanic renders a panicking value-receiver error through
// Object: the message must be a "<panic: ...>" placeholder, not a crash.
func TestObjectBadErrNoPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Error().Object("err", aerrzerolog.Object(badErr{})).Msg("x")

	event := parseLine(t, buf.Bytes())
	obj, ok := event["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got:\n%s", buf.String())
	}
	msg, _ := obj["message"].(string)
	if !strings.Contains(msg, "panic") {
		t.Errorf("expected panic placeholder in message, got %q\n%s", msg, buf.String())
	}
}

// TestAttrTypedNilAerrNoPanic passes a typed-nil *aerr.Error as an
// attribute value: the key must remain present (never silently omitted)
// with a "<nil>" value.
func TestAttrTypedNilAerrNoPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	var typedNil *aerr.Error
	err := aerr.Code("C").Message("m").With("bad", typedNil).Err(nil)

	logger.Error().Err(err).Msg("x")

	event := parseLine(t, buf.Bytes())
	errObj, ok := event["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error object, got:\n%s", buf.String())
	}
	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected attributes object, got:\n%s", buf.String())
	}
	got, present := attrs["bad"]
	if !present {
		t.Fatalf("attribute key %q silently omitted:\n%s", "bad", buf.String())
	}
	if got != "<nil>" {
		t.Errorf("expected attribute %q, got %q\n%s", "<nil>", got, buf.String())
	}
}

// TestAttrBadErrNoPanic passes a panicking value-receiver error as an
// attribute value: the key must be present with a "<panic: ...>" value.
func TestAttrBadErrNoPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("C").Message("m").With("bad", badErr{}).Err(nil)

	logger.Error().Err(err).Msg("x")

	event := parseLine(t, buf.Bytes())
	errObj, ok := event["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error object, got:\n%s", buf.String())
	}
	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected attributes object, got:\n%s", buf.String())
	}
	got, present := attrs["bad"]
	if !present {
		t.Fatalf("attribute key %q silently omitted:\n%s", "bad", buf.String())
	}
	msg, _ := got.(string)
	if !strings.Contains(msg, "panic") {
		t.Errorf("expected panic placeholder in attribute, got %q\n%s", msg, buf.String())
	}
}
