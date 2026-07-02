package aerr_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// --- fmt.Formatter ---

func TestFormatVerbs(t *testing.T) {
	err := aerr.Code("F_CODE").Message("format me").With("k", "v").Err(nil)

	if got := fmt.Sprintf("%s", err); got != "format me" {
		t.Errorf("%%s = %q, want %q", got, "format me")
	}
	if got := fmt.Sprintf("%v", err); got != "format me" {
		t.Errorf("%%v = %q, want %q", got, "format me")
	}
	if got := fmt.Sprintf("%q", err); got != `"format me"` {
		t.Errorf("%%q = %q, want %q", got, `"format me"`)
	}
}

func TestFormatPlusVIncludesDetail(t *testing.T) {
	err := aerr.Code("F_CODE").
		Message("detailed failure").
		StackTrace().
		With("user_id", "42").
		Err(errors.New("root cause"))

	out := fmt.Sprintf("%+v", err)

	for _, want := range []string{
		"detailed failure: root cause",
		"code: F_CODE",
		"user_id=42",
		"stacktrace:",
		"TestFormatPlusVIncludesDetail",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("%%+v output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatNilAndUnknownVerb(t *testing.T) {
	var e *aerr.Error
	if got := fmt.Sprintf("%+v", e); got != "<nil>" {
		t.Errorf("%%+v of nil = %q, want <nil>", got)
	}
	err := aerr.ErrMsg("x")
	if got := fmt.Sprintf("%d", err); !strings.Contains(got, "%!d") {
		t.Errorf("unknown verb should degrade to %%!d notice, got %q", got)
	}
}

// --- json.Marshaler ---

func TestMarshalJSON(t *testing.T) {
	err := aerr.Code("J_CODE").
		Message("json me").
		StackTrace().
		With("str", "v").
		With("num", 7).
		With("cause", errors.New("attr error")).
		Err(nil)

	e, _ := aerr.AsAerr(err)
	raw, jerr := json.Marshal(e)
	if jerr != nil {
		t.Fatalf("MarshalJSON failed: %v", jerr)
	}

	var decoded map[string]any
	if jerr := json.Unmarshal(raw, &decoded); jerr != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", jerr, raw)
	}
	if decoded["code"] != "J_CODE" {
		t.Errorf("code = %v, want J_CODE", decoded["code"])
	}
	if decoded["message"] != "json me" {
		t.Errorf("message = %v, want 'json me'", decoded["message"])
	}
	attrs, ok := decoded["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes missing or wrong shape: %s", raw)
	}
	if attrs["str"] != "v" || attrs["num"] != float64(7) {
		t.Errorf("attribute values wrong: %v", attrs)
	}
	if attrs["cause"] != "attr error" {
		t.Errorf("error attr should marshal as its message, got %v", attrs["cause"])
	}
	if traces, ok := decoded["stacktrace"].([]any); !ok || len(traces) == 0 {
		t.Errorf("stacktrace missing: %s", raw)
	}
}

func TestMarshalJSONEdgeCases(t *testing.T) {
	var e *aerr.Error
	raw, err := json.Marshal(e)
	if err != nil || string(raw) != "null" {
		t.Errorf("nil *Error = %s (%v), want null", raw, err)
	}

	// Unmarshalable attr value degrades instead of failing.
	ch := make(chan int)
	werr := aerr.Message("m").With("bad", ch).Err(nil)
	ae, _ := aerr.AsAerr(werr)
	raw, err = json.Marshal(ae)
	if err != nil {
		t.Fatalf("MarshalJSON must not fail on unmarshalable attr: %v", err)
	}
	if !json.Valid(raw) {
		t.Errorf("invalid JSON produced: %s", raw)
	}

	// Typed-nil error attr must not panic.
	var typedNil *aerr.Error
	werr2 := aerr.Message("m").With("nilerr", typedNil).Err(nil)
	ae2, _ := aerr.AsAerr(werr2)
	raw, err = json.Marshal(ae2)
	if err != nil || !json.Valid(raw) {
		t.Errorf("typed-nil attr: %s (%v)", raw, err)
	}

	// Nested aerr attr marshals structurally (json.Marshaler wins).
	inner := aerr.Code("IN").ErrMsg("inner")
	ie, _ := aerr.AsAerr(inner)
	werr3 := aerr.Message("m").With("nested", ie).Err(nil)
	ae3, _ := aerr.AsAerr(werr3)
	raw, _ = json.Marshal(ae3)
	if !strings.Contains(string(raw), `"code":"IN"`) {
		t.Errorf("nested aerr attr should marshal structurally: %s", raw)
	}
}

