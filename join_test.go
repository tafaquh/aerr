package aerr_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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

// --- structured rendering ---

// joinItems is the batch the rendering tests share: a plain failure and two
// aerr members with their own codes and attributes.
func joinItems() error {
	return aerr.Join(
		errors.New("cache write failed"),
		aerr.Code("ITEM_FAILED").Message("item a").With("index", 0).Err(errors.New("bad sku")),
		aerr.Code("ITEM_INVALID").Message("item b").With("index", 1).Err(errors.New("no price")),
	)
}

// joinItemsMembers is joinItems' "errors" array as aerr encodes it.
const joinItemsMembers = `[{"message":"cache write failed"},` +
	`{"code":"ITEM_FAILED","message":"item a: bad sku","attributes":{"index":0}},` +
	`{"code":"ITEM_INVALID","message":"item b: no price","attributes":{"index":1}}]`

// mustDecode parses a JSON object for structural comparison.
func mustDecode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, s)
	}
	return m
}

// multiErr is a foreign aggregate following the standard Unwrap() []error
// convention. It deliberately allows nil members, which the convention
// forbids but a hand-rolled aggregate can still produce.
type multiErr []error

func (m multiErr) Error() string   { return "multi" }
func (m multiErr) Unwrap() []error { return m }

// TestJoinMarshalJSON is the golden test for an aggregate's JSON: the
// combined message plus one object per member, each with its own code and
// attributes — not the "{}" an aggregate marshaled to before it implemented
// json.Marshaler.
func TestJoinMarshalJSON(t *testing.T) {
	got, err := json.Marshal(joinItems())
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"message":"cache write failed; item a: bad sku; item b: no price","errors":` + joinItemsMembers + `}`
	if string(got) != want {
		t.Errorf("json.Marshal(join) =\n%s\nwant\n%s", got, want)
	}
}

// joinItemsSlogMembers is joinItems' "errors" group as slog renders it:
// keyed by position, because slog has no array kind for groups.
var joinItemsSlogMembers = map[string]any{
	"0": map[string]any{"message": "cache write failed"},
	"1": map[string]any{"code": "ITEM_FAILED", "message": "item a: bad sku", "attributes": map[string]any{"index": float64(0)}},
	"2": map[string]any{"code": "ITEM_INVALID", "message": "item b: no price", "attributes": map[string]any{"index": float64(1)}},
}

// TestJoinLogValue pins the slog rendering: the members form a group keyed
// by position, so every member value goes through the handler like any
// other attribute, and the text handler keeps the record on one line.
func TestJoinLogValue(t *testing.T) {
	line := logJSON(t, func(l *slog.Logger) { l.Error("import", slog.Any("err", joinItems())) })
	want := map[string]any{
		"message": "cache write failed; item a: bad sku; item b: no price",
		"errors":  joinItemsSlogMembers,
	}
	if got := errField(t, line); !reflect.DeepEqual(got, want) {
		t.Errorf("slog err =\n%#v\nwant\n%#v", got, want)
	}

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("import", slog.Any("err", joinItems()))
	out := buf.String()
	if strings.Count(out, "\n") != 1 {
		t.Errorf("text record spans several lines:\n%s", out)
	}
	for _, sub := range []string{
		`err.message="cache write failed; item a: bad sku; item b: no price"`,
		`err.errors.0.message="cache write failed"`,
		`err.errors.1.code=ITEM_FAILED`,
		`err.errors.2.attributes.index=1`,
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("text record missing %s:\n%s", sub, out)
		}
	}
}

// TestWrappedJoinRendersMembers pins that an *Error wrapping an aggregate
// keeps its own payload and adds the members under "errors", in JSON and in
// slog.
func TestWrappedJoinRendersMembers(t *testing.T) {
	err := aerr.Code("IMPORT_FAILED").Message("import failed").Wrap(joinItems())
	want := `{"code":"IMPORT_FAILED","message":"import failed: cache write failed; item a: bad sku; item b: no price",` +
		`"attributes":{"index":0},"errors":` + joinItemsMembers + `}`

	got, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatalf("json.Marshal: %v", jerr)
	}
	if string(got) != want {
		t.Errorf("json.Marshal =\n%s\nwant\n%s", got, want)
	}

	line := logJSON(t, func(l *slog.Logger) { l.Error("import", slog.Any("err", err)) })
	wantSlog := map[string]any{
		"code":       "IMPORT_FAILED",
		"message":    "import failed: cache write failed; item a: bad sku; item b: no price",
		"attributes": map[string]any{"index": float64(0)},
		"errors":     joinItemsSlogMembers,
	}
	if got := errField(t, line); !reflect.DeepEqual(got, wantSlog) {
		t.Errorf("slog err =\n%#v\nwant\n%#v", got, wantSlog)
	}
}

// TestJoinMembersFoundThroughWrappers pins where the members come from: an
// aggregate reached through a non-aerr wrapper still renders, while an
// *Error whose chain holds no aggregate gets no "errors" key at all.
func TestJoinMembersFoundThroughWrappers(t *testing.T) {
	err := aerr.Message("import failed").Wrap(fmt.Errorf("stage 2: %w", aerr.Join(errors.New("a"), errors.New("b"))))
	got, _ := json.Marshal(err)
	if !strings.Contains(string(got), `"errors":[{"message":"a"},{"message":"b"}]`) {
		t.Errorf("members behind a fmt wrapper not rendered: %s", got)
	}

	plain, _ := json.Marshal(aerr.Message("query failed").Wrap(errors.New("timeout")))
	if strings.Contains(string(plain), `"errors"`) {
		t.Errorf("an *Error without an aggregate rendered an errors key: %s", plain)
	}
}

// TestJoinMemberRenderingRules pins that each member is rendered by the same
// rules as a top-level error: a wrapper keeps its message and exposes the
// aerr payload it wraps, a nested aggregate lists its own members, and a nil
// member renders as "<nil>" — untyped, a typed-nil *Error, or a typed-nil
// *fs.PathError, whose Unwrap would panic if the walk did not recover.
func TestJoinMemberRenderingRules(t *testing.T) {
	inner := aerr.Code("ITEM_FAILED").Message("item a").With("index", 0).Err(nil)
	var pathErr *fs.PathError
	err := aerr.Message("batch failed").Wrap(multiErr{
		fmt.Errorf("batch 7: %w", inner),
		errors.Join(errors.New("x"), errors.New("y")),
		nil,
		(*aerr.Error)(nil),
		pathErr,
	})

	got, jerr := json.Marshal(err)
	if jerr != nil {
		t.Fatalf("json.Marshal: %v", jerr)
	}
	// The wrapper inherits the first aerr member's code and attributes, per
	// the chain-merge rules; the members are what this test is about.
	want := `{"code":"ITEM_FAILED","message":"batch failed: multi","attributes":{"index":0},"errors":[` +
		`{"code":"ITEM_FAILED","message":"batch 7: item a","attributes":{"index":0}},` +
		`{"message":"x\ny","errors":[{"message":"x"},{"message":"y"}]},` +
		`{"message":"\u003cnil\u003e"},{"message":"\u003cnil\u003e"},{"message":"\u003cnil\u003e"}]}`
	if string(got) != want {
		t.Errorf("json.Marshal =\n%s\nwant\n%s", got, want)
	}
}

// TestJoinRenderingNeverPanics extends the no-panic contract to aggregates:
// a member whose Error() panics degrades to a "<panic: ...>" message, and a
// typed-nil member whose Unwrap() would panic renders as "<nil>", in JSON and
// in slog, wherever it sits.
func TestJoinRenderingNeverPanics(t *testing.T) {
	got, err := json.Marshal(aerr.Join(errors.New("fine"), badErr{}))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var out struct {
		Message string
		Errors  []struct{ Message string }
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, got)
	}
	if !strings.HasPrefix(out.Message, "<panic: ") {
		t.Errorf("combined message = %q, want a <panic: ...> placeholder", out.Message)
	}
	if len(out.Errors) != 2 || out.Errors[0].Message != "fine" || !strings.HasPrefix(out.Errors[1].Message, "<panic: ") {
		t.Errorf("members = %+v, want \"fine\" then a <panic: ...> placeholder", out.Errors)
	}

	var pathErr *fs.PathError
	item := aerr.Code("ITEM").Message("item a").Err(nil)
	for name, err := range map[string]error{
		"after":   aerr.Join(item, pathErr),
		"before":  aerr.Join(pathErr, item),
		"wrapped": aerr.Join(item, fmt.Errorf("ctx: %w", pathErr)),
	} {
		got, jerr := json.Marshal(err)
		if jerr != nil || !json.Valid(got) {
			t.Errorf("%s: json.Marshal = %s, %v", name, got, jerr)
		}
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Error("x", slog.Any("err", err))
		if strings.Contains(buf.String(), "PANIC") || !strings.Contains(buf.String(), `"code":"ITEM"`) {
			t.Errorf("%s: slog lost the members to a panic:\n%s", name, buf.String())
		}
	}
}

// TestJoinRenderingBudget pins the size cap: at most 32 members are expanded
// per rendering, counted across nesting, and the rest are counted under
// "errors_omitted". All direct members are allotted before any expands its
// own, so a large nested aggregate cannot crowd out its siblings.
func TestJoinRenderingBudget(t *testing.T) {
	items := func(n int) []error {
		out := make([]error, n)
		for i := range out {
			out[i] = aerr.Code("ITEM").With("index", i).Err(nil)
		}
		return out
	}

	var flat struct {
		Errors        []json.RawMessage
		ErrorsOmitted int `json:"errors_omitted"`
	}
	got, _ := json.Marshal(aerr.Join(items(40)...))
	if err := json.Unmarshal(got, &flat); err != nil || len(flat.Errors) != 32 || flat.ErrorsOmitted != 8 {
		t.Errorf("40 members: rendered %d, omitted %d (%v); want 32 and 8", len(flat.Errors), flat.ErrorsOmitted, err)
	}
	line := logJSON(t, func(l *slog.Logger) { l.Error("x", slog.Any("err", aerr.Join(items(40)...))) })
	obj := errField(t, line)
	if members, _ := obj["errors"].(map[string]any); len(members) != 32 || obj["errors_omitted"] != float64(8) {
		t.Errorf("slog: rendered %d, omitted %#v; want 32 and 8", len(members), obj["errors_omitted"])
	}

	var nested struct {
		Errors []struct {
			Errors        []json.RawMessage
			ErrorsOmitted int `json:"errors_omitted"`
		}
		ErrorsOmitted int `json:"errors_omitted"`
	}
	got, _ = json.Marshal(aerr.Join(multiErr(items(40)), items(1)[0], items(1)[0]))
	if err := json.Unmarshal(got, &nested); err != nil || len(nested.Errors) != 3 || nested.ErrorsOmitted != 0 {
		t.Fatalf("top level: %d members, omitted %d (%v); want all 3 direct members", len(nested.Errors), nested.ErrorsOmitted, err)
	}
	if n, omitted := len(nested.Errors[0].Errors), nested.Errors[0].ErrorsOmitted; n != 29 || omitted != 11 {
		t.Errorf("nested aggregate: %d members, omitted %d; want 29 and 11", n, omitted)
	}
}

// TestJoinRenderingCycleTerminates pins that an aggregate containing itself —
// pathological, but constructible with a mutable foreign type — renders a
// bounded record in JSON and slog instead of recursing until the stack
// overflows.
func TestJoinRenderingCycleTerminates(t *testing.T) {
	cyclic := multiErr{aerr.Code("ITEM").Message("item a").Err(nil), nil}
	cyclic[1] = cyclic
	err := aerr.Message("wrapped").Wrap(cyclic)

	got, jerr := json.Marshal(err)
	if jerr != nil || !json.Valid(got) {
		t.Fatalf("json.Marshal = %s, %v", got, jerr)
	}
	if n := strings.Count(string(got), `"code":"ITEM"`); n > 33 {
		t.Errorf("expanded %d members, want at most 32 (plus the inherited top-level code)", n)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Error("x", slog.Any("err", err))
	if !json.Valid(buf.Bytes()) {
		t.Errorf("slog record is not valid JSON:\n%s", buf.String())
	}
}

// token is a slog.LogValuer that masks itself — slog's documented idiom for
// keeping a sensitive value out of logs.
type token string

func (token) LogValue() slog.Value { return slog.StringValue("TOKEN") }

// TestJoinSlogRedactionHooks pins that slog's own redaction mechanisms reach
// aggregate members, because members render as slog groups rather than as an
// opaque value: a LogValuer type, a key-based ReplaceAttr, a ReplaceAttr that
// matches the "attributes" group at any depth (the README recipe), and one
// that drops stack traces by key.
func TestJoinSlogRedactionHooks(t *testing.T) {
	member := aerr.Code("AUTH").Message("login failed").StackTrace().
		With("password", secret).With("token", token(secret)).Err(nil)
	err := aerr.Message("batch failed").Wrap(aerr.Join(errors.New("other"), member))

	hooks := map[string]func([]string, slog.Attr) slog.Attr{
		"by key": func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == "password" {
				return slog.String(a.Key, aerr.RedactedText)
			}
			return a
		},
		"by attributes group at any depth": func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 && groups[len(groups)-1] == "attributes" && a.Key == "password" {
				return slog.String(a.Key, aerr.RedactedText)
			}
			return a
		},
	}
	for name, hook := range hooks {
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: hook})).Error("x", slog.Any("err", err))
		out := buf.String()
		if strings.Contains(out, secret) {
			t.Errorf("%s: secret leaked:\n%s", name, out)
		}
		// Masked at the top level (inherited from the member) and inside it;
		// the token is masked by its own LogValue in both places as well.
		if strings.Count(out, aerr.RedactedText) != 2 || strings.Count(out, "TOKEN") != 2 {
			t.Errorf("%s: want the password and token masked twice each:\n%s", name, out)
		}
	}

	var buf bytes.Buffer
	dropStacks := func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == "stacktrace" {
			return slog.Attr{}
		}
		return a
	}
	slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{ReplaceAttr: dropStacks})).Error("x", slog.Any("err", err))
	if strings.Contains(buf.String(), "stacktrace") {
		t.Errorf("a key-based ReplaceAttr did not reach a member's stacktrace:\n%s", buf.String())
	}
}

// TestJoinRenderingMasksRedactedMembers is the leak canary for aggregates:
// now that member attributes are rendered, a member's Redacted value must
// stay masked in every renderer.
func TestJoinRenderingMasksRedactedMembers(t *testing.T) {
	member := aerr.Code("AUTH").Message("login failed").With("password", aerr.Redact(secret)).Err(nil)
	err := aerr.Message("batch failed").Wrap(aerr.Join(errors.New("other"), member))

	js, _ := json.Marshal(err)
	var slogJSON, slogText bytes.Buffer
	slog.New(slog.NewJSONHandler(&slogJSON, nil)).Error("x", slog.Any("err", err))
	slog.New(slog.NewTextHandler(&slogText, nil)).Error("x", slog.Any("err", err))
	outputs := map[string]string{
		"json.Marshal": string(js),
		"slog JSON":    slogJSON.String(),
		"slog text":    slogText.String(),
		"%+v":          fmt.Sprintf("%+v", err),
	}
	for name, out := range outputs {
		if strings.Contains(out, secret) {
			t.Errorf("%s leaked the canary:\n%s", name, out)
		}
		if !strings.Contains(out, aerr.RedactedText) {
			t.Errorf("%s lost the mask:\n%s", name, out)
		}
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

// ExampleJoin_structured shows that structured output keeps every member of
// an aggregate visible: json.Marshal — like slog and the logging adapters —
// lists each member under "errors" with its own code and attributes.
func ExampleJoin_structured() {
	err := aerr.Join(
		aerr.Code("ITEM_FAILED").Message("widget").With("index", 0).Err(nil),
		errors.New("cache write failed"),
	)

	b, _ := json.Marshal(err)
	fmt.Println(string(b))
	// Output:
	// {"message":"widget; cache write failed","errors":[{"code":"ITEM_FAILED","message":"widget","attributes":{"index":0}},{"message":"cache write failed"}]}
}
