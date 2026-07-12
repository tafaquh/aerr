package aerr_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/tafaquh/aerr"
)

// RedactKeys is process-global, so every test here must clear it on cleanup
// (see t.Cleanup calls) and must not run in parallel — matching the rest of
// the package, which never calls t.Parallel. The suite stays green under
// `go test -race -count=2 ./...`.

// TestRedactKeysWrapsOnWith proves that once a key is blacklisted, With stores
// a Redacted value (checked via RangeAttrs) and every core render path emits
// the placeholder while the plaintext canary never appears.
func TestRedactKeysWrapsOnWith(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	err := aerr.Code("AUTH").
		Message("login failed").
		With("password", secret).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	// Stored value is the wrapper, and Value still recovers the original.
	var seen any
	e.RangeAttrs(func(k string, v any) bool {
		if k == "password" {
			seen = v
		}
		return true
	})
	r, ok := seen.(aerr.Redacted)
	if !ok {
		t.Fatalf("attr is %T, want aerr.Redacted (auto-wrapped by RedactKeys)", seen)
	}
	if r.Value() != secret {
		t.Errorf("Value() = %v, want the original recovered", r.Value())
	}

	// Every core render path masks the value.
	raw, _ := json.Marshal(e)
	renders := map[string]string{
		"MarshalJSON": string(raw),
		"%+v":         fmt.Sprintf("%+v", e),
	}
	var jbuf bytes.Buffer
	slog.New(slog.NewJSONHandler(&jbuf, nil)).Error("x", slog.Any("err", e))
	renders["slog/json"] = jbuf.String()

	for path, out := range renders {
		if strings.Contains(out, secret) {
			t.Errorf("secret leaked via %s: %s", path, out)
		}
		if !strings.Contains(out, aerr.RedactedText) {
			t.Errorf("%s = %q, want it to contain %s", path, out, aerr.RedactedText)
		}
	}
}

// TestRedactKeysOverwriteArm confirms the overwrite-in-place path also wraps:
// two With calls on the same blacklisted key leave a single, masked attribute.
func TestRedactKeysOverwriteArm(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	err := aerr.Message("m").
		With("password", "first").
		With("password", secret).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	if e.NumAttrs() != 1 {
		t.Fatalf("NumAttrs = %d, want 1 (overwrite, not append)", e.NumAttrs())
	}
	r, ok := e.Attributes()["password"].(aerr.Redacted)
	if !ok {
		t.Fatalf("overwritten attr is %T, want aerr.Redacted", e.Attributes()["password"])
	}
	if r.Value() != secret {
		t.Errorf("Value() = %v, want the latest original", r.Value())
	}
	if got := fmt.Sprintf("%+v", e); !strings.Contains(got, "password=[REDACTED]") {
		t.Errorf("%%+v = %q, want it to contain password=[REDACTED]", got)
	}
}

// TestRedactKeysNoDoubleWrap proves an already-Redacted value passed for a
// blacklisted key is stored as-is, not wrapped a second time: Value returns
// the original plaintext, not a nested Redacted.
func TestRedactKeysNoDoubleWrap(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	err := aerr.Message("m").With("password", aerr.Redact(secret)).Err(nil)
	e, _ := aerr.AsAerr(err)

	r, ok := e.Attributes()["password"].(aerr.Redacted)
	if !ok {
		t.Fatalf("attr is %T, want aerr.Redacted", e.Attributes()["password"])
	}
	if r.Value() != secret {
		t.Errorf("Value() = %#v, want %q (no double-wrap)", r.Value(), secret)
	}
}

// TestRedactKeysNonMatching documents that matching is exact and
// case-sensitive: keys not in the set — including one differing only by case —
// are stored raw.
func TestRedactKeysNonMatching(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	err := aerr.Message("m").
		With("user", "alice").
		With("Password", secret). // differs only by case → not redacted
		Err(nil)
	e, _ := aerr.AsAerr(err)

	if got := e.Attributes()["user"]; got != "alice" {
		t.Errorf("user = %#v, want alice (non-matching key untouched)", got)
	}
	if got := e.Attributes()["Password"]; got != secret {
		t.Errorf("Password = %#v, want raw %q (matching is case-sensitive)", got, secret)
	}
}

