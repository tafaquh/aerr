package aerr

import (
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
