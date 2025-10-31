# aerr

Simple error logging with stack traces for Go.

## Features

- **Automatic stack trace capture** - Every error captures where it was created with function names
- **Simplified error output** - Single code, combined message, merged attributes
- **Structured logging** - Stack traces as structured data, not strings
- **Method chaining** - Fluent API for both errors and logging
- **slog integration** - First-class support for Go's structured logging
- **High performance** - Optimized to minimize allocations
- **Zero dependencies** - Only uses Go standard library

## Installation

```bash
go get github.com/tafaquh/aerr
```

## Quick Start

```go
package main

import (
    "errors"
    "log/slog"
    "os"
    "github.com/tafaquh/aerr"
)

func main() {
    // Setup default slog logger
    slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

    // Create error with builder pattern
    err := aerr.Code("DB_ERROR").
        Message("failed to query user").
        StackTrace().
        With("user_id", "123").
        With("table", "users").
        Err(errors.New("connection timeout"))

    // Just use slog.Error - it automatically structures everything!
    slog.Error("operation failed", slog.Any("err", err))
}
```

Output:
```json
{
  "time": "2025-10-31T09:07:57Z",
  "level": "ERROR",
  "msg": "operation failed",
  "err": {
    "code": "DB_ERROR",
    "message": "failed to query user: connection timeout",
    "attributes": {
      "user_id": "123",
      "table": "users"
    },
    "stacktrace": [
      "/path/to/main.go.(main.main):34",
      "/usr/local/go/src/runtime/proc.go.(runtime.main):250"
    ]
  }
}
```

## Usage

### Builder Pattern

Create rich errors with the fluent builder API:

```go
err := aerr.Code("USER_NOT_FOUND").
    Message("failed to find user").
    StackTrace().
    With("user_id", userID).
    With("query", query).
    Err(dbErr)

// Then just log with slog
slog.Error("database operation failed", slog.Any("err", err))
```

### Start with Code or Message

```go
// Start with error code
err := aerr.Code("VALIDATION_ERROR").
    Message("invalid email format").
    With("email", email).
    Err(nil)

// Or start with message
err := aerr.Message("database query failed").
    Code("DB_ERROR").
    With("query", sql).
    Err(dbErr)
```

### Error Wrapping - Simplified Output

When you wrap errors, they're combined into a **single simplified structure**:

```go
// Layer 1: Database error
dbErr := aerr.Code("DB_ERROR").
    Message("database query failed").
    StackTrace().
    With("query", "SELECT * FROM users").
    Err(errors.New("connection timeout"))

// Layer 2: Repository wraps database error
repoErr := aerr.Code("REPOSITORY_ERROR").
    Message("failed to find user in repository").
    With("user_id", "12345").
    Wrap(dbErr)

// Layer 3: Service wraps repository error
serviceErr := aerr.Code("SERVICE_ERROR").
    Message("user service failed").
    With("operation", "GetUser").
    Wrap(repoErr)

slog.Error("request failed", slog.Any("err", serviceErr))
```

Output shows **simplified structure**:
```json
{
  "err": {
    "code": "SERVICE_ERROR",
    "message": "user service failed: failed to find user in repository: database query failed: connection timeout",
    "attributes": {
      "operation": "GetUser",
      "user_id": "12345",
      "query": "SELECT * FROM users"
    },
    "stacktrace": [
      "/path/to/database.go.(main.QueryDatabase):42",
      "/path/to/repository.go.(main.FindUser):28"
    ]
  }
}
```

**Key benefits:**
- **Single code**: Shows the outermost/top-level error code
- **Combined message**: All error messages joined with `: ` for easy reading
- **Merged attributes**: All fields from the error chain in one object
- **Deepest stacktrace**: Shows where the error originated

### Control Stack Traces

```go
// With stack trace (captures automatically with function names)
err := aerr.Code("ERR001").
    Message("something failed").
    StackTrace().
    Err(cause)

// Without stack trace (don't call StackTrace())
err := aerr.Code("ERR002").
    Message("validation failed").
    Err(nil)
```

### Stack Trace Format

Stack traces use the format `file_path.(package.function):line`:

```json
"stacktrace": [
  "/home/user/project/database.go.(main.QueryDatabase):75",
  "/home/user/project/repository.go.(main.FindUserRepository):52",
  "/home/user/project/service.go.(main.GetUserService):39",
  "/home/user/project/handler.go.(main.HandleUserRequest):26",
  "/home/user/project/services/service.go.(github.com/user/project/services.(*Service).HandleRequest):142"
]
```

## API

