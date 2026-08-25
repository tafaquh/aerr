package aerr_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// joinType is the dynamic type Join uses for a real aggregate. The type is
// unexported, so the invariant tests identify it by reflection instead of a
// type assertion.
var joinType = reflect.TypeOf(aerr.Join(errors.New("a"), errors.New("b")))

// members returns err's aggregated errors via the standard convention, or
// nil when err is not an aggregate. It deliberately does not go through
// aerr.Errors, so the formatting and invariant tests observe the raw
// Unwrap() []error contract.
func members(err error) []error {
	agg, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return nil
	}
	return agg.Unwrap()
}

// --- Join: nil filtering, passthrough, message ---

func TestJoinTruthTable(t *testing.T) {
	e1 := errors.New("e1 msg")
	e2 := errors.New("e2 msg")

	if got := aerr.Join(); got != nil {
		t.Errorf("Join() = %v, want nil", got)
	}
	if got := aerr.Join(nil, nil); got != nil {
		t.Errorf("Join(nil, nil) = %v, want nil", got)
	}
	if got := aerr.Join(e1); got != e1 {
		t.Errorf("Join(e1) = %v, want the same value by identity", got)
	}
	if got := aerr.Join(nil, e1, nil); got != e1 {
		t.Errorf("Join(nil, e1, nil) = %v, want the same value by identity", got)
	}
	joined := aerr.Join(e1, e2)
	if got, want := joined.Error(), "e1 msg; e2 msg"; got != want {
		t.Errorf("Join(e1, e2).Error() = %q, want %q", got, want)
	}
	if got := len(members(joined)); got != 2 {
		t.Errorf("len(members) = %d, want 2", got)
	}
}

// --- flattening: conservative on construction ---

func TestJoinFlattensOwnAggregates(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")

	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nested left", aerr.Join(aerr.Join(a, b), c), 3},
		{"nested right", aerr.Join(a, aerr.Join(b, c)), 3},
		{"nested both sides", aerr.Join(aerr.Join(a, b), aerr.Join(c, c)), 4},
		{"three levels deep", aerr.Join(aerr.Join(aerr.Join(a, b), c), a), 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := members(tc.err)
			if len(got) != tc.want {
				t.Fatalf("len(members) = %d, want %d", len(got), tc.want)
			}
			for i, m := range got {
				if reflect.TypeOf(m) == joinType {
					t.Errorf("member %d is a nested aggregate: %v", i, m)
				}
			}
		})
	}
}

// TestJoinTreatsForeignAggregatesAsOpaque pins the construction/extraction
// asymmetry: an errors.Join result is a single member, not spliced.
func TestJoinTreatsForeignAggregatesAsOpaque(t *testing.T) {
	a, b, c := errors.New("a"), errors.New("b"), errors.New("c")

	joined := aerr.Join(errors.Join(a, b), c)
	got := members(joined)
	if len(got) != 2 {
		t.Fatalf("len(members) = %d, want 2 (stdlib aggregate stays opaque)", len(got))
	}
	if !errors.Is(joined, a) || !errors.Is(joined, b) {
		t.Error("errors.Is should still reach through the nested stdlib aggregate")
	}
}

// TestJoinInvariants walks mixed constructions and asserts the documented
// invariants: at least two members, no nil member, no nested aggregate.
func TestJoinInvariants(t *testing.T) {
	a := errors.New("a")
	coded := aerr.Code("C").Message("coded").Err(nil)

	var accumulated error
	for _, err := range []error{a, nil, coded, nil, aerr.Join(a, coded)} {
		aerr.JoinInto(&accumulated, err)
	}

	cases := []error{
		aerr.Join(a, nil, coded),
		aerr.Join(aerr.Join(a, coded), nil, aerr.Join(coded, a)),
		accumulated,
	}
	for i, err := range cases {
		got := members(err)
		if len(got) < 2 {
			t.Errorf("case %d: len(members) = %d, want >= 2", i, len(got))
		}
		for k, m := range got {
			if m == nil {
				t.Errorf("case %d: member %d is nil", i, k)
			}
			if reflect.TypeOf(m) == joinType {
				t.Errorf("case %d: member %d is a nested aggregate", i, k)
			}
		}
	}
}

