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

// --- nil-receiver safety for every accessor ---

func TestNilReceiverAccessors(t *testing.T) {
	var e *aerr.Error

	if got := e.Error(); got != "" {
		t.Errorf("nil.Error() = %q, want \"\"", got)
	}
	if got := e.Unwrap(); got != nil {
		t.Errorf("nil.Unwrap() = %v, want nil", got)
	}
	if got := e.Code(); got != "" {
		t.Errorf("nil.Code() = %q, want \"\"", got)
	}
	if got := e.NumAttrs(); got != 0 {
		t.Errorf("nil.NumAttrs() = %d, want 0", got)
	}
	called := false
	e.RangeAttrs(func(string, any) bool {
		called = true
		return true
	})
	if called {
		t.Error("nil.RangeAttrs must not invoke the callback")
	}
	if got := e.Attributes(); got != nil {
		t.Errorf("nil.Attributes() = %v, want nil", got)
	}
	if got := e.Traces(); got != nil {
		t.Errorf("nil.Traces() = %v, want nil", got)
	}
	if got := e.Frames(); got != nil {
		t.Errorf("nil.Frames() = %v, want nil", got)
	}
	if v := e.LogValue(); !v.Equal(slog.Value{}) {
		t.Errorf("nil.LogValue() = %v, want the zero slog.Value", v)
	}
}

// --- RangeAttrs ---

func TestRangeAttrsOrderAndEarlyStop(t *testing.T) {
	err := aerr.Message("m").
		With("a", 1).
		With("b", 2).
		With("c", 3).
		Err(nil)
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected aerr")
	}

	var keys []string
	e.RangeAttrs(func(k string, _ any) bool {
		keys = append(keys, k)
		return true
	})
	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("RangeAttrs order = %v, want %v", keys, want)
	}

	// Returning false after the first item must stop iteration immediately.
	var seen []string
	e.RangeAttrs(func(k string, _ any) bool {
		seen = append(seen, k)
		return false
	})
	if len(seen) != 1 || seen[0] != "a" {
		t.Errorf("early stop visited %v, want exactly [a]", seen)
	}
}

// --- Attributes ---

func TestAttributesNilWhenEmpty(t *testing.T) {
	e, ok := aerr.AsAerr(aerr.ErrMsg("no attrs"))
	if !ok {
		t.Fatal("expected aerr")
	}
	if got := e.Attributes(); got != nil {
		t.Errorf("Attributes() = %v, want nil for attr-less error", got)
	}
	if got := e.NumAttrs(); got != 0 {
		t.Errorf("NumAttrs() = %d, want 0", got)
	}
}

func TestAttributesReturnsIsolatedSnapshot(t *testing.T) {
	err := aerr.Message("m").With("a", 1).With("b", "x").Err(nil)
	e, _ := aerr.AsAerr(err)

	m := e.Attributes()
	m["a"] = 999
	m["intruder"] = true

	fresh := e.Attributes()
	want := map[string]any{"a": 1, "b": "x"}
	if !reflect.DeepEqual(fresh, want) {
		t.Errorf("mutating the returned map leaked into the error: %v, want %v", fresh, want)
	}
	if e.NumAttrs() != 2 {
		t.Errorf("NumAttrs() = %d, want 2 after map mutation", e.NumAttrs())
	}
}

// --- Traces / Frames without capture ---

func TestTracesNilWithoutCapture(t *testing.T) {
	e, _ := aerr.AsAerr(aerr.Message("no stack").Err(nil))
	if got := e.Traces(); got != nil {
		t.Errorf("Traces() = %v, want nil when StackTrace() was never requested", got)
	}
	if got := e.Frames(); got != nil {
		t.Errorf("Frames() = %v, want nil when StackTrace() was never requested", got)
	}
}

// --- LogValue ---

func logValueKeys(v slog.Value) []string {
	attrs := v.Group()
	keys := make([]string, 0, len(attrs))
	for i := 0; i < len(attrs); i++ {
		keys = append(keys, attrs[i].Key)
	}
	return keys
}

