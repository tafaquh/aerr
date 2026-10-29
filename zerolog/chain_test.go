package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

// multiErr is a foreign aggregate following the standard Unwrap() []error
// convention. It deliberately allows nil members, which the convention
// forbids but a hand-rolled aggregate can still produce.
type multiErr []error

func (m multiErr) Error() string   { return "multi" }
func (m multiErr) Unwrap() []error { return m }

// registeredError logs err through the Register()-installed marshal func
// and returns the decoded "error" field.
func registeredError(t *testing.T, err error) any {
	t.Helper()
	withFreshRegister(t)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Error().Err(err).Msg("boom")
	return decodeField(t, &buf, "error")
}

// objectError logs err through the Object helper and returns the decoded
// "err" field.
func objectError(t *testing.T, err error) any {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("boom")
	return decodeField(t, &buf, "err")
}

func decodeField(t *testing.T, buf *bytes.Buffer, key string) any {
	t.Helper()
	var event map[string]any
	if err := json.Unmarshal(buf.Bytes(), &event); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, buf.String())
	}
	return event[key]
}

// TestWrappedAerrKeepsWrapperMessage pins that an aerr error behind a
// non-aerr wrapper renders with the wrapper's full message — the context
// fmt.Errorf adds must not be dropped in favor of the inner message — while
// code and attributes still come from the aerr error.
func TestWrappedAerrKeepsWrapperMessage(t *testing.T) {
	inner := aerr.Code("DB_ERROR").Message("query failed").With("table", "users").Err(errors.New("timeout"))
	err := fmt.Errorf("handler GET /users: %w", inner)

	want := map[string]any{
		"code":       "DB_ERROR",
		"message":    "handler GET /users: query failed: timeout",
		"attributes": map[string]any{"table": "users"},
	}
	if got := registeredError(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("Register: error field mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	if got := objectError(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("Object: err field mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// TestAggregateRendersEveryMember pins that an aggregate lists every member
// under "errors", each with its own code and attributes, instead of being
// rendered as its first aerr member with the others silently dropped. A bare
// aggregate has no code or attributes of its own.
func TestAggregateRendersEveryMember(t *testing.T) {
	err := aerr.Join(
		errors.New("cache write failed"),
		aerr.Code("ITEM_FAILED").Message("item a").With("index", 0).Err(errors.New("bad sku")),
		aerr.Code("ITEM_INVALID").Message("item b").With("index", 1).Err(errors.New("no price")),
	)

	want := map[string]any{
		"message": "cache write failed; item a: bad sku; item b: no price",
		"errors": []any{
			map[string]any{"message": "cache write failed"},
			map[string]any{
				"code":       "ITEM_FAILED",
				"message":    "item a: bad sku",
				"attributes": map[string]any{"index": float64(0)},
			},
			map[string]any{
				"code":       "ITEM_INVALID",
				"message":    "item b: no price",
				"attributes": map[string]any{"index": float64(1)},
			},
		},
	}
	if got := registeredError(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("Register: error field mismatch:\n got: %#v\nwant: %#v", got, want)
	}
	if got := objectError(t, err); !reflect.DeepEqual(got, want) {
		t.Errorf("Object: err field mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

// TestWrappedAggregateListsMembers pins that an aerr error wrapping an
// aggregate keeps its own payload and also lists every member, so the
// failures beyond the first stay visible.
func TestWrappedAggregateListsMembers(t *testing.T) {
	join := aerr.Join(
		aerr.Code("ITEM_FAILED").Message("item a").With("index", 0).Err(nil),
		aerr.Code("ITEM_INVALID").Message("item b").With("index", 1).Err(nil),
	)
	err := aerr.Code("IMPORT_FAILED").Message("import failed").Wrap(join)

	got, ok := registeredError(t, err).(map[string]any)
	if !ok {
		t.Fatalf("error field is not an object: %#v", got)
	}
	if got["code"] != "IMPORT_FAILED" || got["message"] != "import failed: item a; item b" {
		t.Errorf("wrapper payload = %#v, want code IMPORT_FAILED and the combined message", got)
	}
	wantMembers := []any{
		map[string]any{"code": "ITEM_FAILED", "message": "item a", "attributes": map[string]any{"index": float64(0)}},
		map[string]any{"code": "ITEM_INVALID", "message": "item b", "attributes": map[string]any{"index": float64(1)}},
	}
	if !reflect.DeepEqual(got["errors"], wantMembers) {
		t.Errorf("errors = %#v, want %#v", got["errors"], wantMembers)
	}
}

// TestAggregateMemberRenderingRules pins that each member is rendered by the
// same rules as a top-level error: a wrapper keeps its message and exposes
// the aerr payload it wraps, a nested aggregate lists its own members, a
// captured stack is kept per member, and a nil member renders as "<nil>"
// instead of crashing the logger.
func TestAggregateMemberRenderingRules(t *testing.T) {
	inner := aerr.Code("ITEM_FAILED").Message("item a").With("index", 0).Err(nil)
	stacked := aerr.Code("STACKED").Message("with stack").StackTrace().Err(nil)
	err := multiErr{
		fmt.Errorf("batch 7: %w", inner),
		errors.Join(errors.New("x"), errors.New("y")),
		stacked,
		nil,
	}

	got, ok := registeredError(t, err).(map[string]any)
	if !ok {
		t.Fatalf("error field is not an object: %#v", got)
	}
	members, ok := got["errors"].([]any)
	if !ok || len(members) != 4 {
		t.Fatalf("errors = %#v, want 4 members", got["errors"])
	}

	wantWrapped := map[string]any{"code": "ITEM_FAILED", "message": "batch 7: item a", "attributes": map[string]any{"index": float64(0)}}
	if !reflect.DeepEqual(members[0], wantWrapped) {
		t.Errorf("wrapped member = %#v, want %#v", members[0], wantWrapped)
	}
	wantNested := map[string]any{
		"message": "x\ny",
		"errors":  []any{map[string]any{"message": "x"}, map[string]any{"message": "y"}},
	}
	if !reflect.DeepEqual(members[1], wantNested) {
		t.Errorf("nested aggregate member = %#v, want %#v", members[1], wantNested)
	}
	if m, _ := members[2].(map[string]any); m["code"] != "STACKED" || len(asSlice(m["stacktrace"])) == 0 {
		t.Errorf("stacked member = %#v, want its own code and a non-empty stacktrace", members[2])
	}
	if !reflect.DeepEqual(members[3], map[string]any{"message": "<nil>"}) {
		t.Errorf("nil member = %#v, want a \"<nil>\" message", members[3])
	}
	if _, has := got["stacktrace"]; has {
		t.Errorf("a bare aggregate has no stack of its own, got top-level stacktrace: %#v", got)
	}
}

// TestAggregateWithoutAerrIsDelegated pins the boundary: an aggregate with
// no *aerr.Error anywhere in it is not aerr's to render, so Register leaves
// it to zerolog's default (a plain message string).
func TestAggregateWithoutAerrIsDelegated(t *testing.T) {
	err := aerr.Join(errors.New("a"), errors.New("b"))
	if got := registeredError(t, err); got != "a; b" {
		t.Errorf("error field = %#v, want the plain message string", got)
	}
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// TestBrokenMembersDoNotCrashTheLogger pins that a member whose chain panics
// when walked — a typed-nil *fs.PathError, whose Unwrap and Error both
// dereference the nil receiver — renders as "<nil>" next to the healthy
// members instead of crashing the log call, wherever it sits in the chain.
func TestBrokenMembersDoNotCrashTheLogger(t *testing.T) {
	var pathErr *fs.PathError
	item := aerr.Code("ITEM").Message("item a").Err(nil)
	cases := map[string]error{
		"typed-nil member after aerr":  aerr.Join(item, pathErr),
		"typed-nil member before aerr": aerr.Join(pathErr, item),
		"wrapped typed-nil member":     aerr.Join(item, fmt.Errorf("ctx: %w", pathErr)),
		"typed-nil *aerr.Error member": aerr.Join(item, (*aerr.Error)(nil)),
		"typed-nil behind a wrapper":   fmt.Errorf("handler: %w", pathErr),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			for via, got := range map[string]any{"Register": registeredError(t, err), "Object": objectError(t, err)} {
				obj, ok := got.(map[string]any)
				if !ok {
					t.Fatalf("%s: error field is not an object: %#v", via, got)
				}
				if _, ok := obj["message"].(string); !ok {
					t.Errorf("%s: message missing: %#v", via, obj)
				}
			}
		})
	}

	got, _ := registeredError(t, aerr.Join(item, pathErr)).(map[string]any)
	want := []any{
		map[string]any{"code": "ITEM", "message": "item a"},
		map[string]any{"message": "<nil>"},
	}
	if !reflect.DeepEqual(got["errors"], want) {
		t.Errorf("errors = %#v, want the healthy member and a \"<nil>\" one: %#v", got["errors"], want)
	}
}

// TestMemberBudgetOmitsTheRest pins the size cap: at most 32 members are
// expanded per line, counted across nesting, and the rest are counted under
// "errors_omitted". Direct members are allotted before nested ones, so a
// large nested aggregate cannot crowd out its siblings.
func TestMemberBudgetOmitsTheRest(t *testing.T) {
	items := func(n int) []error {
		out := make([]error, n)
		for i := range out {
			out[i] = aerr.Code("ITEM").With("index", i).Err(nil)
		}
		return out
	}

	got, _ := registeredError(t, aerr.Join(items(40)...)).(map[string]any)
	if n := len(asSlice(got["errors"])); n != 32 {
		t.Errorf("rendered %d members, want 32", n)
	}
	if got["errors_omitted"] != float64(8) {
		t.Errorf("errors_omitted = %#v, want 8", got["errors_omitted"])
	}

	nested := multiErr(items(40))
	got, _ = registeredError(t, aerr.Join(nested, items(2)[0], items(2)[1])).(map[string]any)
	top := asSlice(got["errors"])
	if len(top) != 3 || got["errors_omitted"] != nil {
		t.Fatalf("top level = %d members, omitted %#v; want all 3 direct members", len(top), got["errors_omitted"])
	}
	inner, _ := top[0].(map[string]any)
	if n := len(asSlice(inner["errors"])); n != 29 || inner["errors_omitted"] != float64(11) {
		t.Errorf("nested aggregate = %d members, omitted %#v; want 29 and 11", n, inner["errors_omitted"])
	}
}

// TestSelfContainingAggregateTerminates pins that an aggregate containing
// itself — pathological, but constructible with a mutable foreign type —
// renders a bounded line instead of recursing until the stack overflows.
func TestSelfContainingAggregateTerminates(t *testing.T) {
	cyclic := multiErr{aerr.Code("ITEM").Message("item a").Err(nil), nil}
	cyclic[1] = cyclic

	got, ok := registeredError(t, cyclic).(map[string]any)
	if !ok {
		t.Fatalf("error field is not an object: %#v", got)
	}
	if n := countMembers(got); n > 32 {
		t.Errorf("expanded %d members in total, want at most 32", n)
	}
}

// TestRedactedMemberStaysMasked is the leak canary for aggregate members:
// rendering member attributes must not unmask a Redacted value.
func TestRedactedMemberStaysMasked(t *testing.T) {
	const secret = "s3cr3t-member-canary"
	err := aerr.Join(errors.New("other"), aerr.Code("AUTH").With("password", aerr.Redact(secret)).Err(nil))

	withFreshRegister(t)
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Error().Err(err).Msg("boom")
	logger.Error().Object("err", aerrzerolog.Object(err)).Msg("boom")
	if out := buf.String(); strings.Contains(out, secret) || strings.Count(out, aerr.RedactedText) != 2 {
		t.Errorf("member attribute not masked on both paths:\n%s", out)
	}
}

// countMembers counts every member object under "errors", at any depth.
func countMembers(obj map[string]any) int {
	n := 0
	for _, m := range asSlice(obj["errors"]) {
		member, _ := m.(map[string]any)
		n += 1 + countMembers(member)
	}
	return n
}

// selfLoop is a buggy wrapper whose Unwrap returns itself: a single-cause
// cycle, which errors.Is would also loop on.
type selfLoop struct{}

func (*selfLoop) Error() string   { return "self" }
func (s *selfLoop) Unwrap() error { return s }

// TestSelfLoopingMemberTerminates pins that a member whose Unwrap chain
// loops back on itself still renders — as its message — instead of hanging
// the log call.
func TestSelfLoopingMemberTerminates(t *testing.T) {
	err := aerr.Join(aerr.Code("ITEM").Message("item a").Err(nil), &selfLoop{})
	done := make(chan any, 1)
	go func() { done <- registeredError(t, err) }()
	select {
	case got := <-done:
		obj, _ := got.(map[string]any)
		members := asSlice(obj["errors"])
		if len(members) != 2 || !reflect.DeepEqual(members[1], map[string]any{"message": "self"}) {
			t.Errorf("errors = %#v, want the self-looping member rendered as its message", obj["errors"])
		}
	case <-time.After(10 * time.Second):
		t.Fatal("rendering a self-looping member did not terminate")
	}
}