// --- Errors ---

func TestErrorsExtraction(t *testing.T) {
	a, b := errors.New("a"), errors.New("b")

	if got := aerr.Errors(nil); got != nil {
		t.Errorf("Errors(nil) = %v, want nil", got)
	}
	if got := aerr.Errors(a); len(got) != 1 || got[0] != a {
		t.Errorf("Errors(plain) = %v, want [a]", got)
	}
	if got := aerr.Errors(aerr.Join(a, b)); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("Errors(Join(a, b)) = %v, want [a b]", got)
	}
	// Read-liberal: any Unwrap() []error implementation, not just ours.
	if got := aerr.Errors(errors.Join(a, b)); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("Errors(errors.Join(a, b)) = %v, want [a b]", got)
	}
}

// TestErrorsReturnsCopy proves the caller may keep and mutate the slice
// without corrupting the aggregate it came from.
func TestErrorsReturnsCopy(t *testing.T) {
	a, b := errors.New("a"), errors.New("b")
	joined := aerr.Join(a, b)

	got := aerr.Errors(joined)
	got[0] = errors.New("mutated")

	if again := aerr.Errors(joined); again[0] != a {
		t.Errorf("mutating the returned slice changed the source: %v", again[0])
	}
	if joined.Error() != "a; b" {
		t.Errorf("Error() = %q, want %q", joined.Error(), "a; b")
	}
}

// --- JoinInto ---

func TestJoinIntoNilTargetPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("JoinInto(nil, nil) did not panic")
		}
		const want = "aerr: JoinInto: into pointer must not be nil"
		if got, ok := r.(string); !ok || got != want {
			t.Errorf("panic = %v, want %q", r, want)
		}
	}()
	// The nil-pointer check comes first: a nil err must not mask it.
	aerr.JoinInto(nil, nil)
}

func TestJoinIntoNilErr(t *testing.T) {
	sentinel := errors.New("sentinel")
	target := sentinel

	if aerr.JoinInto(&target, nil) {
		t.Error("JoinInto(&target, nil) = true, want false")
	}
	if target != sentinel {
		t.Errorf("target = %v, want untouched", target)
	}

	var empty error
	if aerr.JoinInto(&empty, nil) {
		t.Error("JoinInto(&empty, nil) = true, want false")
	}
	if empty != nil {
		t.Errorf("empty = %v, want nil", empty)
	}
}

func TestJoinIntoAccumulates(t *testing.T) {
	steps := []error{nil, errors.New("step 2"), nil, errors.New("step 4"), errors.New("step 5")}

	var err error
	failures := 0
	for i, step := range steps {
		if aerr.JoinInto(&err, step) {
			failures++
			continue
		}
		if err != nil && i == 0 {
			t.Error("a nil step must not start an accumulation")
		}
	}
	if failures != 3 {
		t.Errorf("failures = %d, want 3", failures)
	}
	if got, want := err.Error(), "step 2; step 4; step 5"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := len(members(err)); got != 3 {
		t.Errorf("len(members) = %d, want 3 (accumulation must stay flat)", got)
	}
}

// TestJoinIntoFirstAppendPassesIdentity pins that accumulating a single
// failure allocates no wrapper.
func TestJoinIntoFirstAppendPassesIdentity(t *testing.T) {
	first := errors.New("first")

	var err error
	aerr.JoinInto(&err, first)
	if err != first {
		t.Errorf("err = %v, want the first error by identity", err)
	}
}

// --- JoinFunc ---

func TestJoinFuncNilTargetPanicsBeforeCall(t *testing.T) {
	called := false
	fn := func() error {
		called = true
		return nil
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("JoinFunc(nil, fn) did not panic")
		}
		const want = "aerr: JoinFunc: into pointer must not be nil"
		if got, ok := r.(string); !ok || got != want {
			t.Errorf("panic = %v, want %q", r, want)
		}
		if called {
			t.Error("fn ran before the nil-pointer check")
		}
	}()
	aerr.JoinFunc(nil, fn)
}

