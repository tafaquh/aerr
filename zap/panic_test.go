package aerrzap_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
	aerrzap "github.com/tafaquh/aerr/zap"
	"go.uber.org/zap"
)

// badErr is a value-receiver error whose Error() dereferences a nil
// field. Stored in an error interface its reflect.Kind is Struct, so it
// slips past nil-interface and pointer-nil guards and panics when Error()
// is called. zapcore recovers such panics only on its ErrorType path, not
// on the ObjectMarshaler paths this adapter uses.
type badErr struct{ p *int }

func (b badErr) Error() string { return fmt.Sprintf("v=%d", *b.p) }

// TestBadErrAttrNoPanic renders a panicking value-receiver error as an
// attribute value: it flows through addAttr -> errMessage and must render
// a "<panic: ...>" placeholder instead of crashing the logger.
func TestBadErrAttrNoPanic(t *testing.T) {
	logger, buf := newJSONLogger()

	err := aerr.Code("C").Message("m").With("bad", badErr{}).Err(nil)
	logger.Error("x", aerrzap.Field(err))

	line := decodeLine(t, buf)
	errObj, ok := line["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested error object, got:\n%s", buf.String())
	}
	attrs, ok := errObj["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested attributes object, got:\n%s", buf.String())
	}
	got, present := attrs["bad"]
	if !present {
		t.Fatalf("panicking attr key silently omitted:\n%s", buf.String())
	}
	msg, _ := got.(string)
	if !strings.Contains(msg, "panic") {
		t.Errorf("expected panic placeholder in attr, got %q\n%s", msg, buf.String())
	}
}

// TestObjectBadErrNoPanic renders a panicking value-receiver error through
// Object (the plainMarshaler path): the message must be a "<panic: ...>"
// placeholder, not a crash.
func TestObjectBadErrNoPanic(t *testing.T) {
	logger, buf := newJSONLogger()

	logger.Error("x", zap.Object("err", aerrzap.Object(badErr{})))

	line := decodeLine(t, buf)
	errObj, ok := line["err"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested err object, got:\n%s", buf.String())
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "panic") {
		t.Errorf("expected panic placeholder in message, got %q\n%s", msg, buf.String())
	}
}

// TestFieldBadErrNoPanic passes a panicking non-aerr error to Field, which
// routes it through zap.Error (whose encoder recovers panics itself). The
// point is only that the process survives and a valid line is produced.
func TestFieldBadErrNoPanic(t *testing.T) {
	logger, buf := newJSONLogger()

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Field(badErr) panicked: %v", r)
			}
		}()
		logger.Error("x", aerrzap.Field(badErr{}))
	}()

	decodeLine(t, buf)
}

// TestAddReflectedFailurePropagates ensures a value the JSON encoder
// cannot reflect (a channel) does not silently vanish: addAttr propagates
// AddReflected's error up through MarshalLogObject so zap emits its
// standard "<key>Error" indicator ("errorError" for the top-level "error"
// key) rather than dropping the attribute without a trace.
func TestAddReflectedFailurePropagates(t *testing.T) {
	logger, buf := newJSONLogger()

	ch := make(chan int)
	err := aerr.Code("C").Message("m").With("ch", ch).Err(nil)
	logger.Error("x", aerrzap.Field(err))

	line := decodeLine(t, buf)
	if _, ok := line["errorError"]; !ok {
		t.Errorf("expected zap error indicator field %q, got:\n%s", "errorError", buf.String())
	}
}
