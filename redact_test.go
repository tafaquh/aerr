package aerr_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// secret is the distinctive plaintext canary the redaction tests scan for:
// if it appears in any rendered output, masking failed.
const secret = "s3cr3t-canary"

// --- JSON ---

// TestRedactMarshalJSON pins the golden JSON: a Redacted attribute value
// serializes as the placeholder, and the plaintext never appears.
func TestRedactMarshalJSON(t *testing.T) {
	err := aerr.Code("AUTH").
		Message("login failed").
		With("password", aerr.Redact(secret)).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	raw, jerr := json.Marshal(e)
	if jerr != nil {
		t.Fatalf("MarshalJSON failed: %v", jerr)
	}
	const want = `{"code":"AUTH","message":"login failed","attributes":{"password":"[REDACTED]"}}`
	if got := string(raw); got != want {
		t.Errorf("MarshalJSON = %s\nwant %s", got, want)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("plaintext leaked into JSON: %s", raw)
	}
}

// TestRedactMarshalJSONNested proves encoding/json honors the wrapper's
// MarshalJSON when it is nested inside another attribute value (a map), the
// case a top-level check would miss.
func TestRedactMarshalJSONNested(t *testing.T) {
	err := aerr.Message("m").
		With("creds", map[string]any{"pw": aerr.Redact(secret)}).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	raw, _ := json.Marshal(e)
	const want = `{"message":"m","attributes":{"creds":{"pw":"[REDACTED]"}}}`
	if got := string(raw); got != want {
		t.Errorf("nested MarshalJSON = %s\nwant %s", got, want)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("plaintext leaked through nested map: %s", raw)
	}
}

// TestRedactMarshalJSONNeverFails checks MarshalJSON's no-error, no-panic
// contract even when wrapping an otherwise unmarshalable value.
func TestRedactMarshalJSONNeverFails(t *testing.T) {
	raw, err := aerr.Redact(make(chan int)).MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON returned error: %v", err)
	}
	if string(raw) != `"[REDACTED]"` {
		t.Errorf("MarshalJSON = %s, want %q", raw, `"[REDACTED]"`)
	}
}

// --- slog ---

// TestRedactSlogJSON confirms a Redacted attribute rides slog.LogValuer
// through the JSON handler, masking the value.
func TestRedactSlogJSON(t *testing.T) {
	err := aerr.Message("auth failed").With("token", aerr.Redact(secret)).Err(nil)
	line := logJSON(t, func(l *slog.Logger) {
		l.Error("boom", slog.Any("err", err))
	})

	attrs, ok := errField(t, line)["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("attributes missing: %v", line)
	}
	if attrs["token"] != aerr.RedactedText {
		t.Errorf("token = %v, want %s", attrs["token"], aerr.RedactedText)
	}
}

// TestRedactSlogText confirms the same masking through the text handler,
// which formats attributes rather than JSON-encoding them.
func TestRedactSlogText(t *testing.T) {
	err := aerr.Message("auth failed").With("token", aerr.Redact(secret)).Err(nil)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger.Error("boom", slog.Any("err", err))

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Errorf("plaintext leaked into text handler output: %s", out)
	}
	if !strings.Contains(out, aerr.RedactedText) {
		t.Errorf("text output missing %s: %s", aerr.RedactedText, out)
	}
}

// --- fmt ---

// TestRedactFormatVerbs is the critical leak-closer table: every verb
// applied to a bare Redacted must emit the placeholder and never the
// plaintext, including %#v and the numeric verbs that bypass fmt.Stringer.
func TestRedactFormatVerbs(t *testing.T) {
	r := aerr.Redact(secret)
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x"} {
		got := fmt.Sprintf(verb, r)
		if strings.Contains(got, secret) {
			t.Errorf("verb %s leaked plaintext: %q", verb, got)
		}
		if !strings.Contains(got, aerr.RedactedText) {
			t.Errorf("verb %s = %q, want it to contain %s", verb, got, aerr.RedactedText)
		}
	}
}

// TestRedactFormatInError renders an *Error carrying a Redacted attribute
// with %+v (the detail path that prints attributes) and asserts the
// wrapper masks the value while the message path stays unaffected.
func TestRedactFormatInError(t *testing.T) {
	err := aerr.Code("C").Message("m").With("password", aerr.Redact(secret)).Err(nil)

	out := fmt.Sprintf("%+v", err)
	if strings.Contains(out, secret) {
		t.Errorf("plaintext leaked in %%+v: %s", out)
	}
	if !strings.Contains(out, "password=[REDACTED]") {
		t.Errorf("%%+v = %q, want it to contain password=[REDACTED]", out)
	}

	// The message path carries no attributes, so it is unchanged.
	if got := fmt.Sprintf("%v", err); got != "m" {
		t.Errorf("%%v = %q, want m", got)
	}
	if err.Error() != "m" {
		t.Errorf("Error() = %q, want m", err.Error())
	}
}

// --- Value round-trip ---