// TestJoinFuncDeferredCleanup exercises the documented defer idiom: the body
// fails, the deferred cleanup fails too, and the caller sees both.
func TestJoinFuncDeferredCleanup(t *testing.T) {
	run := func(bodyErr, closeErr error) (err error) {
		defer aerr.JoinFunc(&err, func() error { return closeErr })
		return bodyErr
	}

	body := errors.New("primary")
	cleanup := errors.New("cleanup")

	if got, want := run(body, cleanup).Error(), "primary; cleanup"; got != want {
		t.Errorf("both failed: Error() = %q, want %q", got, want)
	}
	if got := run(body, nil); got != body {
		t.Errorf("body only: got %v, want the body error unchanged", got)
	}
	if got := run(nil, cleanup); got != cleanup {
		t.Errorf("cleanup only: got %v, want the cleanup error unchanged", got)
	}
	if got := run(nil, nil); got != nil {
		t.Errorf("success path: got %v, want nil", got)
	}
}

// --- formatting ---

func TestJoinFormatSingleLine(t *testing.T) {
	err := aerr.Join(errors.New("first"), errors.New("second"))

	if got := fmt.Sprintf("%v", err); got != "first; second" {
		t.Errorf("%%v = %q, want %q", got, "first; second")
	}
	if got := fmt.Sprintf("%s", err); got != "first; second" {
		t.Errorf("%%s = %q, want %q", got, "first; second")
	}
	if got := fmt.Sprintf("%q", err); got != `"first; second"` {
		t.Errorf("%%q = %q, want %q", got, `"first; second"`)
	}
	if got := fmt.Sprintf("%d", err); !strings.Contains(got, "%!d") {
		t.Errorf("unknown verb should degrade to a %%!d notice, got %q", got)
	}
}

// TestJoinFormatDetailed is the golden layout test: a header, one bullet per
// member, and six-space continuation indent so an *Error's detail block
// aligns under the bullet text.
func TestJoinFormatDetailed(t *testing.T) {
	inner := aerr.Code("DB_ERROR").
		Message("query failed").
		StackTrace().
		With("table", "users").
		Err(nil)
	err := aerr.Join(inner, errors.New("cache write failed"))

	out := fmt.Sprintf("%+v", err)
	if strings.HasSuffix(out, "\n") {
		t.Error("detailed output must not end with a newline")
	}

	lines := strings.Split(out, "\n")
	want := []string{
		"2 errors occurred:",
		"    - query failed",
		"      code: DB_ERROR",
		"      attributes:",
		"          table=users",
		"      stacktrace:",
	}
	if len(lines) < len(want)+2 {
		t.Fatalf("output has %d lines, want at least %d:\n%s", len(lines), len(want)+2, out)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d = %q, want %q\nfull output:\n%s", i, lines[i], w, out)
		}
	}

	// Stack frames are non-deterministic; assert their shape only. They run
	// until the final bullet.
	last := lines[len(lines)-1]
	if last != "    - cache write failed" {
		t.Errorf("last line = %q, want %q", last, "    - cache write failed")
	}
	for i := len(want); i < len(lines)-1; i++ {
		if !strings.HasPrefix(lines[i], "          ") {
			t.Errorf("stack line %d = %q, want a ten-space indent", i, lines[i])
		}
		if strings.TrimSpace(lines[i]) == "" {
			t.Errorf("stack line %d is blank", i)
		}
	}
	if !strings.Contains(out, "TestJoinFormatDetailed") {
		t.Errorf("stack trace should name the capturing test:\n%s", out)
	}
}

// --- interop with errors, HasCode, AsAerr, and the Builder ---

