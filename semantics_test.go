package aerr_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// rangeAttrs collects an error's attributes in insertion order.
func rangeAttrs(t *testing.T, err error) ([]string, []any) {
	t.Helper()
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected *aerr.Error")
	}
	var keys []string
	var vals []any
	e.RangeAttrs(func(k string, v any) bool {
		keys = append(keys, k)
		vals = append(vals, v)
		return true
	})
	return keys, vals
}

// --- code inheritance matrix (outer set/unset × inner set/unset) ---

func TestCodeInheritanceMatrix(t *testing.T) {
	cases := []struct {
		name  string
		outer string
		inner string
		want  string
	}{
		{"both set: outer wins", "OUTER", "INNER", "OUTER"},
		{"outer only", "OUTER", "", "OUTER"},
		{"inner only: inherited", "", "INNER", "INNER"},
		{"neither", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := aerr.Code(tc.inner).Message("inner").Err(nil)
			outer := aerr.Code(tc.outer).Message("outer").Wrap(inner)
			e, ok := aerr.AsAerr(outer)
			if !ok {
				t.Fatal("expected aerr")
			}
			if e.Code() != tc.want {
				t.Errorf("Code() = %q, want %q", e.Code(), tc.want)
			}
		})
	}
}

// TestCodeInheritedThroughNonAerrWrapper confirms inheritance also works
// across an fmt.Errorf %w layer between the two aerr layers.
func TestCodeInheritedThroughNonAerrWrapper(t *testing.T) {
	inner := aerr.Code("INNER").ErrMsg("in")
	mid := fmt.Errorf("mid: %w", inner)
	outer := aerr.Message("out").Wrap(mid) // outer has no code
	e, _ := aerr.AsAerr(outer)
	if e.Code() != "INNER" {
		t.Errorf("Code() = %q, want INNER inherited through %%w wrapper", e.Code())
	}
}

// --- message join matrix (empty left / right / both) ---

func TestMessageJoinMatrix(t *testing.T) {
	cases := []struct {
		name     string
		outerMsg string
		innerMsg string
		want     string
	}{
		{"both present", "outer", "inner", "outer: inner"},
		{"empty outer", "", "inner", "inner"},
		{"empty inner", "outer", "", "outer"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := aerr.Message(tc.innerMsg).Err(nil)
			outer := aerr.Message(tc.outerMsg).Wrap(inner)
			if outer.Error() != tc.want {
				t.Errorf("Error() = %q, want %q", outer.Error(), tc.want)
			}
		})
	}
}

// --- attribute merge: outer-wins on duplicate keys, ordered outer-then-inner ---

func TestAttrMergeOuterWinsAndOrder(t *testing.T) {
	inner := aerr.Message("inner").
		With("shared", "inner-val").
		With("inner-only", 1).
		Err(nil)
	outer := aerr.Message("outer").
		With("shared", "outer-val").
		With("outer-only", 2).
		Wrap(inner)

	e, _ := aerr.AsAerr(outer)
	if got := e.Attributes()["shared"]; got != "outer-val" {
		t.Errorf("duplicate key: got %v, want outer-val (outer wins)", got)
	}

	keys, vals := rangeAttrs(t, outer)
	wantKeys := []string{"shared", "outer-only", "inner-only"}
	wantVals := []any{"outer-val", 2, 1}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("merge order keys = %v, want %v (outer first, then non-dup inner)", keys, wantKeys)
	}
	if !reflect.DeepEqual(vals, wantVals) {
		t.Errorf("merge order vals = %v, want %v", vals, wantVals)
	}
}

// TestAttrMergeNoInnerAttrs exercises the merge path when the inner error
// carries no attributes (only the outer contributes).
func TestAttrMergeNoInnerAttrs(t *testing.T) {
	inner := aerr.Code("IN").ErrMsg("in") // no attrs
	outer := aerr.Message("out").With("a", 1).Wrap(inner)
	keys, vals := rangeAttrs(t, outer)
	if !reflect.DeepEqual(keys, []string{"a"}) || !reflect.DeepEqual(vals, []any{1}) {
		t.Errorf("attrs = %v/%v, want [a]/[1]", keys, vals)
	}
}

