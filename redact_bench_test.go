package aerr_test

import (
	"testing"

	"github.com/tafaquh/aerr"
)

// benchBuilder is a package-level sink so the benchmarked With calls below
// cannot be optimized away.
var benchBuilder *aerr.Builder

// BenchmarkWith_RedactDisabled measures With with no redaction set installed:
// the disabled fast path is a single atomic load plus a nil check, so its
// alloc count is the With baseline the other two are compared against.
func BenchmarkWith_RedactDisabled(b *testing.B) {
	aerr.RedactKeys()
	b.Cleanup(func() { aerr.RedactKeys() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBuilder = aerr.Message("m").With("password", "hunter2")
	}
}

// BenchmarkWith_RedactMiss measures With when a set is installed but the key
// is not in it. This should match the disabled baseline: a map lookup miss
// adds no allocation.
func BenchmarkWith_RedactMiss(b *testing.B) {
	aerr.RedactKeys("other")
	b.Cleanup(func() { aerr.RedactKeys() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBuilder = aerr.Message("m").With("password", "hunter2")
	}
}

// BenchmarkWith_RedactHit measures With when the key is in the set: the value
// is wrapped in Redact and boxed into the attribute's any, the one extra
// allocation the disabled-path delta documents.
func BenchmarkWith_RedactHit(b *testing.B) {
	aerr.RedactKeys("password")
	b.Cleanup(func() { aerr.RedactKeys() })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchBuilder = aerr.Message("m").With("password", "hunter2")
	}
}
