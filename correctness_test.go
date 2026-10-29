package aerr_test

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// --- typed-nil *Error inputs ---

func TestErrWithTypedNilAerrCause(t *testing.T) {
	var typedNil *aerr.Error
	var cause error = typedNil

	err := aerr.Code("OUTER").Message("outer failed").Err(cause) // must not panic
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "outer failed" {
		t.Errorf("expected message %q, got %q", "outer failed", err.Error())
	}
}

func TestWrapTypedNilAerr(t *testing.T) {
	var typedNil *aerr.Error
	var cause error = typedNil

	err := aerr.Message("outer").Wrap(cause) // must not panic
	if err == nil {
		t.Fatal("expected error, got nil (Wrap only returns nil for untyped nil)")
	}
	if err.Error() != "outer" {
		t.Errorf("expected message %q, got %q", "outer", err.Error())
	}
}

func TestAsAerrTypedNil(t *testing.T) {
	var typedNil *aerr.Error

	if e, ok := aerr.AsAerr(typedNil); ok || e != nil {
		t.Errorf("AsAerr(typed nil) = (%v, %v), want (nil, false)", e, ok)
	}

	wrapped := fmt.Errorf("wrapper: %w", typedNil)
	if e, ok := aerr.AsAerr(wrapped); ok || e != nil {
		t.Errorf("AsAerr(wrapped typed nil) = (%v, %v), want (nil, false)", e, ok)
	}
}

func TestAsAerrContract(t *testing.T) {
	direct := aerr.Code("X").ErrMsg("boom")

	if e, ok := aerr.AsAerr(direct); !ok || e == nil {
		t.Fatalf("AsAerr(direct) = (%v, %v), want match", e, ok)
	}
	wrapped := fmt.Errorf("mid: %w", direct)
	if e, ok := aerr.AsAerr(wrapped); !ok || e == nil || e.Code() != "X" {
		t.Fatalf("AsAerr(wrapped) = (%v, %v), want the inner *Error", e, ok)
	}
	if e, ok := aerr.AsAerr(errors.New("plain")); ok || e != nil {
		t.Errorf("AsAerr(plain) = (%v, %v), want (nil, false)", e, ok)
	}
	if e, ok := aerr.AsAerr(nil); ok || e != nil {
		t.Errorf("AsAerr(nil) = (%v, %v), want (nil, false)", e, ok)
	}
}

// --- stack precedence: deepest wins in both Err and Wrap ---

func originWithStack() error {
	return aerr.Code("INNER").Message("origin").StackTrace().Err(nil)
}

func TestErrKeepsDeepestStack(t *testing.T) {
	inner := originWithStack()
	outer := aerr.Code("OUTER").Message("outer").StackTrace().Err(inner)

	e, ok := aerr.AsAerr(outer)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected traces")
	}
	if !strings.Contains(traces[0], "originWithStack") {
		t.Errorf("expected deepest frame originWithStack first, got %q", traces[0])
	}
}

func TestWrapKeepsDeepestStack(t *testing.T) {
	inner := originWithStack()
	outer := aerr.Code("OUTER").Message("outer").StackTrace().Wrap(inner)

	e, ok := aerr.AsAerr(outer)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected traces")
	}
	if !strings.Contains(traces[0], "originWithStack") {
		t.Errorf("expected deepest frame originWithStack first (deepest wins), got %q", traces[0])
	}
}

func TestWrapCapturesWhenInnerHasNoStack(t *testing.T) {
	inner := aerr.Code("INNER").Message("no stack here").Err(nil)
	outer := aerr.Message("outer").StackTrace().Wrap(inner)

	e, ok := aerr.AsAerr(outer)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected outer StackTrace() to capture when inner has none")
	}
	if !strings.Contains(traces[0], "TestWrapCapturesWhenInnerHasNoStack") {
		t.Errorf("expected first frame in this test, got %q", traces[0])
	}
}

// --- no aerr-internal frames leak into traces ---

func assertCleanTrace(t *testing.T, err error, wantFirstFrame string) {
	t.Helper()
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected non-empty traces")
	}
	for _, fr := range traces {
		if strings.Contains(fr, "tafaquh/aerr.") {
			t.Errorf("aerr-internal frame leaked into trace: %q", fr)
		}
		if strings.Contains(fr, "testing.") && strings.Contains(fr, "/testing/") {
			t.Errorf("stdlib testing frame leaked into trace: %q", fr)
		}
	}
	if !strings.Contains(traces[0], wantFirstFrame) {
		t.Errorf("expected first frame to be %s, got %q", wantFirstFrame, traces[0])
	}
}

