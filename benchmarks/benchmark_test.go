package aerr_test

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

// BenchmarkDisabled tests logging when disabled (should be very fast)
func BenchmarkDisabled(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err := aerr.Message("test error").WithoutStack().Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug("debug message", slog.Any("err", err))
	}
}

// BenchmarkSimpleError tests a simple error with no fields
func BenchmarkSimpleError(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err := aerr.Message("test error").WithoutStack().Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("error message", slog.Any("err", err))
	}
}

// BenchmarkSimpleErrorWithStack tests a simple error with stack trace
func BenchmarkSimpleErrorWithStack(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err := aerr.Message("test error").StackTrace().Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("error message", slog.Any("err", err))
	}
}

// BenchmarkErrorWith10Fields tests an error with 10 fields (like zerolog/zap benchmarks)
func BenchmarkErrorWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		WithoutStack().
		With("field1", "value1").
		With("field2", "value2").
		With("field3", "value3").
		With("field4", "value4").
		With("field5", "value5").
		With("field6", "value6").
		With("field7", "value7").
		With("field8", "value8").
		With("field9", "value9").
		With("field10", "value10").
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("error message", slog.Any("err", err))
	}
}

// BenchmarkErrorWith10FieldsAndStack tests an error with 10 fields and stack trace
func BenchmarkErrorWith10FieldsAndStack(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		StackTrace().
		With("field1", "value1").
		With("field2", "value2").
		With("field3", "value3").
		With("field4", "value4").
		With("field5", "value5").
		With("field6", "value6").
		With("field7", "value7").
		With("field8", "value8").
		With("field9", "value9").
		With("field10", "value10").
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("error message", slog.Any("err", err))
	}
}

// BenchmarkErrorChain tests a chain of 3 errors
func BenchmarkErrorChain(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err1 := aerr.Code("DB_ERROR").
		Message("database query failed").
		StackTrace().
		With("query", "SELECT * FROM users").
		With("table", "users").
		Err(errors.New("connection timeout"))

	err2 := aerr.Code("SERVICE_ERROR").
		Message("user service failed").
		With("operation", "GetUser").
		With("user_id", "12345").
		Wrap(err1)

	err3 := aerr.Code("CONTROLLER_ERROR").
		Message("failed to handle user request").
		With("endpoint", "/api/users/12345").
		With("method", "GET").
		Wrap(err2)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("request failed", slog.Any("err", err3))
	}
}

// BenchmarkErrorChainDeep tests a chain of 5 errors
func BenchmarkErrorChainDeep(b *testing.B) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	err1 := aerr.Code("ERROR1").Message("error 1").StackTrace().With("field1", "value1").Err(nil)
	err2 := aerr.Code("ERROR2").Message("error 2").With("field2", "value2").Wrap(err1)
	err3 := aerr.Code("ERROR3").Message("error 3").With("field3", "value3").Wrap(err2)
	err4 := aerr.Code("ERROR4").Message("error 4").With("field4", "value4").Wrap(err3)
	err5 := aerr.Code("ERROR5").Message("error 5").With("field5", "value5").Wrap(err4)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("error occurred", slog.Any("err", err5))
	}
}

// BenchmarkErrorCreation tests just error creation (no logging)
func BenchmarkErrorCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = aerr.Code("TEST_ERROR").
			Message("test error").
			WithoutStack().
			With("field1", "value1").
			With("field2", "value2").
			Err(nil)
	}
}

// BenchmarkErrorCreationWithStack tests error creation with stack trace
func BenchmarkErrorCreationWithStack(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = aerr.Code("TEST_ERROR").
			Message("test error").
			StackTrace().
			With("field1", "value1").
			With("field2", "value2").
			Err(nil)
	}
}

// ==================== Zerolog Comparison Benchmarks ====================

// BenchmarkZerologDisabled tests zerolog with disabled logging
func BenchmarkZerologDisabled(b *testing.B) {
	logger := zerolog.New(io.Discard).Level(zerolog.ErrorLevel)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Debug().Msg("debug message")
	}
}

