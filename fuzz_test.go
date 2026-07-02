package aerr_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
)

// FuzzWrapMessageJoin exercises the message-join rule through the public
// API: for non-empty, ": "-free messages, wrapping one aerr in another must
// produce exactly "outer: inner".
func FuzzWrapMessageJoin(f *testing.F) {
	seeds := []struct{ outer, inner string }{
		{"outer", "inner"},
		{"a", "b"},
		{"database connection failed", "connection refused"},
		{"read", "unexpected EOF"},
	}
	for _, s := range seeds {
		f.Add(s.outer, s.inner)
	}

	f.Fuzz(func(t *testing.T, msgOuter, msgInner string) {
		if msgOuter == "" || msgInner == "" {
			t.Skip("empty side is covered by fixed cases")
		}
		if strings.Contains(msgOuter, ": ") || strings.Contains(msgInner, ": ") {
			t.Skip("':'-joined inputs make the concatenation ambiguous")
		}

		inner := aerr.Message(msgInner).Err(nil)
		outer := aerr.Message(msgOuter).Wrap(inner)

		want := msgOuter + ": " + msgInner
		if outer.Error() != want {
			t.Errorf("Error() = %q, want %q", outer.Error(), want)
		}
	})
}

// TestMessageJoinEmptySides covers the empty-side join rules that the fuzz
// target skips, using the same public API.
func TestMessageJoinEmptySides(t *testing.T) {
	cases := []struct {
		name     string
		outerMsg string
		innerMsg string
		want     string
	}{
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

// TestWrapChainProperties builds aerr chains of N layers and checks three
// invariants across every depth: the combined message has exactly N ": "
// segments (N+1 with a leaf non-aerr cause), errors.Is reaches every layer,
// and HasCode finds every layer's code.
func TestWrapChainProperties(t *testing.T) {
	for _, withLeaf := range []bool{false, true} {
		for n := 1; n <= 8; n++ {
			name := fmt.Sprintf("n=%d/leaf=%v", n, withLeaf)
			t.Run(name, func(t *testing.T) {
				var leaf error
				layers := make([]error, 0, n)

				// Innermost layer (index 0).
				var err error
				if withLeaf {
					leaf = errors.New("leaf")
					err = aerr.Code("C0").Message("m0").Err(leaf)
				} else {
					err = aerr.Code("C0").Message("m0").Err(nil)
				}
				layers = append(layers, err)

				// Wrap outward, keeping distinct codes and ":"-free messages.
				for i := 1; i < n; i++ {
					err = aerr.
						Code(fmt.Sprintf("C%d", i)).
						Message(fmt.Sprintf("m%d", i)).
						Wrap(err)
					layers = append(layers, err)
				}

				// Segment count.
				segs := strings.Split(err.Error(), ": ")
				wantSegs := n
				if withLeaf {
					wantSegs = n + 1
				}
				if len(segs) != wantSegs {
					t.Errorf("Error()=%q has %d segments, want %d", err.Error(), len(segs), wantSegs)
				}

				// errors.Is reaches every aerr layer.
				for i, layer := range layers {
					if !errors.Is(err, layer) {
						t.Errorf("errors.Is must reach layer %d", i)
					}
				}
				// ...and the leaf non-aerr cause when present.
				if withLeaf && !errors.Is(err, leaf) {
					t.Error("errors.Is must reach the leaf cause")
				}

				// HasCode finds every layer's code.
				for i := 0; i < n; i++ {
					code := fmt.Sprintf("C%d", i)
					if !aerr.HasCode(err, code) {
						t.Errorf("HasCode must find layer code %q", code)
					}
				}
			})
		}
	}
}