func TestLogValueEmptyErrorIsEmptyGroup(t *testing.T) {
	e, _ := aerr.AsAerr(aerr.ErrMsg(""))
	v := e.LogValue()
	if v.Kind() != slog.KindGroup {
		t.Fatalf("LogValue kind = %v, want group", v.Kind())
	}
	if got := v.Group(); len(got) != 0 {
		t.Errorf("empty error LogValue group = %v, want no members", got)
	}
}

func TestLogValueFieldOmission(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "message only",
			err:  aerr.ErrMsg("just message"),
			want: []string{"message"},
		},
		{
			name: "code only",
			err:  aerr.Code("ONLY_CODE").Err(nil),
			want: []string{"code"},
		},
		{
			name: "attributes only",
			err:  aerr.Message("").With("k", "v").Err(nil),
			want: []string{"attributes"},
		},
		{
			name: "stacktrace only",
			err:  aerr.StackTrace().Err(nil),
			want: []string{"stacktrace"},
		},
		{
			name: "all fields in canonical order",
			err: aerr.Code("C").
				Message("m").
				With("k", "v").
				StackTrace().
				Err(nil),
			want: []string{"message", "code", "attributes", "stacktrace"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := aerr.AsAerr(tc.err)
			if !ok {
				t.Fatal("expected aerr")
			}
			got := logValueKeys(e.LogValue())
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("LogValue keys = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLogValueAttributesSubgroup(t *testing.T) {
	err := aerr.Message("m").With("a", "1").With("b", 2).Err(nil)
	e, _ := aerr.AsAerr(err)

	var attrsValue slog.Value
	found := false
	group := e.LogValue().Group()
	for i := 0; i < len(group); i++ {
		if group[i].Key == "attributes" {
			attrsValue = group[i].Value
			found = true
		}
	}
	if !found {
		t.Fatal("attributes group missing from LogValue")
	}
	if attrsValue.Kind() != slog.KindGroup {
		t.Fatalf("attributes kind = %v, want group", attrsValue.Kind())
	}
	sub := attrsValue.Group()
	if len(sub) != 2 || sub[0].Key != "a" || sub[1].Key != "b" {
		t.Errorf("attributes subgroup = %v, want ordered a then b", sub)
	}
	if sub[0].Value.String() != "1" || sub[1].Value.Int64() != 2 {
		t.Errorf("attributes values wrong: %v", sub)
	}
}

// TestLogValueGoldenJSON pins the full serialized shape (and key order) of
// a logged aerr error, with the time field stripped via ReplaceAttr.
func TestLogValueGoldenJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))

	err := aerr.Code("G_CODE").
		Message("golden").
		With("a", "1").
		With("b", 2).
		Err(errors.New("cause"))
	logger.Error("evt", slog.Any("err", err))

	got := strings.TrimSpace(buf.String())
	want := `{"level":"ERROR","msg":"evt","err":{"message":"golden: cause","code":"G_CODE","attributes":{"a":"1","b":2}}}`
	if got != want {
		t.Errorf("golden mismatch:\n got: %s\nwant: %s", got, want)
	}
}

// --- MarshalJSON field combinations (omit-empty and comma placement) ---