// TestRedactKeysClear confirms RedactKeys() with no arguments clears the set:
// a subsequent With on a previously blacklisted key stores the raw value.
func TestRedactKeysClear(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	aerr.RedactKeys() // clear

	err := aerr.Message("m").With("password", secret).Err(nil)
	e, _ := aerr.AsAerr(err)
	if got := e.Attributes()["password"]; got != secret {
		t.Errorf("password = %#v, want raw %q after clearing", got, secret)
	}
}

// TestRedactKeysNoRetroWrap documents the ordering rule: attributes attached
// before RedactKeys runs are not retroactively wrapped.
func TestRedactKeysNoRetroWrap(t *testing.T) {
	t.Cleanup(func() { aerr.RedactKeys() })

	// Built while the set is empty: the value stays raw.
	err := aerr.Message("m").With("password", secret).Err(nil)

	aerr.RedactKeys("password") // installed too late to affect the error above

	e, _ := aerr.AsAerr(err)
	if got := e.Attributes()["password"]; got != secret {
		t.Errorf("password = %#v, want raw %q (no retro-wrap)", got, secret)
	}
}

// TestRedactKeysSurvivesWrap checks that an inner error built under the
// blacklist keeps its wrapped attribute after an outer Wrap merges it in.
func TestRedactKeysSurvivesWrap(t *testing.T) {
	aerr.RedactKeys("password")
	t.Cleanup(func() { aerr.RedactKeys() })

	inner := aerr.Code("IN").With("password", secret).ErrMsg("inner")
	outer := aerr.Message("outer").Wrap(inner)
	e, _ := aerr.AsAerr(outer)

	r, ok := e.Attributes()["password"].(aerr.Redacted)
	if !ok {
		t.Fatalf("merged attr is %T, want aerr.Redacted (mask survives merge)", e.Attributes()["password"])
	}
	if r.Value() != secret {
		t.Errorf("Value() = %v, want the original recovered", r.Value())
	}
	if got := fmt.Sprintf("%+v", e); strings.Contains(got, secret) {
		t.Errorf("secret leaked after Wrap: %s", got)
	}
}

// TestRedactKeysConcurrent exercises the atomic swap under -race: worker
// goroutines build errors via With while a mutator flips the set between a
// blacklist and cleared. There is no torn state — each observed value is
// either the raw canary or a Redacted that renders the placeholder.
func TestRedactKeysConcurrent(t *testing.T) {
	t.Cleanup(func() { aerr.RedactKeys() })

	const workers = 8
	const iters = 2000

	// Mutator: swap the set between present and cleared until the workers
	// finish. Its own lifetime is tracked separately so the worker Wait
	// below never blocks on it.
	stop := make(chan struct{})
	mutatorDone := make(chan struct{})
	go func() {
		defer close(mutatorDone)
		for {
			select {
			case <-stop:
				return
			default:
				aerr.RedactKeys("password")
				aerr.RedactKeys()
			}
		}
	}()

	// Workers: build errors and assert every observed value is consistent.
	var workerWg sync.WaitGroup
	for w := 0; w < workers; w++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for i := 0; i < iters; i++ {
				err := aerr.Message("m").With("password", secret).Err(nil)
				e, _ := aerr.AsAerr(err)
				switch v := e.Attributes()["password"].(type) {
				case string:
					if v != secret {
						t.Errorf("torn raw value: %q", v)
						return
					}
				case aerr.Redacted:
					if v.Value() != secret {
						t.Errorf("torn wrapped value: %v", v.Value())
						return
					}
				default:
					t.Errorf("unexpected value type %T", v)
					return
				}
			}
		}()
	}

	workerWg.Wait()
	close(stop)
	<-mutatorDone
}