// --- With(): same-key overwrite updates the value but keeps the position ---

func TestWithOverwriteKeepsPosition(t *testing.T) {
	err := aerr.Message("m").
		With("a", 1).
		With("b", 2).
		With("a", 99). // overwrite a, must stay in slot 0
		With("c", 3).
		Err(nil)

	keys, vals := rangeAttrs(t, err)
	wantKeys := []string{"a", "b", "c"}
	wantVals := []any{99, 2, 3}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("keys = %v, want %v (position preserved on overwrite)", keys, wantKeys)
	}
	if !reflect.DeepEqual(vals, wantVals) {
		t.Errorf("vals = %v, want %v", vals, wantVals)
	}
}

// --- Err(nil) and builder ErrMsg("") produce equivalent errors ---

func TestErrNilEqualsBuilderErrMsgEmpty(t *testing.T) {
	a := aerr.Code("C").Message("m").With("k", "v").Err(nil)
	b := aerr.Code("C").Message("m").With("k", "v").ErrMsg("")

	ea, _ := aerr.AsAerr(a)
	eb, _ := aerr.AsAerr(b)

	if ea.Error() != eb.Error() {
		t.Errorf("message: Err(nil)=%q vs ErrMsg(\"\")=%q", ea.Error(), eb.Error())
	}
	if ea.Code() != eb.Code() {
		t.Errorf("code: %q vs %q", ea.Code(), eb.Code())
	}
	if ea.Unwrap() != nil || eb.Unwrap() != nil {
		t.Errorf("both must have a nil cause: %v vs %v", ea.Unwrap(), eb.Unwrap())
	}
	if !reflect.DeepEqual(ea.Attributes(), eb.Attributes()) {
		t.Errorf("attrs: %v vs %v", ea.Attributes(), eb.Attributes())
	}
}

// TestBuilderErrMsgWithMessage covers the non-empty branch of builder ErrMsg.
func TestBuilderErrMsgWithMessage(t *testing.T) {
	err := aerr.Code("C").Message("outer").ErrMsg("cause text")
	if err.Error() != "outer: cause text" {
		t.Errorf("Error() = %q, want 'outer: cause text'", err.Error())
	}
	if errors.Unwrap(err) == nil {
		t.Error("ErrMsg with a non-empty message must record a cause")
	}
}

// --- package-level constructors: ErrMsg / Errorf / Wrapf ---

func TestPackageConstructors(t *testing.T) {
	if got := aerr.ErrMsg("plain").Error(); got != "plain" {
		t.Errorf("ErrMsg = %q, want plain", got)
	}
	if got := aerr.Errorf("n=%d", 5).Error(); got != "n=5" {
		t.Errorf("Errorf = %q, want n=5", got)
	}

	base := errors.New("io down")
	w := aerr.Wrapf(base, "attempt %d", 2)
	if w == nil || w.Error() != "attempt 2: io down" {
		t.Errorf("Wrapf = %v, want 'attempt 2: io down'", w)
	}
	if !errors.Is(w, base) {
		t.Error("Wrapf must keep the wrapped chain")
	}
	if aerr.Wrapf(nil, "ignored %d", 1) != nil {
		t.Error("Wrapf(nil, ...) must be nil")
	}
}

// TestWrapfInheritsMetadata verifies Wrapf follows the same merge rules as
// (*Builder).Wrap (code inherited, attrs merged, stack inherited).
func TestWrapfInheritsMetadata(t *testing.T) {
	inner := aerr.Code("INNER").With("k", "v").StackTrace().ErrMsg("in")
	w := aerr.Wrapf(inner, "outer %d", 1)
	e, ok := aerr.AsAerr(w)
	if !ok {
		t.Fatal("expected aerr")
	}
	if e.Code() != "INNER" {
		t.Errorf("Wrapf code = %q, want INNER inherited", e.Code())
	}
	if e.Attributes()["k"] != "v" {
		t.Errorf("Wrapf must merge inner attrs: %v", e.Attributes())
	}
	if len(e.Traces()) == 0 {
		t.Error("Wrapf must inherit the inner stack trace")
	}
}