func TestMarshalJSONFieldCombos(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "code only",
			err:  aerr.Code("C").Err(nil),
			want: `{"code":"C"}`,
		},
		{
			name: "message only",
			err:  aerr.ErrMsg("m"),
			want: `{"message":"m"}`,
		},
		{
			name: "attributes only",
			err:  aerr.Message("").With("k", "v").Err(nil),
			want: `{"attributes":{"k":"v"}}`,
		},
		{
			name: "empty error",
			err:  aerr.ErrMsg(""),
			want: `{}`,
		},
		{
			name: "code message attributes",
			err:  aerr.Code("C").Message("m").With("k", 1).Err(nil),
			want: `{"code":"C","message":"m","attributes":{"k":1}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, ok := aerr.AsAerr(tc.err)
			if !ok {
				t.Fatal("expected aerr")
			}
			raw, err := json.Marshal(e)
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}
			if string(raw) != tc.want {
				t.Errorf("MarshalJSON = %s, want %s", raw, tc.want)
			}
		})
	}
}

func TestMarshalJSONStackOnly(t *testing.T) {
	err := aerr.StackTrace().Err(nil)
	e, _ := aerr.AsAerr(err)
	raw, jerr := json.Marshal(e)
	if jerr != nil {
		t.Fatalf("MarshalJSON failed: %v", jerr)
	}
	var decoded map[string]any
	if jerr := json.Unmarshal(raw, &decoded); jerr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jerr, raw)
	}
	if len(decoded) != 1 {
		t.Errorf("expected only the stacktrace key, got %v", decoded)
	}
	if traces, ok := decoded["stacktrace"].([]any); !ok || len(traces) == 0 {
		t.Errorf("stacktrace missing or empty: %s", raw)
	}
}

// TestMarshalJSONValueReceiverErrorAttr covers the non-nilable branch of
// the typed-nil guard: an error implemented on a value type can never be
// nil, and must marshal as its message.
type valErr struct{ msg string }

func (v valErr) Error() string { return v.msg }

func TestMarshalJSONValueReceiverErrorAttr(t *testing.T) {
	err := aerr.Message("m").With("cause", valErr{msg: "value receiver"}).Err(nil)
	e, _ := aerr.AsAerr(err)
	raw, jerr := json.Marshal(e)
	if jerr != nil {
		t.Fatalf("MarshalJSON failed: %v", jerr)
	}
	var decoded map[string]any
	if jerr := json.Unmarshal(raw, &decoded); jerr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jerr, raw)
	}
	attrs, _ := decoded["attributes"].(map[string]any)
	if attrs["cause"] != "value receiver" {
		t.Errorf("value-receiver error attr = %v, want its message", attrs["cause"])
	}
}

// --- remaining constructors and setters ---

func TestPackageStackTraceAndBuilderSetters(t *testing.T) {
	err := aerr.StackTrace().Message("built").Code("SET_CODE").Err(nil)
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected aerr")
	}
	if e.Error() != "built" {
		t.Errorf("Error() = %q, want built", e.Error())
	}
	if e.Code() != "SET_CODE" {
		t.Errorf("Code() = %q, want SET_CODE", e.Code())
	}
	if len(e.Traces()) == 0 {
		t.Error("package-level StackTrace() must enable capture")
	}
}

// ptrErr is an error whose Error has a pointer receiver and which, unlike
// *aerr.Error, does NOT implement json.Marshaler. A typed-nil ptrErr thus
// reaches attrJSON's typed-nil guard instead of encoding/json's own.
type ptrErr struct{ msg string }

func (p *ptrErr) Error() string { return p.msg }

func TestMarshalJSONNilReceiverAndNilErrorAttr(t *testing.T) {
	// A nil receiver goes through MarshalJSON's own nil guard (json.Marshal
	// of a nil pointer would short-circuit before ever calling the method).
	var e *aerr.Error
	raw, err := e.MarshalJSON()
	if err != nil || string(raw) != "null" {
		t.Errorf("(*Error)(nil).MarshalJSON() = %s (%v), want null", raw, err)
	}

	// A typed-nil non-Marshaler error attribute must marshal as JSON null
	// without calling Error() on the nil pointer.
	var typedNil *ptrErr
	werr := aerr.Message("m").With("nilerr", typedNil).Err(nil)
	ae, _ := aerr.AsAerr(werr)
	out, jerr := json.Marshal(ae)
	if jerr != nil {
		t.Fatalf("MarshalJSON failed: %v", jerr)
	}
	var decoded map[string]any
	if jerr := json.Unmarshal(out, &decoded); jerr != nil {
		t.Fatalf("invalid JSON: %v\n%s", jerr, out)
	}
	attrs, _ := decoded["attributes"].(map[string]any)
	if v, ok := attrs["nilerr"]; !ok || v != nil {
		t.Errorf("typed-nil error attr = %v (present=%v), want JSON null", v, ok)
	}
}
