//go:build !race

// The race detector makes sync.Pool drop entries at random, so zap's pooled
// encoders and entries allocate under -race; this guard runs in the plain
// test runs (including CI's consumer-smoke job) instead.

package aerrzap_test

import (
	"errors"
	"io"
	"testing"

	"github.com/tafaquh/aerr"
	aerrzap "github.com/tafaquh/aerr/zap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TestFieldRendersWithoutAllocating guards the adapter's own cost: building
// the Field and encoding a bare *aerr.Error — attributes and stack trace
// included — must not allocate. The fields slice is reused so the caller's
// variadic slice, which is zap's cost rather than the adapter's, stays out of
// the measurement.
func TestFieldRendersWithoutAllocating(t *testing.T) {
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zapcore.ErrorLevel)
	logger := zap.New(core)
	err := aerr.Code("DB").Message("query failed").StackTrace().
		With("table", "users").With("attempt", 3).Err(errors.New("timeout"))

	fields := make([]zap.Field, 1)
	if n := testing.AllocsPerRun(200, func() {
		fields[0] = aerrzap.Field(err)
		logger.Error("x", fields...)
	}); n != 0 {
		t.Errorf("rendering a bare *aerr.Error allocated %v times per call, want 0", n)
	}
}