### Builder Pattern
- `Code(code string) *aerr` - Start with error code
- `Message(msg string) *aerr` - Start with message
- `(*aerr).Code(code) *aerr` - Set error code
- `(*aerr).Message(msg) *aerr` - Set message
- `(*aerr).With(key, val) *aerr` - Add field
- `(*aerr).StackTrace() *aerr` - Enable stack capture (by default stack is not captured)
- `(*aerr).Err(cause error) error` - Build error with cause
- `(*aerr).Wrap(err error) error` - Wrap another error (creates chain)

## Complete Example - Multi-Layer Application

### Using slog (default)

```go
package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/tafaquh/aerr"
)

func main() {
	// Setup slog default logger
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// Simulate an API request
	err := HandleUserRequest("12345")
	if err != nil {
		slog.Error("request failed", slog.Any("err", err))
	}
}

// HandleUserRequest simulates a controller/handler layer
func HandleUserRequest(userID string) error {
	err := GetUserService(userID)
	if err != nil {
		return aerr.Code("CONTROLLER_ERROR").
			Message("failed to handle user request").
			With("endpoint", "/api/users/"+userID).
			With("method", "GET").
			Wrap(err)
	}
	return nil
}

// GetUserService simulates a service layer
func GetUserService(userID string) error {
	err := FindUserRepository(userID)
	if err != nil {
		return aerr.Code("SERVICE_ERROR").
			Message("user service failed").
			With("service", "UserService").
			With("operation", "GetUser").
			Wrap(err)
	}
	return nil
}

// FindUserRepository simulates a repository layer
func FindUserRepository(userID string) error {
	err := QueryDatabase("SELECT * FROM users WHERE id = ?", userID)
	if err != nil {
		return aerr.Code("REPOSITORY_ERROR").
			Message("failed to find user in repository").
			StackTrace().
			With("user_id", userID).
			With("table", "users").
			Wrap(err)
	}
	return nil
}

// QueryDatabase simulates a database layer
func QueryDatabase(query string, args ...any) error {
	// Simulate a database connection error
	dbErr := errors.New("connection timeout after 5s")

	return aerr.Code("DB_ERROR").
		Message("database query failed").
		StackTrace().
		With("query", query).
		With("args", args).
		With("driver", "postgres").
		Err(dbErr)
}
```

Output shows **simplified structure** with all context:
```json
{
  "time": "2025-10-31T11:21:32.150424577+07:00",
  "level": "ERROR",
  "msg": "request failed",
  "err": {
    "code": "CONTROLLER_ERROR",
    "message": "failed to handle user request: user service failed: failed to find user in repository: database query failed: connection timeout after 5s",
    "attributes": {
      "endpoint": "/api/users/12345",
      "method": "GET",
      "operation": "GetUser",
      "service": "UserService",
      "user_id": "12345",
      "table": "users",
      "query": "SELECT * FROM users WHERE id = ?",
      "args": ["12345"],
      "driver": "postgres"
    },
    "stacktrace": [
      "/home/user/project/main.go.(main.QueryDatabase):75",
      "/home/user/project/main.go.(main.FindUserRepository):52",
      "/home/user/project/main.go.(main.GetUserService):39",
      "/home/user/project/main.go.(main.HandleUserRequest):26",
      "/home/user/project/main.go.(main.main):18"
    ]
  }
}
```

### Using zerolog

```go
package main

import (
	"errors"
	"os"

	"github.com/rs/zerolog"
	"github.com/tafaquh/aerr"
	_ "github.com/tafaquh/aerr/zerolog" // Import to enable aerr integration
)

func main() {
	// Setup zerolog logger
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Simulate an API request
	err := HandleUserRequest("12345")
	if err != nil {
		// Standard zerolog API - just works!
		logger.Error().Stack().Err(err).Msg("request failed")
	}
}

// (Same HandleUserRequest, GetUserService, FindUserRepository, QueryDatabase functions as above)
```

> **Note**: You can run both examples from the `examples/` directory:
> - `go run main.go` for slog output
> - `go run main.go -zerolog` for zerolog output

Output with **zerolog** (same simplified structure):
```json
{
  "level": "error",
  "err": {
    "code": "CONTROLLER_ERROR",
    "message": "failed to handle user request: user service failed: failed to find user in repository: database query failed: connection timeout after 5s",
    "attributes": {
      "service": "UserService",
      "user_id": "12345",
      "table": "users",
      "query": "SELECT * FROM users WHERE id = ?",
      "args": ["12345"],
      "driver": "postgres",
      "endpoint": "/api/users/12345",
      "method": "GET",
      "operation": "GetUser"
    },
    "stacktrace": [
      "/home/user/project/main.go.(main.QueryDatabase):101",
      "/home/user/project/main.go.(main.FindUserRepository):78",
      "/home/user/project/main.go.(main.GetUserService):65",
      "/home/user/project/main.go.(main.HandleUserRequest):52",
      "/home/user/project/main.go.(main.runWithZerolog):44",
      "/home/user/project/main.go.(main.main):20"
    ]
  },
  "time": "2025-10-31T16:01:45+07:00",
  "message": "request failed"
}
```