// --- HasCode ---

func TestHasCode(t *testing.T) {
	inner := aerr.Code("INNER").ErrMsg("in")
	mid := fmt.Errorf("mid: %w", inner)
	outer := aerr.Code("OUTER").Message("out").Wrap(mid)

	if !aerr.HasCode(outer, "OUTER") {
		t.Error("HasCode must find the outer code")
	}
	if !aerr.HasCode(outer, "INNER") {
		t.Error("HasCode must find inner codes hidden by outer ones")
	}
	if aerr.HasCode(outer, "MISSING") {
		t.Error("HasCode must not match absent codes")
	}
	if aerr.HasCode(nil, "OUTER") {
		t.Error("HasCode(nil, ...) must be false")
	}

	joined := errors.Join(errors.New("a"), aerr.Code("JOINED").ErrMsg("j"))
	if !aerr.HasCode(joined, "JOINED") {
		t.Error("HasCode must search errors.Join trees")
	}
}

// --- printf constructors ---

func TestPrintfConstructors(t *testing.T) {
	if got := aerr.Errorf("failed %d times", 3).Error(); got != "failed 3 times" {
		t.Errorf("Errorf = %q", got)
	}

	base := errors.New("io down")
	w := aerr.Wrapf(base, "attempt %d", 2)
	if w == nil || w.Error() != "attempt 2: io down" {
		t.Errorf("Wrapf = %v", w)
	}
	if !errors.Is(w, base) {
		t.Error("Wrapf must keep the chain")
	}
	if aerr.Wrapf(nil, "x") != nil {
		t.Error("Wrapf(nil) must be nil")
	}

	if got := aerr.Messagef("user %s", "bob").Err(nil).Error(); got != "user bob" {
		t.Errorf("Messagef = %q", got)
	}
	if got := aerr.Code("C").Messagef("n=%d", 9).Err(nil).Error(); got != "n=9" {
		t.Errorf("(*Builder).Messagef = %q", got)
	}
}

// --- structured frames ---

func TestFrames(t *testing.T) {
	err := aerr.Code("FR").StackTrace().ErrMsg("boom")
	e, _ := aerr.AsAerr(err)

	frames := e.Frames()
	if len(frames) == 0 {
		t.Fatal("expected frames")
	}
	first := frames[0]
	if !strings.Contains(first.Function, "TestFrames") {
		t.Errorf("first frame function = %q, want this test", first.Function)
	}
	if !strings.HasSuffix(first.File, "api_test.go") {
		t.Errorf("first frame file = %q, want api_test.go", first.File)
	}
	if first.Line <= 0 {
		t.Errorf("frame line = %d, want > 0", first.Line)
	}
	if len(frames) != len(e.Traces()) {
		t.Errorf("Frames (%d) and Traces (%d) must agree on filtering", len(frames), len(e.Traces()))
	}

	var nilErr *aerr.Error
	if nilErr.Frames() != nil {
		t.Error("nil receiver Frames() must be nil")
	}
	noStack, _ := aerr.AsAerr(aerr.ErrMsg("plain"))
	if noStack.Frames() != nil {
		t.Error("Frames() without capture must be nil")
	}
}
