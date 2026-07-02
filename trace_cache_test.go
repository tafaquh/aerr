package aerr_test

import (
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/tafaquh/aerr"
)

func TestTracesCachedAndStable(t *testing.T) {
	err := aerr.Code("X").StackTrace().ErrMsg("boom")
	e, ok := aerr.AsAerr(err)
	if !ok {
		t.Fatal("expected aerr")
	}

	first := e.Traces()
	if len(first) == 0 {
		t.Fatal("expected traces")
	}
	second := e.Traces()
	if len(second) != len(first) {
		t.Fatalf("Traces() unstable across calls: %d vs %d frames", len(first), len(second))
	}
	if &first[0] != &second[0] {
		t.Error("Traces() re-rendered instead of returning the cached slice")
	}
}

func TestConcurrentLoggingOfSharedError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	err := aerr.Code("SHARED").
		Message("shared error").
		StackTrace().
		With("k", "v").
		Err(nil)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				logger.Error("event", slog.Any("err", err))
			}
		}()
	}
	wg.Wait() // must be race-free under -race
}