func TestNoInternalFramesErr(t *testing.T) {
	err := aerr.Code("X").StackTrace().Err(errors.New("cause"))
	assertCleanTrace(t, err, "TestNoInternalFramesErr")
}

func TestNoInternalFramesErrMsg(t *testing.T) {
	err := aerr.Code("X").StackTrace().ErrMsg("boom")
	assertCleanTrace(t, err, "TestNoInternalFramesErrMsg")
}

func TestNoInternalFramesWrap(t *testing.T) {
	err := aerr.Code("X").StackTrace().Wrap(errors.New("cause"))
	assertCleanTrace(t, err, "TestNoInternalFramesWrap")
}

// --- stdlib frames (multi-segment packages like net/http) are filtered ---

func TestStdlibFramesFiltered(t *testing.T) {
	var captured error
	h := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		captured = aerr.Code("H").Message("handler failed").StackTrace().Err(nil)
	})
	srv := httptest.NewServer(h)
	defer srv.Close()
	if _, err := http.Get(srv.URL); err != nil {
		t.Fatalf("GET failed: %v", err)
	}

	e, ok := aerr.AsAerr(captured)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected traces from handler")
	}
	for _, fr := range traces {
		if strings.Contains(fr, "net/http") {
			t.Errorf("stdlib net/http frame not filtered: %q", fr)
		}
	}
	if !strings.Contains(traces[0], "TestStdlibFramesFiltered") {
		t.Errorf("expected handler closure frame first, got %q", traces[0])
	}
}

// --- absorption through non-aerr wrappers (fmt.Errorf %w) ---

func TestAbsorbThroughNonAerrWrapper(t *testing.T) {
	inner := aerr.Code("DB_ERROR").
		Message("query failed").
		StackTrace().
		With("query", "SELECT 1").
		Err(errors.New("timeout"))

	mid := fmt.Errorf("mid layer: %w", inner)

	outer := aerr.Message("request failed").With("endpoint", "/x").Wrap(mid)

	e, ok := aerr.AsAerr(outer)
	if !ok {
		t.Fatal("expected aerr")
	}

	// Full chain text preserved, including the non-aerr middle layer.
	want := "request failed: mid layer: query failed: timeout"
	if e.Error() != want {
		t.Errorf("message = %q, want %q", e.Error(), want)
	}

	// Code inherited through the wrapper (outer has none).
	if e.Code() != "DB_ERROR" {
		t.Errorf("code = %q, want DB_ERROR inherited through %%w wrapper", e.Code())
	}

	// Attributes merged through the wrapper.
	attrs := e.Attributes()
	if attrs["query"] != "SELECT 1" {
		t.Errorf("inner attribute lost through %%w wrapper: %v", attrs)
	}
	if attrs["endpoint"] != "/x" {
		t.Errorf("outer attribute missing: %v", attrs)
	}

	// Stack inherited through the wrapper.
	if len(e.Traces()) == 0 {
		t.Error("inner stack trace lost through %w wrapper")
	}

	// The full original chain is still walkable.
	if !errors.Is(outer, inner) {
		t.Error("errors.Is must reach the inner error through the wrapper")
	}
}

func TestOuterCodeWinsThroughWrapper(t *testing.T) {
	inner := aerr.Code("INNER").ErrMsg("in")
	mid := fmt.Errorf("mid: %w", inner)
	outer := aerr.Code("OUTER").Message("out").Wrap(mid)

	e, _ := aerr.AsAerr(outer)
	if e.Code() != "OUTER" {
		t.Errorf("code = %q, want OUTER (outer wins)", e.Code())
	}
}

// --- builder reuse must not mutate issued errors ---

func TestBuilderReuseDoesNotMutateIssuedErrors(t *testing.T) {
	base := aerr.Code("BASE").With("attempt", 1)

	first := base.Err(nil)
	e1, _ := aerr.AsAerr(first)
	if got := e1.Attributes()["attempt"]; got != 1 {
		t.Fatalf("first error attempt = %v, want 1", got)
	}

	// Reuse the builder as a template: overwrite the attribute and add one.
	second := base.With("attempt", 2).With("extra", "x").Err(nil)

	if got := e1.Attributes()["attempt"]; got != 1 {
		t.Errorf("issued error mutated by builder reuse: attempt = %v, want 1", got)
	}
	if _, ok := e1.Attributes()["extra"]; ok {
		t.Error("issued error gained an attribute from builder reuse")
	}
	e2, _ := aerr.AsAerr(second)
	if got := e2.Attributes()["attempt"]; got != 2 {
		t.Errorf("second error attempt = %v, want 2", got)
	}
}