// TestRedactValueRoundTrip proves Value recovers the exact original,
// including nil, a scalar, and a struct, plus the zero-value wrapper.
func TestRedactValueRoundTrip(t *testing.T) {
	type creds struct{ User, Pass string }
	for _, v := range []any{
		"plain",
		42,
		nil,
		creds{User: "u", Pass: "p"},
	} {
		if got := aerr.Redact(v).Value(); !reflect.DeepEqual(got, v) {
			t.Errorf("Value() = %#v, want %#v", got, v)
		}
	}

	// The zero value is valid: nil Value, still masks.
	var zero aerr.Redacted
	if zero.Value() != nil {
		t.Errorf("zero Redacted Value() = %v, want nil", zero.Value())
	}
	if zero.String() != aerr.RedactedText {
		t.Errorf("zero Redacted String() = %q, want %s", zero.String(), aerr.RedactedText)
	}
	if got := fmt.Sprintf("%#v", zero); !strings.Contains(got, aerr.RedactedText) {
		t.Errorf("zero Redacted %%#v = %q, want %s", got, aerr.RedactedText)
	}
}

// TestRedactNoFlatten pins that Redact does not flatten: the inner wrapper
// is preserved and both layers still mask.
func TestRedactNoFlatten(t *testing.T) {
	inner := aerr.Redact(secret)
	outer := aerr.Redact(inner)

	got, ok := outer.Value().(aerr.Redacted)
	if !ok {
		t.Fatalf("Redact(Redact(v)).Value() = %T, want aerr.Redacted (no flattening)", outer.Value())
	}
	if got.Value() != secret {
		t.Errorf("inner Value() = %v, want the original", got.Value())
	}
	if s := fmt.Sprintf("%v", outer); s != aerr.RedactedText {
		t.Errorf("nested Redacted %%v = %q, want %s", s, aerr.RedactedText)
	}
}

// --- builder semantics ---

// TestRedactBuilderOverwriteInPlace confirms With overwrites a Redacted
// value in place with a later plain value, preserving order and count.
func TestRedactBuilderOverwriteInPlace(t *testing.T) {
	err := aerr.Message("m").
		With("k", aerr.Redact(secret)).
		With("k", "plain").
		Err(nil)
	e, _ := aerr.AsAerr(err)

	if got := e.Attributes()["k"]; got != "plain" {
		t.Errorf("overwrite-in-place: k = %#v, want plain", got)
	}
	if e.NumAttrs() != 1 {
		t.Errorf("NumAttrs = %d, want 1 (overwrite, not append)", e.NumAttrs())
	}
}

// TestRedactMergePreservesWrapper checks the outer-wins merge across Wrap
// keeps the wrapper intact, and that Attributes / RangeAttrs return the
// Redacted value so the mask survives programmatic re-logging while Value
// still recovers the original.
func TestRedactMergePreservesWrapper(t *testing.T) {
	inner := aerr.Code("IN").With("password", aerr.Redact(secret)).ErrMsg("inner")
	outer := aerr.Message("outer").Wrap(inner)
	e, _ := aerr.AsAerr(outer)

	v, ok := e.Attributes()["password"].(aerr.Redacted)
	if !ok {
		t.Fatalf("merged attr is %T, want aerr.Redacted (mask survives merge)", e.Attributes()["password"])
	}
	if v.Value() != secret {
		t.Errorf("Value() = %v, want the original recovered", v.Value())
	}

	var seen any
	e.RangeAttrs(func(k string, val any) bool {
		if k == "password" {
			seen = val
		}
		return true
	})
	if _, ok := seen.(aerr.Redacted); !ok {
		t.Errorf("RangeAttrs value = %T, want aerr.Redacted wrapper", seen)
	}
}

// --- leak scan ---

// TestRedactNoLeakAcrossAllPaths renders one error carrying Redacted values
// (direct and nested) through every core render path and asserts the secret
// canary appears in none of them.
func TestRedactNoLeakAcrossAllPaths(t *testing.T) {
	err := aerr.Code("SEC").
		Message("handling secret").
		With("password", aerr.Redact(secret)).
		With("nested", map[string]any{"token": aerr.Redact(secret)}).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	renders := map[string]string{}

	raw, _ := json.Marshal(e)
	renders["MarshalJSON"] = string(raw)
	renders["%v"] = fmt.Sprintf("%v", e)
	renders["%+v"] = fmt.Sprintf("%+v", e)
	renders["%q"] = fmt.Sprintf("%q", e)

	var jbuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&jbuf, nil)).Error("x", slog.Any("err", e))
	renders["slog/json"] = jbuf.String()

	var tbuf bytes.Buffer
	slog.New(slog.NewTextHandler(&tbuf, nil)).Error("x", slog.Any("err", e))
	renders["slog/text"] = tbuf.String()

	for path, out := range renders {
		if strings.Contains(out, secret) {
			t.Errorf("secret leaked via %s:\n%s", path, out)
		}
	}
}

// --- errors.Is/As non-interference ---

// TestRedactDoesNotDisturbErrorChain confirms wrapping attribute values has
// no effect on the error chain itself: Is/As and code inheritance still
// work with Redacted attributes present.
func TestRedactDoesNotDisturbErrorChain(t *testing.T) {
	sentinel := fmt.Errorf("root cause")
	err := aerr.Code("DB").
		Message("query failed").
		With("password", aerr.Redact(secret)).
		Wrap(sentinel)

	if !errors.Is(err, sentinel) {
		t.Error("errors.Is must still reach the cause with a Redacted attr present")
	}
	var ae *aerr.Error
	if !errors.As(err, &ae) || ae.Code() != "DB" {
		t.Errorf("errors.As must still extract the *Error, got code %q", ae.Code())
	}
}
