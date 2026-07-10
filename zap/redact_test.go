package aerrzap_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tafaquh/aerr"
	aerrzap "github.com/tafaquh/aerr/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// canary is a distinctive plaintext secret. It must never appear in any
// rendered output: if a leak regresses, grepping for this literal catches it.
const canary = "s3cr3t-canary"

// reflectRedacted has the exact json.Marshaler output of aerr.Redacted but
// is a different type, so addAttr renders it through the AddReflected
// reflection fallback — the pre-fast-path route the new case aerr.Redacted
// branch replaces. Comparing the two proves the fast path is byte-identical
// to reflection.
type reflectRedacted struct{}

func (reflectRedacted) MarshalJSON() ([]byte, error) {
	return []byte(`"` + aerr.RedactedText + `"`), nil
}

// newJSONLoggerNoTime is newJSONLogger with the per-entry timestamp
// suppressed, so two independent log lines differing only in one attribute
// value can be compared byte-for-byte.
func newJSONLoggerNoTime() (*zap.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = ""
	encoder := zapcore.NewJSONEncoder(cfg)
	core := zapcore.NewCore(encoder, zapcore.AddSync(buf), zapcore.ErrorLevel)
	return zap.New(core), buf
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

// TestZapRedactedFastPathMatchesReflection asserts the typed fast path emits
// bytes identical to the reflection route, through both Field() and Object().
func TestZapRedactedFastPathMatchesReflection(t *testing.T) {
	cases := []struct {
		name  string
		field func(err error) zap.Field
	}{
		{"Field", func(err error) zap.Field { return aerrzap.Field(err) }},
		{"Object", func(err error) zap.Field { return zap.Object("error", aerrzap.Object(err)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			render := func(secret any) []byte {
				logger, buf := newJSONLoggerNoTime()
				logger.Error("x", tc.field(redactMixedErr(secret)))
				return append([]byte(nil), buf.Bytes()...)
			}

			fast := render(aerr.Redact(canary))
			reflected := render(reflectRedacted{})

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

// TestZapRedactedLeakCanary asserts the plaintext never reaches output on any
// adapter path.
func TestZapRedactedLeakCanary(t *testing.T) {
	err := redactMixedErr(aerr.Redact(canary))

	logger, buf := newJSONLogger()
	logger.Error("x", aerrzap.Field(err))
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through Field:\n%s", buf.String())
	}

	buf.Reset()
	logger.Error("x", zap.Object("error", aerrzap.Object(err)))
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through Object:\n%s", buf.String())
	}
}

// TestZapRedactedDirectToLogger documents the research finding that a bare
// aerr.Redacted masks even outside aerr's adapters: zap.Any resolves it via
// its fmt.Stringer precedence (or the reflection json.Marshaler fallback),
// so a mis-plumbed secret still cannot leak.
func TestZapRedactedDirectToLogger(t *testing.T) {
	logger, buf := newJSONLogger()
	logger.Error("x", zap.Any("secret", aerr.Redact(canary)))

	line := decodeLine(t, buf)
	if got := line["secret"]; got != aerr.RedactedText {
		t.Errorf("zap.Any(Redact) = %v, want %q", got, aerr.RedactedText)
	}
	if strings.Contains(buf.String(), canary) {
		t.Errorf("canary leaked through zap.Any:\n%s", buf.String())
	}
}

// BenchmarkFieldRedactedFastPath measures the typed fast path: rendering a
// Redacted attribute through the adapter. ReportAllocs is the primary,
// hardware-independent evidence.
func BenchmarkFieldRedactedFastPath(b *testing.B) {
	logger, buf := newJSONLogger()
	err := aerr.Code("SEC").Message("boom").With("password", aerr.Redact(canary)).Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("x", aerrzap.Field(err))
	}
}

// BenchmarkFieldRedactedReflection renders the same shape through the
// AddReflected route (reflectRedacted), so the allocs saved by the fast path
// show up as the delta between the two benchmarks.
func BenchmarkFieldRedactedReflection(b *testing.B) {
	logger, buf := newJSONLogger()
	err := aerr.Code("SEC").Message("boom").With("password", reflectRedacted{}).Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("x", aerrzap.Field(err))
	}
}
