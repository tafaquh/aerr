package aerr

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"runtime"
	"testing"
)

// TestCaptureStackDeepSkip drives captureStack's empty-result branch: a skip
// larger than the live stack makes runtime.Callers return zero frames.
func TestCaptureStackDeepSkip(t *testing.T) {
	if got := captureStack(1000); got != nil {
		t.Errorf("captureStack(hugeSkip) = %v, want nil", got)
	}
}

// TestMergeAttrsGrows covers the reallocation branch of mergeAttrs, which the
// finalize caller never reaches because it always pre-sizes the destination.
func TestMergeAttrsGrows(t *testing.T) {
	dst := []attr{{key: "a", val: 1}} // len 1, cap 1: forces a grow
	src := []attr{{key: "a", val: 99}, {key: "b", val: 2}, {key: "c", val: 3}}

	got := mergeAttrs(dst, src)
	want := []attr{{key: "a", val: 1}, {key: "b", val: 2}, {key: "c", val: 3}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeAttrs = %v, want %v (dedup 'a', preserve dst value)", got, want)
	}

	// An empty src returns dst untouched.
	if out := mergeAttrs(dst, nil); !reflect.DeepEqual(out, dst) {
		t.Errorf("mergeAttrs(dst, nil) = %v, want %v", out, dst)
	}
}

// TestMergeAttrsSharesSrcWhenDstIsEmpty pins the zero-copy case finalize
// relies on when a wrapper adds no attributes: the inner error's slice is
// returned as is, and never written to.
func TestMergeAttrsSharesSrcWhenDstIsEmpty(t *testing.T) {
	src := []attr{{key: "b", val: 2}}
	if got := mergeAttrs(nil, src); &got[0] != &src[0] {
		t.Error("mergeAttrs(nil, src) should return src itself, not a copy")
	}
	mergeAttrs([]attr{{key: "a", val: 1}}, src)
	if src[0] != (attr{key: "b", val: 2}) || len(src) != 1 {
		t.Errorf("mergeAttrs wrote to src: %v", src)
	}
}

// TestIsNilValueNilInterface covers the untyped-nil branch of isNilValue.
func TestIsNilValueNilInterface(t *testing.T) {
	if !isNilValue(nil) {
		t.Error("isNilValue(nil) = false, want true")
	}
	if isNilValue(0) {
		t.Error("isNilValue(0) = true, want false")
	}
	var p *int
	if !isNilValue(p) {
		t.Error("isNilValue(nil pointer) = false, want true")
	}
}

// TestSkipFrameFallback exercises the heuristic used when the stdlib source
// directory could not be resolved (stdlibDir == "").
func TestSkipFrameFallback(t *testing.T) {
	saved := stdlibDir
	stdlibDir = ""
	defer func() { stdlibDir = saved }()

	cases := []struct {
		name string
		fn   string
		skip bool
	}{
		{"empty symbol", "", true},
		{"aerr internal", selfPkgPrefix + "finalize", true},
		{"package main is user code", "main.run", false},
		{"slashless stdlib", "strings.Contains", true},
		// Multi-segment stdlib packages carry a slash in their function
		// names; the fallback must still classify them as stdlib. This is
		// the case the pre-fix `!strings.Contains(fn, "/")` heuristic got
		// wrong, leaking net/http and encoding/json frames under -trimpath.
		{"multi-segment stdlib (net/http)", "net/http.(*conn).serve", true},
		{"multi-segment stdlib (encoding/json)", "encoding/json.Marshal", true},
		{"multi-segment stdlib (database/sql)", "database/sql.(*DB).Query", true},
		{"module path is user code", "github.com/user/project.Handle", false},
		{"deep module path is user code", "github.com/user/project/svc.(*S).Do", false},
		{"dotted-domain module is user code", "gopkg.in/yaml.v3.Unmarshal", false},
		// Documented limitation: a dotless local module path (e.g.
		// `module myapp`) is indistinguishable from stdlib by name alone,
		// so its frames are mis-dropped on a -trimpath build.
		{"slashless local module mis-dropped", "myapp.LoadConfig", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := skipFrame(runtime.Frame{Function: tc.fn})
			if got != tc.skip {
				t.Errorf("skipFrame(%q) = %v, want %v", tc.fn, got, tc.skip)
			}
		})
	}
}

// TestRenderTracesAllInternalFiltered drives the "everything filtered" branch
// of renderTraces: a live stack captured from inside package aerr contains
// only aerr-internal and stdlib frames, all of which are dropped.
func TestRenderTracesAllInternalFiltered(t *testing.T) {
	var pcs [16]uintptr
	n := runtime.Callers(0, pcs[:])
	if got := renderTraces(pcs[:n]); got != nil {
		t.Errorf("renderTraces of only aerr/stdlib frames = %v, want nil", got)
	}
}

// TestFramesAllInternalFiltered is the Frames() counterpart of the above.
func TestFramesAllInternalFiltered(t *testing.T) {
	var pcs [16]uintptr
	n := runtime.Callers(0, pcs[:])
	e := &Error{pcs: pcs[:n]}
	if got := e.Frames(); got != nil {
		t.Errorf("Frames of only aerr/stdlib frames = %v, want nil", got)
	}
}

// TestJoinErrorNilReceiver pins that every method of the aggregate type
// tolerates a nil receiver, like *Error's. Join never returns a typed-nil
// *joinError, so only an in-package test can reach these guards.
func TestJoinErrorNilReceiver(t *testing.T) {
	var j *joinError
	if j.Error() != "" || j.Unwrap() != nil {
		t.Error("nil *joinError: want empty Error() and nil Unwrap()")
	}
	if got := j.LogValue(); !got.Equal(slog.Value{}) {
		t.Errorf("nil *joinError LogValue() = %v, want the zero Value", got)
	}
	if got, err := j.MarshalJSON(); string(got) != "null" || err != nil {
		t.Errorf("nil *joinError MarshalJSON() = %s, %v; want null, nil", got, err)
	}
	if got := fmt.Sprintf("%v", j); got != "<nil>" {
		t.Errorf("nil *joinError %%v = %q, want <nil>", got)
	}
}

// FuzzAppendJSONString pins appendJSONString to encoding/json byte for byte:
// the plain-ASCII fast path must never change what json.Marshal would emit.
func FuzzAppendJSONString(f *testing.F) {
	for _, s := range []string{"", "plain", "a<b", "a>b", "a&b", `quo"te`, `back\slash`, "tab\t", "nul\x00", "del\x7f",
		"ünïcödé", "line sep", "bad\xffutf8", "/app/main.go:19 (main.main)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		want, _ := json.Marshal(s)
		if got := appendJSONString(nil, s); string(got) != string(want) {
			t.Errorf("appendJSONString(%q) = %s, want %s", s, got, want)
		}
	})
}

// TestAppendAttrJSONMatchesEncodingJSON pins the typed fast paths to
// encoding/json's output for the same values.
func TestAppendAttrJSONMatchesEncodingJSON(t *testing.T) {
	for _, v := range []any{"s", "x<y", 0, -42, math.MaxInt, math.MinInt, true, false} {
		want, _ := json.Marshal(v)
		if got := appendAttrJSON(nil, v); string(got) != string(want) {
			t.Errorf("appendAttrJSON(%#v) = %s, want %s", v, got, want)
		}
	}
}