## Why aerr?

**Simple** - Builder pattern that chains naturally with your code.

**Automatic** - Implements `slog.LogValuer` for automatic structured logging.

**Simplified Output** - Single code, combined messages, merged attributes. Much easier to read!

**Rich Stack Traces** - Compact format with full function paths for quick debugging.

**High Performance** - Optimized to minimize allocations. Faster than logrus and go-kit.

**Flexible** - Add error codes, custom fields, and control stack traces.

**Compatible** - Works with standard `errors.Is()`, `errors.As()`, and `errors.Unwrap()`.

## Zerolog Integration

aerr provides optional zerolog integration through a separate package for high-performance logging.

### Installation

```bash
go get github.com/tafaquh/aerr/zerolog
```

### Usage

```go
import (
    "github.com/rs/zerolog"
    "github.com/tafaquh/aerr"
    _ "github.com/tafaquh/aerr/zerolog" // Import to enable aerr integration
)

func main() {
    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    err := aerr.Code("DB_ERROR").
        Message("query failed").
        StackTrace().
        With("user_id", "123").
        Err(nil)

    // Just use standard zerolog API!
    logger.Error().Stack().Err(err).Msg("operation failed")
}
```

## Performance Benchmarks

All benchmarks run on Intel Core Ultra 9 185H with 3 second benchmark time.

### slog Integration (Default)

```
BenchmarkDisabled-4                      	140M	  26.38 ns/op	  48 B/op	   1 allocs/op
BenchmarkSimpleError-4                   	1.5M	  2373 ns/op	1481 B/op	  14 allocs/op
BenchmarkSimpleErrorWithStack-4          	1.5M	  2291 ns/op	1497 B/op	  14 allocs/op
BenchmarkErrorWith10Fields-4             	838K	  4416 ns/op	2650 B/op	  39 allocs/op
BenchmarkErrorWith10FieldsAndStack-4     	806K	  4325 ns/op	2682 B/op	  39 allocs/op
BenchmarkErrorChain-4                    	841K	  4177 ns/op	2282 B/op	  31 allocs/op
BenchmarkErrorChainDeep-4                	1.0M	  3552 ns/op	2170 B/op	  29 allocs/op
BenchmarkErrorCreation-4                 	2.9M	  1223 ns/op	 960 B/op	   6 allocs/op
BenchmarkErrorCreationWithStack-4        	3.0M	  1269 ns/op	 960 B/op	   6 allocs/op
```

### Logger Comparison Benchmarks

**Native Loggers (baseline):**
```
# Zerolog (fastest)
BenchmarkZerologSimple-4                 	66M	 58.81 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologWith10Fields-4           	17M	 216.5 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologErrorChain-4             	5.3M	 639.9 ns/op	 296 B/op	   7 allocs/op

# Zap (very fast, structured)
BenchmarkZapSimple-4                     	16M	 228.1 ns/op	   0 B/op	   0 allocs/op
BenchmarkZapWith10Fields-4               	5.4M	 664.5 ns/op	 704 B/op	   1 allocs/op
BenchmarkZapErrorChain-4                 	4.1M	 892.1 ns/op	 640 B/op	   6 allocs/op

# Logrus (mature, flexible)
BenchmarkLogrusSimple-4                  	3.3M	  1081 ns/op	 872 B/op	  19 allocs/op
BenchmarkLogrusWith10Fields-4            	887K	  4197 ns/op	3979 B/op	  52 allocs/op
BenchmarkLogrusErrorChain-4              	952K	  3394 ns/op	2661 B/op	  45 allocs/op

# slog (standard library)
BenchmarkSimpleError-4                   	1.6M	  2282 ns/op	1481 B/op	  14 allocs/op
BenchmarkErrorWith10Fields-4             	802K	  4315 ns/op	2650 B/op	  39 allocs/op
BenchmarkErrorChain-4                    	878K	  4113 ns/op	2282 B/op	  31 allocs/op
```

**Aerr with Zerolog:**
```
BenchmarkAerrZerologSimple-4             	2.2M	  1606 ns/op	1449 B/op	  15 allocs/op
BenchmarkAerrZerologWith10Fields-4       	1.0M	  3558 ns/op	2425 B/op	  38 allocs/op
BenchmarkAerrZerologErrorChain-4         	1.3M	  2779 ns/op	2073 B/op	  30 allocs/op
```

### Comparison Table

