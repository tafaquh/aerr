package aerr

import (
	"fmt"
	"io"
	"log/slog"
)

// RedactedText is the placeholder every render path emits in place of a
// [Redacted] value.
const RedactedText = "[REDACTED]"

// redactedJSON is RedactedText as a JSON string literal. Derived from the
// constant so the two cannot drift.
const redactedJSON = `"` + RedactedText + `"`

// Redact wraps v so every rendering — JSON, slog, and fmt — emits
// [RedactedText] while the original stays available in-process via
// [Redacted.Value]. Wrapping is a value copy with no allocation, and the
// result is comparable when v is. Redact does not flatten: Redact(Redact(v))
// nests, and Value then returns the inner Redacted.
func Redact(v any) Redacted {
	return Redacted{value: v}
}

// Redacted masks an attribute value on every render path aerr and its
// adapters use: json.Marshaler, fmt.Formatter, fmt.Stringer, and
// slog.LogValuer all resolve to [RedactedText], so plaintext never reaches
// a log buffer.
//
// value stays unexported deliberately: reflection-based encoders that skip
// unexported fields (encoding/xml, gopkg.in/yaml, mapstructure) see an
// empty struct and cannot leak the original either. The zero value and
// Redact(nil) are valid — both render as RedactedText and report a nil
// Value.
type Redacted struct {
	value any
}

// Value returns the original wrapped value for in-process use. It is the
// only way to recover the plaintext; every rendering path masks it.
func (r Redacted) Value() any {
	return r.value
}

// String implements fmt.Stringer, returning RedactedText. Format masks
// every verb, so this exists only for libraries that assert Stringer
// directly.
func (r Redacted) String() string {
	return RedactedText
}

// Format implements fmt.Formatter, writing RedactedText for every verb
// (%v, %+v, %#v, %s, %q, %d, %x, ...) and ignoring width and flags. This
// is the leak-closer fmt.Stringer alone cannot be: %#v prints a struct's
// unexported fields and the numeric verbs print the raw value, both
// bypassing String, whereas fmt.Formatter takes precedence for every verb.
func (r Redacted) Format(s fmt.State, _ rune) {
	// A fmt.State write has no error-return channel to the fmt caller, so
	// the result is intentionally discarded (see format.go for the same
	// convention).
	_, _ = io.WriteString(s, RedactedText)
}

// LogValue implements slog.LogValuer, resolving to RedactedText so a
// Redacted attribute logs masked through any slog handler.
func (r Redacted) LogValue() slog.Value {
	return slog.StringValue(RedactedText)
}

// MarshalJSON implements json.Marshaler, emitting RedactedText as a JSON
// string. It never fails and never panics, so it is safe on every JSON
// render path — including aerr's panic-guarded attribute encoding and the
// nested-marshaler resolution encoding/json performs for maps and structs.
func (r Redacted) MarshalJSON() ([]byte, error) {
	return []byte(redactedJSON), nil
}
