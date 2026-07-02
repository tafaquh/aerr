// Package aerr provides structured errors that carry an error code, a
// human-readable message, ordered key/value attributes, and an optional
// stack trace.
//
// An issued error is built once with a fluent [Builder] and then read many
// times. The [Error] type implements error, slog.LogValuer, json.Marshaler,
// and fmt.Formatter, so a single value logs cleanly through log/slog,
// marshals to JSON, and prints with %+v — while remaining fully compatible
// with errors.Is, errors.As, and errors.Unwrap.
//
// # Building errors
//
// Start a chain with [Code], [Message], [Messagef], or [StackTrace], attach
// attributes with [Builder.With], and finalize with [Builder.Err] (or
// [Builder.Wrap] to wrap an existing error):
//
//	err := aerr.Code("DB_ERROR").
//		Message("failed to query user").
//		StackTrace().
//		With("user_id", "123").
//		With("table", "users").
//		Err(dbErr)
//
//	slog.Error("request failed", slog.Any("err", err))
//
// Stack capture is opt-in: it happens only when StackTrace() is requested,
// and at most once per chain (see below).
//
// # Chain merging
//
// When errors are wrapped, aerr flattens the chain into one value:
//
//   - Message: the outer message and the full cause message are joined
//     with ": ", so the rendered message reads outermost-first.
//   - Code: the outermost code wins; an inner code is inherited only when
//     the outer builder set none.
//   - Attributes: outer attributes win; inner attributes are appended when
//     their keys are not already present, preserving order.
//   - Stack trace: the deepest stack wins. If the wrapped chain already
//     carries a trace it is inherited and an outer StackTrace() is a no-op,
//     so each chain captures at most once and traces point at the origin.
//
// Metadata is absorbed from the nearest inner [Error] in the chain even
// through non-aerr wrappers such as fmt.Errorf with %w.
//
// # Concurrency
//
// An issued *Error is immutable and safe to log from multiple goroutines;
// its stack trace is rendered lazily and cached race-free. A [Builder] is
// reusable as a template after finalizing (its state is copied into each
// issued error) but is not safe for concurrent use.
//
// # Logging adapters
//
// The core module depends only on the standard library. Optional logging
// adapters live in separate modules with their own dependencies:
//
//   - github.com/tafaquh/aerr/zerolog — github.com/rs/zerolog integration
//   - github.com/tafaquh/aerr/zap — go.uber.org/zap integration
package aerr