// TestBuilderReuseAcrossMergesKeepsIssuedErrors re-finalizes one builder
// against two different inner errors with no With in between. finalize hands
// the builder's attribute slice to each issued error, so merging must never
// append into that shared slice's spare capacity, or the second error's
// inner attribute would overwrite the first's.
func TestBuilderReuseAcrossMergesKeepsIssuedErrors(t *testing.T) {
	base := aerr.Message("outer").With("o", 1)
	first := base.Wrap(aerr.Code("A").With("a", "from-a").ErrMsg("a"))
	second := base.Wrap(aerr.Code("B").With("b", "from-b").ErrMsg("b"))

	keys1, vals1 := rangeAttrs(t, first)
	keys2, vals2 := rangeAttrs(t, second)
	if got := fmt.Sprint(keys1, vals1); got != "[o a] [1 from-a]" {
		t.Errorf("first error attributes = %s, want [o a] [1 from-a]", got)
	}
	if got := fmt.Sprint(keys2, vals2); got != "[o b] [1 from-b]" {
		t.Errorf("second error attributes = %s, want [o b] [1 from-b]", got)
	}
}

// TestBuilderReuseMatchesCopyModel drives one builder through random
// sequences of With, Err, and Wrap over inner errors carrying attributes of
// their own, then checks every issued error against a model that copies
// eagerly: finalize's hand-off must be indistinguishable from a copy.
func TestBuilderReuseMatchesCopyModel(t *testing.T) {
	type kv struct {
		k string
		v int
	}
	set := func(m []kv, k string, v int) []kv {
		for i := range m {
			if m[i].k == k {
				m[i].v = v
				return m
			}
		}
		return append(m, kv{k, v})
	}
	keys := []string{"a", "b", "c", "d"}
	for seed := int64(0); seed < 500; seed++ {
		r := rand.New(rand.NewSource(seed))
		b, model := aerr.Code("X"), []kv(nil)
		var issued []error
		var want []string
		for step := 0; step < 30; step++ {
			switch r.Intn(4) {
			case 0, 1:
				k, v := keys[r.Intn(len(keys))], r.Intn(100)
				b.With(k, v)
				model = set(model, k, v)
			case 2:
				issued = append(issued, b.Err(nil))
				want = append(want, fmt.Sprint(model))
			case 3:
				ib, merged := aerr.Code("IN"), append([]kv(nil), model...)
				for j := r.Intn(3); j >= 0; j-- {
					k, v := keys[r.Intn(len(keys))], -r.Intn(100)
					ib.With(k, v)
					if !strings.Contains(fmt.Sprint(model), "{"+k+" ") {
						merged = set(merged, k, v) // outer keys win
					}
				}
				issued = append(issued, b.Wrap(ib.ErrMsg("in")))
				want = append(want, fmt.Sprint(merged))
			}
		}
		for i, err := range issued {
			var got []kv
			e, _ := aerr.AsAerr(err)
			e.RangeAttrs(func(k string, v any) bool { got = append(got, kv{k, v.(int)}); return true })
			if fmt.Sprint(got) != want[i] {
				t.Fatalf("seed %d, issued error %d: attributes %v, want %s", seed, i, got, want[i])
			}
		}
	}
}

// TestErrorCreationAllocations pins the creation cost: the builder stays on
// the stack and its attribute slice is handed to the issued error, leaving
// one allocation each for the attribute slice, the *Error, and the
// errors.New cause.
func TestErrorCreationAllocations(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() {
		_ = aerr.Code("TEST").With("k1", "v1").With("k2", "v2").ErrMsg("cause")
	}); n != 3 {
		t.Errorf("creating an error allocated %v times, want 3", n)
	}
}

func TestBuilderReuseDoesNotCorruptMergedAttrs(t *testing.T) {
	inner := aerr.Code("IN").With("k", "v").ErrMsg("in")

	base := aerr.Message("outer").With("o", 1)
	first := base.Wrap(inner)
	e1, _ := aerr.AsAerr(first)
	if e1.Attributes()["k"] != "v" {
		t.Fatalf("merged attr missing: %v", e1.Attributes())
	}

	// A second finalize on the same builder must not clobber the first
	// error's merged attributes.
	_ = base.With("o2", 2).Wrap(inner)
	if e1.Attributes()["k"] != "v" || e1.Attributes()["o"] != 1 {
		t.Errorf("first error corrupted by builder reuse: %v", e1.Attributes())
	}
	if _, ok := e1.Attributes()["o2"]; ok {
		t.Errorf("first error gained attr from later finalize: %v", e1.Attributes())
	}
}
