//go:build !race

// The race detector makes sync.Pool drop entries at random, so zerolog's
// pooled events allocate under -race; this guard runs in the plain test runs
// (including CI's consumer-smoke job) instead.

package aerrzerolog_test

import (
	"errors"
	"io"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
)

// TestBareErrorLogsWithoutAllocating guards the documented zero-allocation
// path: logging a bare *aerr.Error through Register must not allocate, so
// the work done for wrapped and aggregated errors stays off it.
func TestBareErrorLogsWithoutAllocating(t *testing.T) {
	withFreshRegister(t)
	logger := zerolog.New(io.Discard)
	err := aerr.Code("DB").Message("query failed").With("table", "users").With("attempt", 3).Err(errors.New("timeout"))

	if n := testing.AllocsPerRun(200, func() { logger.Error().Err(err).Msg("x") }); n != 0 {
		t.Errorf("logging a bare *aerr.Error allocated %v times per call, want 0", n)
	}
}