// --- errors.Is / errors.As through mixed aerr + %w chains ---

func TestIsAsMixedChain(t *testing.T) {
	sentinel := errors.New("sentinel")
	inner := aerr.Code("INNER").Message("inner").Err(sentinel)
	mid := fmt.Errorf("mid: %w", inner)
	outer := aerr.Code("OUTER").Message("outer").Wrap(mid)

	if !errors.Is(outer, sentinel) {
		t.Error("errors.Is must reach the leaf sentinel through the mixed chain")
	}
	if !errors.Is(outer, inner) {
		t.Error("errors.Is must reach the inner aerr through the %w wrapper")
	}

	var ae *aerr.Error
	if !errors.As(outer, &ae) {
		t.Fatal("errors.As must find an *aerr.Error")
	}
	if ae.Code() != "OUTER" {
		t.Errorf("errors.As found code %q, want the outermost OUTER", ae.Code())
	}

	// When the outermost layer is a non-aerr wrapper, As reaches the inner.
	plainOuter := fmt.Errorf("plain: %w", inner)
	var ae2 *aerr.Error
	if !errors.As(plainOuter, &ae2) || ae2.Code() != "INNER" {
		t.Errorf("errors.As through non-aerr outer = %v, want INNER", ae2)
	}
}

// --- AsAerr / HasCode across errors.Join (Unwrap() []error) trees ---

func TestAsAerrThroughJoin(t *testing.T) {
	target := aerr.Code("JOINED").ErrMsg("j")
	joined := errors.Join(errors.New("a"), target)
	e, ok := aerr.AsAerr(joined)
	if !ok || e == nil || e.Code() != "JOINED" {
		t.Errorf("AsAerr(join) = (%v, %v), want the joined aerr", e, ok)
	}

	none := errors.Join(errors.New("x"), errors.New("y"))
	if e2, ok := aerr.AsAerr(none); ok || e2 != nil {
		t.Errorf("AsAerr(join without aerr) = (%v, %v), want (nil, false)", e2, ok)
	}
}

func TestHasCodeEdges(t *testing.T) {
	joined := errors.Join(errors.New("a"), aerr.Code("X").ErrMsg("x"))
	if !aerr.HasCode(joined, "X") {
		t.Error("HasCode must find a code inside an errors.Join tree")
	}
	if aerr.HasCode(joined, "Y") {
		t.Error("HasCode must not match an absent code in a join tree")
	}

	// A typed-nil *Error in the chain must neither match nor panic.
	var typedNil *aerr.Error
	wrapped := fmt.Errorf("w: %w", typedNil)
	if aerr.HasCode(wrapped, "") {
		t.Error("typed-nil *Error must not count as a code match")
	}
}

// --- stack depth truncation ---

// deepChain recurses n times, creating a stack-capturing aerr at the bottom
// so the capture site sits far below the call depth limit.
func deepChain(n int) error {
	if n <= 0 {
		return aerr.Code("DEEP").Message("bottom").StackTrace().Err(nil)
	}
	return deepChain(n - 1)
}

func TestStackDepthTruncation(t *testing.T) {
	err := deepChain(45)
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected aerr")
	}
	traces := e.Traces()
	if len(traces) == 0 {
		t.Fatal("expected captured traces")
	}
	// stackMaxDepth caps the raw capture at 32 PCs; filtering only removes
	// frames, so the rendered trace can never exceed that.
	if len(traces) > 32 {
		t.Errorf("len(Traces()) = %d, want <= 32 (stackMaxDepth)", len(traces))
	}
	// The deepest frames are kept (origin preserved); the recursive function
	// must appear, and the very first frame is its capture site.
	if !strings.Contains(traces[0], "deepChain") {
		t.Errorf("deepest frame = %q, want the recursive deepChain call site", traces[0])
	}
	found := false
	for _, fr := range traces {
		if strings.Contains(fr, "deepChain") {
			found = true
			break
		}
	}
	if !found {
		t.Error("recursive deepChain must be present in the deepest frames")
	}
}
