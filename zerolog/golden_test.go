package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

// withFreshRegister installs a clean aerr marshal func (no chained
// predecessor) for the duration of the test, isolating it from whatever
// global state other tests left behind.
func withFreshRegister(t *testing.T) {
	t.Helper()
	saved := zerolog.ErrorMarshalFunc
	zerolog.ErrorMarshalFunc = nil
	aerrzerolog.Register()
	t.Cleanup(func() { zerolog.ErrorMarshalFunc = saved })
}

// TestZerologAerrObjectGolden pins the exact parsed structure zerolog emits
// for an aerr error through the Register()-installed marshal func. The stack
// trace is intentionally omitted (StackTrace() not called) because rendered
// frames are non-deterministic; the full shape is otherwise fixed.
func TestZerologAerrObjectGolden(t *testing.T) {
	withFreshRegister(t)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("G_CODE").
		Message("golden").
		With("s", "v").
		With("n", 7).
		Wrap(errors.New("cause"))

	logger.Error().Err(err).Msg("boom")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}

	want := map[string]any{
		"level":   "error",
		"message": "boom",
		"error": map[string]any{
			"code":    "G_CODE",
			"message": "golden: cause",
			"attributes": map[string]any{
				"s": "v",
				"n": float64(7),
			},
		},
	}
	if !reflect.DeepEqual(event, want) {
		t.Errorf("zerolog aerr object mismatch:\n got: %#v\nwant: %#v", event, want)
	}
}

// TestZerologObjectHelperGolden pins the same shape produced by the Object
// helper, which needs no global registration.
func TestZerologObjectHelperGolden(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("OBJ").Message("via object").With("k", "v").Err(nil)
	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("boom")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}

	wantErr := map[string]any{
		"code":    "OBJ",
		"message": "via object",
		"attributes": map[string]any{
			"k": "v",
		},
	}
	if !reflect.DeepEqual(event["err"], wantErr) {
		t.Errorf("Object() err field = %#v, want %#v", event["err"], wantErr)
	}
}

// TestZerologNonAerrFallbackParsed asserts, via parsed JSON, that a non-aerr
// error falls through to zerolog's default rendering: a bare message string
// under the error field, never a structured object.
func TestZerologNonAerrFallbackParsed(t *testing.T) {
	withFreshRegister(t)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Error().Err(errors.New("ordinary failure")).Msg("x")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}
	if event["error"] != "ordinary failure" {
		t.Errorf("non-aerr error = %v, want the plain message string", event["error"])
	}
	if _, isObj := event["error"].(map[string]any); isObj {
		t.Errorf("non-aerr error must not render as a structured object: %v", event["error"])
	}
}

// TestZerologObjectNonAerrFallbackParsed does the same for the Object helper,
// which renders a non-aerr error as an object carrying only its message.
func TestZerologObjectNonAerrFallbackParsed(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Error().Object("err", aerrzerolog.Object(errors.New("plain"))).Msg("x")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}
	errObj, ok := event["err"].(map[string]any)
	if !ok {
		t.Fatalf("Object() non-aerr err field = %v, want an object with a message", event["err"])
	}
	want := map[string]any{"message": "plain"}
	if !reflect.DeepEqual(errObj, want) {
		t.Errorf("Object() non-aerr err = %#v, want %#v", errObj, want)
	}
}

// TestZerologAppendAttrTypes drives every typed fast-path of appendAttr plus
// the reflection fallback, asserting each attribute round-trips through the
// parsed JSON.
func TestZerologAppendAttrTypes(t *testing.T) {
	withFreshRegister(t)

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	ts := time.Date(2026, 7, 2, 10, 30, 0, 0, time.UTC)
	err := aerr.Message("m").
		With("str", "s").
		With("int", 7).
		With("int64", int64(8)).
		With("uint64", uint64(9)).
		With("bool", true).
		With("f64", 1.5).
		With("f32", float32(2.5)).
		With("time", ts).
		With("dur", 1500*time.Millisecond).
		With("strs", []string{"a", "b"}).
		With("bytes", []byte("hi")).
		With("err", errors.New("inner")).
		With("other", struct{ X int }{X: 3}).
		Err(nil)

	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("x")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}
	attrs, ok := event["err"].(map[string]any)["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes missing: %s", buf.String())
	}

	checks := map[string]any{
		"str":    "s",
		"int":    float64(7),
		"int64":  float64(8),
		"uint64": float64(9),
		"bool":   true,
		"f64":    1.5,
		"f32":    2.5,
		"time":   ts.Format(zerolog.TimeFieldFormat),
		"strs":   []any{"a", "b"},
		"bytes":  "hi",
		"err":    "inner",
		"other":  map[string]any{"X": float64(3)},
	}
	for k, want := range checks {
		if !reflect.DeepEqual(attrs[k], want) {
			t.Errorf("attr %q = %#v, want %#v", k, attrs[k], want)
		}
	}
	// Duration renders as a number under the default field unit.
	if _, isNum := attrs["dur"].(float64); !isNum {
		t.Errorf("attr dur = %#v, want a JSON number", attrs["dur"])
	}
}

// TestAerrMarshalFunc covers the exported marshal-func helper for callers
// composing their own zerolog.ErrorMarshalFunc.
func TestAerrMarshalFunc(t *testing.T) {
	if _, ok := aerrzerolog.AerrMarshalFunc(aerr.Code("A").ErrMsg("a")).(zerolog.LogObjectMarshaler); !ok {
		t.Error("AerrMarshalFunc(aerr) must return a LogObjectMarshaler")
	}
	plain := errors.New("p")
	if got := aerrzerolog.AerrMarshalFunc(plain); got != plain {
		t.Errorf("AerrMarshalFunc(non-aerr) = %v, want the error unchanged", got)
	}
}

// TestAerrStackMarshaler covers the deprecated top-level stack marshaler in
// both its match and fall-through branches.
func TestAerrStackMarshaler(t *testing.T) {
	withStack := aerr.Code("S").StackTrace().ErrMsg("boom")
	stack, ok := aerrzerolog.AerrStackMarshaler(withStack).([]string)
	if !ok || len(stack) == 0 {
		t.Errorf("AerrStackMarshaler(aerr with stack) = %v, want a non-empty []string", stack)
	}
	if got := aerrzerolog.AerrStackMarshaler(errors.New("p")); got != nil {
		t.Errorf("AerrStackMarshaler(non-aerr) = %v, want nil", got)
	}
	if got := aerrzerolog.AerrStackMarshaler(aerr.ErrMsg("no stack")); got != nil {
		t.Errorf("AerrStackMarshaler(no stack) = %v, want nil", got)
	}
}

// TestZerologObjectNilError exercises plainMarshaller's nil-error guard: a
// nil error must render as an empty object without panicking.
func TestZerologObjectNilError(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Error().Object("err", aerrzerolog.Object(nil)).Msg("x")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}
	errObj, ok := event["err"].(map[string]any)
	if !ok || len(errObj) != 0 {
		t.Errorf("Object(nil) err = %v, want an empty object", event["err"])
	}
}
