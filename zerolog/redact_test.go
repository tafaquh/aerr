package aerrzerolog_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

// canary is a distinctive plaintext secret. It must never appear in any
// rendered output: if a leak regresses, grepping for this literal catches it.
const canary = "s3cr3t-canary"

// reflectRedacted has the exact json.Marshaler output of aerr.Redacted but
// is a different type, so appendAttr renders it through the Interface
// reflection fallback — the pre-fast-path route the new case aerr.Redacted
// branch replaces. Comparing the two proves the fast path is byte-identical
// to reflection.
type reflectRedacted struct{}

func (reflectRedacted) MarshalJSON() ([]byte, error) {
	return []byte(`"` + aerr.RedactedText + `"`), nil
}

// redactMixedErr builds an error whose attributes interleave a string, the
// secret-carrying value, and an int, pinning field ordering through the
// adapter. secret is either an aerr.Redacted (fast path) or a reflectRedacted
// (reflection route).
func redactMixedErr(secret any) error {
	return aerr.Code("SEC").
		Message("boom").
		With("user", "alice").
		With("password", secret).
		With("attempt", 3).
		Err(nil)
}

// TestZerologRedactedFastPathMatchesReflection asserts the typed fast path
// emits bytes identical to the reflection route, through both the
// Register()-installed Err() path and the Object() helper. zerolog writes no
// timestamp by default, so the whole log line compares byte-for-byte.
func TestZerologRedactedFastPathMatchesReflection(t *testing.T) {
	withFreshRegister(t)

	cases := []struct {
		name   string
		render func(secret any) []byte
	}{
		{"Err", func(secret any) []byte {
			var buf bytes.Buffer
			logger := zerolog.New(&buf)
			logger.Error().Err(redactMixedErr(secret)).Msg("boom")
			return append([]byte(nil), buf.Bytes()...)
		}},
		{"Object", func(secret any) []byte {
			var buf bytes.Buffer
			logger := zerolog.New(&buf)
			logger.Error().Object("error", aerrzerolog.Object(redactMixedErr(secret))).Msg("boom")
			return append([]byte(nil), buf.Bytes()...)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fast := tc.render(aerr.Redact(canary))
			reflected := tc.render(reflectRedacted{})

			if !bytes.Equal(fast, reflected) {
				t.Errorf("fast path not byte-identical to reflection route:\nfast:   %s\nreflect:%s", fast, reflected)
			}
			if !bytes.Contains(fast, []byte(`"password":"`+aerr.RedactedText+`"`)) {
				t.Errorf("masked attribute missing from output:\n%s", fast)
			}
			// Ordering pin: the string precedes the mask precedes the int.
			if !bytes.Contains(fast, []byte(`"user":"alice","password":"`+aerr.RedactedText+`","attempt":3`)) {
				t.Errorf("attribute ordering not preserved:\n%s", fast)
			}
		})
	}
}

// TestZerologRedactedLeakCanary asserts the plaintext never reaches output on
// any adapter path.
func TestZerologRedactedLeakCanary(t *testing.T) {
	withFreshRegister(t)
	err := redactMixedErr(aerr.Redact(canary))

	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	logger.Error().Err(err).Msg("boom")
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through Err:\n%s", buf.String())
	}

	buf.Reset()
	logger.Error().Object("error", aerrzerolog.Object(err)).Msg("boom")
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through Object:\n%s", buf.String())
	}
}

// TestZerologRedactedDirectToLogger documents the research finding that a
// bare aerr.Redacted masks even outside aerr's adapters: zerolog's
// .Interface resolves it via InterfaceMarshalFunc (json.Marshal ->
// json.Marshaler), so a mis-plumbed secret still cannot leak.
func TestZerologRedactedDirectToLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	logger.Error().Interface("secret", aerr.Redact(canary)).Msg("x")

	var event map[string]any
	if jerr := json.Unmarshal(buf.Bytes(), &event); jerr != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", jerr, buf.String())
	}
	if got := event["secret"]; got != aerr.RedactedText {
		t.Errorf(".Interface(Redact) = %v, want %q", got, aerr.RedactedText)
	}
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through .Interface:\n%s", buf.String())
	}
}

// BenchmarkObjectRedactedFastPath measures the typed fast path: rendering a
// Redacted attribute through the adapter. ReportAllocs is the primary,
// hardware-independent evidence.
func BenchmarkObjectRedactedFastPath(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	err := aerr.Code("SEC").Message("boom").With("password", aerr.Redact(canary)).Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Object("err", aerrzerolog.Object(err)).Msg("x")
	}
}

// BenchmarkObjectRedactedReflection renders the same shape through the
// Interface route (reflectRedacted), so the allocs saved by the fast path
// show up as the delta between the two benchmarks.
func BenchmarkObjectRedactedReflection(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	err := aerr.Code("SEC").Message("boom").With("password", reflectRedacted{}).Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Object("err", aerrzerolog.Object(err)).Msg("x")
	}
}