func TestJoinInteropIsAs(t *testing.T) {
	sentinel := errors.New("sentinel")
	joined := aerr.Join(errors.New("other"), sentinel)

	if !errors.Is(joined, sentinel) {
		t.Error("errors.Is should find a sentinel inside a join")
	}
	if !errors.Is(aerr.Message("wrapped").Wrap(joined), sentinel) {
		t.Error("errors.Is should find a sentinel inside a wrapped join")
	}
	if errors.Is(joined, errors.New("absent")) {
		t.Error("errors.Is matched an error that is not in the join")
	}

	custom := &customErr{code: 7}
	var target *customErr
	if !errors.As(aerr.Join(errors.New("other"), custom), &target) {
		t.Fatal("errors.As should extract a custom type from a join")
	}
	if target.code != 7 {
		t.Errorf("extracted code = %d, want 7", target.code)
	}
}

// TestJoinSingleUnwrapReturnsNil documents that an aggregate has no single
// cause: errors.Unwrap (the one-error form) reports nothing, which is what
// makes Unwrap() []error the only way in.
func TestJoinSingleUnwrapReturnsNil(t *testing.T) {
	joined := aerr.Join(errors.New("a"), errors.New("b"))
	if got := errors.Unwrap(joined); got != nil {
		t.Errorf("errors.Unwrap(join) = %v, want nil", got)
	}
}

func TestJoinInteropAerrAccessors(t *testing.T) {
	coded := aerr.Code("A").Message("coded failure").Err(nil)
	joined := aerr.Join(coded, errors.New("plain"))

	if !aerr.HasCode(joined, "A") {
		t.Error("HasCode should see a code carried by a member of a join")
	}
	if aerr.HasCode(joined, "MISSING") {
		t.Error("HasCode matched a code no member carries")
	}
	e, ok := aerr.AsAerr(joined)
	if !ok {
		t.Fatal("AsAerr should find an *Error inside a join")
	}
	if e.Error() != "coded failure" {
		t.Errorf("AsAerr found %q, want the first *Error member", e.Error())
	}
}

// TestJoinWrappedByBuilder pins the composition the docs recommend: wrapping
// an aggregate merges the message and absorbs the first aerr member's code
// and attributes.
func TestJoinWrappedByBuilder(t *testing.T) {
	coded := aerr.Code("ITEM_FAILED").Message("item 3").With("index", 3).Err(nil)
	joined := aerr.Join(coded, errors.New("item 5"))

	wrapped := aerr.Message("import failed").Wrap(joined)
	if got, want := wrapped.Error(), "import failed: item 3; item 5"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	e, ok := aerr.AsAerr(wrapped)
	if !ok {
		t.Fatal("expected *aerr.Error")
	}
	if e.Code() != "ITEM_FAILED" {
		t.Errorf("Code() = %q, want ITEM_FAILED inherited from the join", e.Code())
	}
	if got := e.Attributes()["index"]; got != 3 {
		t.Errorf("attribute index = %v, want 3 absorbed from the join", got)
	}
	if !errors.Is(wrapped, joined) {
		t.Error("the wrapped error should still unwrap to the aggregate")
	}
}

// customErr is a distinct error type for the errors.As test.
type customErr struct {
	code int
}

func (c *customErr) Error() string { return fmt.Sprintf("custom %d", c.code) }

// --- examples ---

// ExampleJoin combines failures into one error: nil elements drop out, the
// message is a single "; "-separated line, and the members stay reachable.
func ExampleJoin() {
	err := aerr.Join(errors.New("query failed"), nil, errors.New("cache write failed"))

	fmt.Println(err)
	fmt.Println(len(aerr.Errors(err)))
	fmt.Println(aerr.Join(nil, nil) == nil)
	// Output:
	// query failed; cache write failed
	// 2
	// true
}

// ExampleJoinFunc captures a deferred cleanup failure alongside the failure
// the function body returned. The named return value is required.
func ExampleJoinFunc() {
	closeFile := func() error { return errors.New("close failed") }

	write := func() (err error) {
		defer aerr.JoinFunc(&err, closeFile)
		return errors.New("write failed")
	}

	fmt.Println(write())
	// Output: write failed; close failed
}