// BenchmarkZerologSimple tests zerolog with a simple error message
func BenchmarkZerologSimple(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Msg("test error")
	}
}

// BenchmarkZerologWith10Fields tests zerolog with 10 fields
func BenchmarkZerologWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().
			Str("field1", "value1").
			Str("field2", "value2").
			Str("field3", "value3").
			Str("field4", "value4").
			Str("field5", "value5").
			Str("field6", "value6").
			Str("field7", "value7").
			Str("field8", "value8").
			Str("field9", "value9").
			Str("field10", "value10").
			Msg("test error")
	}
}

// BenchmarkZerologWith10FieldsAndStack tests zerolog with 10 fields and stack trace
func BenchmarkZerologWith10FieldsAndStack(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := errors.New("test error")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().
			Stack().
			Err(err).
			Str("field1", "value1").
			Str("field2", "value2").
			Str("field3", "value3").
			Str("field4", "value4").
			Str("field5", "value5").
			Str("field6", "value6").
			Str("field7", "value7").
			Str("field8", "value8").
			Str("field9", "value9").
			Str("field10", "value10").
			Msg("test error")
	}
}

// BenchmarkZerologErrorChain tests zerolog with error wrapping (3 levels)
func BenchmarkZerologErrorChain(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err1 := errors.New("connection timeout")
	err2 := errors.New("database query failed: " + err1.Error())
	err3 := errors.New("user service failed: " + err2.Error())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().
			Err(err3).
			Str("query", "SELECT * FROM users").
			Str("table", "users").
			Str("operation", "GetUser").
			Str("user_id", "12345").
			Str("endpoint", "/api/users/12345").
			Str("method", "GET").
			Msg("request failed")
	}
}

// ==================== Aerr with Zerolog Benchmarks ====================

// BenchmarkAerrZerologSimple tests aerr with zerolog simple error
func BenchmarkAerrZerologSimple(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		WithoutStack().
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err)).Msg("test")
	}
}

// BenchmarkAerrZerologWith10Fields tests aerr with zerolog and 10 fields
func BenchmarkAerrZerologWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		WithoutStack().
		With("field1", "value1").
		With("field2", "value2").
		With("field3", "value3").
		With("field4", "value4").
		With("field5", "value5").
		With("field6", "value6").
		With("field7", "value7").
		With("field8", "value8").
		With("field9", "value9").
		With("field10", "value10").
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err)).Msg("test")
	}
}

// BenchmarkAerrZerologWith10FieldsAndStack tests aerr with zerolog, 10 fields and stack
func BenchmarkAerrZerologWith10FieldsAndStack(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err := aerr.Code("TEST_ERROR").
		Message("test error").
		StackTrace().
		With("field1", "value1").
		With("field2", "value2").
		With("field3", "value3").
		With("field4", "value4").
		With("field5", "value5").
		With("field6", "value6").
		With("field7", "value7").
		With("field8", "value8").
		With("field9", "value9").
		With("field10", "value10").
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err)).Msg("test")
	}
}

// BenchmarkAerrZerologErrorChain tests aerr with zerolog error chain (3 levels)
func BenchmarkAerrZerologErrorChain(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	err1 := aerr.Code("DB_ERROR").
		Message("database query failed").
		StackTrace().
		With("query", "SELECT * FROM users").
		With("table", "users").
		Err(errors.New("connection timeout"))

	err2 := aerr.Code("SERVICE_ERROR").
		Message("user service failed").
		With("operation", "GetUser").
		With("user_id", "12345").
		Wrap(err1)

	err3 := aerr.Code("CONTROLLER_ERROR").
		Message("failed to handle user request").
		With("endpoint", "/api/users/12345").
		With("method", "GET").
		Wrap(err2)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Interface("err", aerrzerolog.AerrMarshaller(err3)).Msg("request failed")
	}
}
