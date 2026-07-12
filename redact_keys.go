package aerr

import "sync/atomic"

// redactedKeys holds the process-global set of attribute keys whose values
// [Builder.With] wraps with [Redact] at attach time. A nil pointer means
// redaction is disabled, which keeps the With fast path to a single atomic
// load plus a nil check. A published map is never mutated: RedactKeys always
// swaps in a freshly built one, so a concurrent reader sees either the old or
// the new set whole, never a torn state.
var redactedKeys atomic.Pointer[map[string]struct{}]

// RedactKeys installs the process-global set of attribute keys whose values
// [Builder.With] wraps with [Redact] at attach time. Call it once from main
// before errors are created (the same convention as the zerolog adapter's
// Register); keys attached before RedactKeys runs are not retroactively
// wrapped. Matching is exact and case-sensitive. Calling RedactKeys with no
// arguments clears the set. Safe for concurrent use, though intended as
// startup configuration.
func RedactKeys(keys ...string) {
	if len(keys) == 0 {
		redactedKeys.Store(nil)
		return
	}
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	redactedKeys.Store(&set)
}

// redactValue applies the active RedactKeys set to a single attribute. When
// key is in the set and value is not already [Redacted], it returns
// Redact(value); otherwise it returns value unchanged. The disabled path —
// no set installed — is a single atomic load and a nil check.
func redactValue(key string, value any) any {
	set := redactedKeys.Load()
	if set == nil {
		return value
	}
	if _, ok := (*set)[key]; !ok {
		return value
	}
	if _, ok := value.(Redacted); ok {
		return value
	}
	return Redact(value)
}
