package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

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
