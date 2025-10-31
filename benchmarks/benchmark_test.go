package aerr_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
	"github.com/tafaquh/aerr"
	_ "github.com/tafaquh/aerr/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// BenchmarkDisabled tests logging when disabled (should be very fast)
func BenchmarkDisabled(b *testing.B) {
	logger := slog.New(slog.NewJSONHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Don't call StackTrace() for benchmark without stack
	err := aerr.Message("test error").Err(nil)

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

	// Don't call StackTrace() for simple error without stack
	err := aerr.Message("test error").Err(nil)

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

	// Don't call StackTrace() for benchmark without stack
	err := aerr.Code("TEST_ERROR").
		Message("test error").
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
		// Don't call StackTrace() for benchmark without stack
		_ = aerr.Code("TEST_ERROR").
			Message("test error").
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
// Uses fmt.Errorf with %w for proper error wrapping, but fields are added manually
func BenchmarkZerologErrorChain(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create error chain each iteration to match aerr's behavior
		err1 := errors.New("connection timeout")
		err2 := fmt.Errorf("database query failed: %w", err1)
		err3 := fmt.Errorf("user service failed: %w", err2)

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

	// Don't call StackTrace() for benchmark without stack
	err := aerr.Code("TEST_ERROR").
		Message("test error").
		Err(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error().Err(err).Msg("test")
	}
}

// BenchmarkAerrZerologWith10Fields tests aerr with zerolog and 10 fields
func BenchmarkAerrZerologWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	// Don't call StackTrace() for benchmark without stack
	err := aerr.Code("TEST_ERROR").
		Message("test error").
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
		logger.Error().Err(err).Msg("test")
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
		logger.Error().Err(err).Msg("test")
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
		logger.Error().Err(err3).Msg("request failed")
	}
}

// ==================== Logrus Comparison Benchmarks ====================

// BenchmarkLogrusSimple tests logrus with a simple error message
func BenchmarkLogrusSimple(b *testing.B) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("test error")
	}
}

// BenchmarkLogrusWith10Fields tests logrus with 10 fields
func BenchmarkLogrusWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.WithFields(logrus.Fields{
			"field1":  "value1",
			"field2":  "value2",
			"field3":  "value3",
			"field4":  "value4",
			"field5":  "value5",
			"field6":  "value6",
			"field7":  "value7",
			"field8":  "value8",
			"field9":  "value9",
			"field10": "value10",
		}).Error("test error")
	}
}

// BenchmarkLogrusErrorChain tests logrus with error wrapping (3 levels)
func BenchmarkLogrusErrorChain(b *testing.B) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err1 := errors.New("connection timeout")
		err2 := fmt.Errorf("database query failed: %w", err1)
		err3 := fmt.Errorf("user service failed: %w", err2)

		buf.Reset()
		logger.WithFields(logrus.Fields{
			"error":    err3,
			"query":    "SELECT * FROM users",
			"table":    "users",
			"operation": "GetUser",
			"user_id":  "12345",
			"endpoint": "/api/users/12345",
			"method":   "GET",
		}).Error("request failed")
	}
}

// ==================== Zap Comparison Benchmarks ====================

// BenchmarkZapSimple tests zap with a simple error message
func BenchmarkZapSimple(b *testing.B) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.ErrorLevel)
	logger := zap.New(core)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("test error")
	}
}

// BenchmarkZapWith10Fields tests zap with 10 fields
func BenchmarkZapWith10Fields(b *testing.B) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.ErrorLevel)
	logger := zap.New(core)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		logger.Error("test error",
			zap.String("field1", "value1"),
			zap.String("field2", "value2"),
			zap.String("field3", "value3"),
			zap.String("field4", "value4"),
			zap.String("field5", "value5"),
			zap.String("field6", "value6"),
			zap.String("field7", "value7"),
			zap.String("field8", "value8"),
			zap.String("field9", "value9"),
			zap.String("field10", "value10"),
		)
	}
}

// BenchmarkZapErrorChain tests zap with error wrapping (3 levels)
func BenchmarkZapErrorChain(b *testing.B) {
	var buf bytes.Buffer
	encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	core := zapcore.NewCore(encoder, zapcore.AddSync(&buf), zapcore.ErrorLevel)
	logger := zap.New(core)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err1 := errors.New("connection timeout")
		err2 := fmt.Errorf("database query failed: %w", err1)
		err3 := fmt.Errorf("user service failed: %w", err2)

		buf.Reset()
		logger.Error("request failed",
			zap.Error(err3),
			zap.String("query", "SELECT * FROM users"),
			zap.String("table", "users"),
			zap.String("operation", "GetUser"),
			zap.String("user_id", "12345"),
			zap.String("endpoint", "/api/users/12345"),
			zap.String("method", "GET"),
		)
	}
}