**Simple Error:**
| Logger | Time | Bytes | Allocs | vs Fastest |
|--------|------|-------|--------|------------|
| Zerolog | 58.81 ns | 0 B | 0 | **baseline** (fastest) |
| Zap | 228.1 ns | 0 B | 0 | 3.9x slower |
| Logrus | 1081 ns | 872 B | 19 | 18x slower |
| Aerr + Zerolog | 1606 ns | 1449 B | 15 | **27x slower** |
| Aerr + slog | 2282 ns | 1481 B | 14 | 39x slower |

**10 Fields:**
| Logger | Time | Bytes | Allocs | vs Fastest |
|--------|------|-------|--------|------------|
| Zerolog | 216.5 ns | 0 B | 0 | **baseline** (fastest) |
| Zap | 664.5 ns | 704 B | 1 | 3.1x slower |
| Aerr + Zerolog | 3558 ns | 2425 B | 38 | **16x slower** |
| Logrus | 4197 ns | 3979 B | 52 | 19x slower |
| Aerr + slog | 4315 ns | 2650 B | 39 | 20x slower |

**Error Chain (3 levels with fields):**
| Logger | Time | Bytes | Allocs | vs Fastest |
|--------|------|-------|--------|------------|
| Zerolog | 639.9 ns | 296 B | 7 | **baseline** (fastest) |
| Zap | 892.1 ns | 640 B | 6 | 1.4x slower |
| Aerr + Zerolog | 2779 ns | 2073 B | 30 | **4.3x slower** |
| Logrus | 3394 ns | 2661 B | 45 | 5.3x slower |
| Aerr + slog | 4113 ns | 2282 B | 31 | 6.4x slower |

**Key Insights:**
- **Zerolog** is the fastest logger, with zero allocations for simple cases
- **Zap** is close behind, also with excellent performance and zero allocs for simple cases
- **Logrus** is slower but very mature and flexible
- **Aerr adds structured error management** (automatic attribute merging, code tracking, stack trace propagation) at the cost of 4-27x overhead depending on complexity
- **Error chains show better relative performance** - aerr is only 4.3x slower vs 27x for simple errors, because the structured features become more valuable with complexity

**Note:** All error chain benchmarks use `fmt.Errorf` with `%w` for proper error wrapping, but fields must be added manually. aerr provides automatic attribute merging from the entire error chain, code tracking, and stack trace propagation.

### Performance Analysis

**When to use each logger:**

- **Zerolog**: Maximum performance (50-640 ns/op), zero allocations for simple cases, manual field management
- **Zap**: Excellent performance (228-892 ns/op), structured logging, production-ready
- **Logrus**: Mature ecosystem (1081-4197 ns/op), flexible, good for existing projects
- **slog**: Standard library (2282-4315 ns/op), no external dependencies, built-in Go support
- **Aerr + Zerolog**: Structured error chains (1606-2779 ns/op) with automatic context merging, best for complex error handling
- **Aerr + slog**: Standard library with structured errors (2282-4113 ns/op), good balance of features and compatibility

**Trade-offs:**
- aerr adds overhead for rich error context (codes, messages, attributes, stack traces)
- Error chain simplification (combining messages, merging attributes) has a cost
- Stack trace capture and formatting adds ~1-2 µs per error
- The convenience of automatic error structuring comes with performance cost
- **More complex operations show better relative performance** - aerr is only 4.3x slower for error chains vs 27x for simple errors
- Native loggers require manual field management and don't provide structured error wrapping features that aerr offers

**Recommendation:**
- Use **zerolog or zap** for hot paths where every nanosecond counts
- Use **logrus** if you're already invested in its ecosystem
- Use **slog** for standard library compatibility without external dependencies
- Use **aerr + zerolog** when you need structured error chains with automatic attribute merging and good performance
- Use **aerr + slog** for standard library integration with structured error management
- **Note:** The overhead is more noticeable for simple operations but becomes relatively smaller for complex error chains

### Run Benchmarks

```bash
# Navigate to benchmarks directory
cd benchmarks

# All benchmarks
go test -bench=. -benchmem -benchtime=3s

# Compare all loggers (simple, 10 fields, error chain)
go test -bench="Simple|With10Fields|ErrorChain" -benchmem -benchtime=3s

# Only zerolog comparisons
go test -bench="Zerolog|AerrZerolog" -benchmem -benchtime=3s

# Only zap benchmarks
go test -bench="Zap" -benchmem -benchtime=3s

# Only logrus benchmarks
go test -bench="Logrus" -benchmem -benchtime=3s

# Only slog benchmarks (aerr with slog)
go test -bench="^Benchmark(Simple|Error|Disabled)" -benchmem -benchtime=3s
```

## License

MIT
