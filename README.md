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

// Without stack trace
err := aerr.Code("ERR002").
    Message("validation failed").
    WithoutStack().
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
- `(*aerr).StackTrace() *aerr` - Enable stack capture
- `(*aerr).WithoutStack() *aerr` - Disable stack capture
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
BenchmarkDisabled-4                      	128M	  29.01 ns/op	  48 B/op	   1 allocs/op
BenchmarkSimpleError-4                   	5.8M	 565.3 ns/op	 208 B/op	   2 allocs/op
BenchmarkSimpleErrorWithStack-4          	3.0M	  1242 ns/op	 856 B/op	   8 allocs/op
BenchmarkErrorWith10Fields-4             	1.0M	  3049 ns/op	1848 B/op	  29 allocs/op
BenchmarkErrorWith10FieldsAndStack-4     	926K	  3759 ns/op	2497 B/op	  35 allocs/op
BenchmarkErrorChain-4                    	1.0M	  3000 ns/op	1881 B/op	  26 allocs/op
BenchmarkErrorChainDeep-4                	1.2M	  2913 ns/op	1865 B/op	  25 allocs/op
BenchmarkErrorCreation-4                 	 27M	 129.6 ns/op	 432 B/op	   3 allocs/op
BenchmarkErrorCreationWithStack-4        	2.8M	  1259 ns/op	 960 B/op	   6 allocs/op
```

### Zerolog Integration Benchmarks

**Native Zerolog (baseline):**
```
BenchmarkZerologDisabled-4               	 1B	  1.627 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologSimple-4                 	74M	 50.05 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologWith10Fields-4           	19M	 192.1 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologWith10FieldsAndStack-4   	17M	 211.1 ns/op	   0 B/op	   0 allocs/op
BenchmarkZerologErrorChain-4             	20M	 178.2 ns/op	   0 B/op	   0 allocs/op
```

**Aerr with Zerolog:**
```
BenchmarkAerrZerologSimple-4             	22M	 152.3 ns/op	  32 B/op	   2 allocs/op
BenchmarkAerrZerologWith10Fields-4       	1.9M	  1916 ns/op	1816 B/op	  26 allocs/op
BenchmarkAerrZerologWith10FieldsAndStack-4	1.4M	  2536 ns/op	2481 B/op	  32 allocs/op
BenchmarkAerrZerologErrorChain-4         	1.4M	  2598 ns/op	2682 B/op	  29 allocs/op
```

### Comparison Table

**Simple Error:**
| Implementation | Time | Bytes | Allocs | vs Native |
|---------------|------|-------|--------|-----------|
| Native Zerolog | 50.05 ns | 0 B | 0 | baseline |
| Aerr + Zerolog | 152.3 ns | 32 B | 2 | **3x slower** |
| Aerr + slog | 565.3 ns | 208 B | 2 | 11x slower |

**10 Fields:**
| Implementation | Time | Bytes | Allocs | vs Native |
|---------------|------|-------|--------|-----------|
| Native Zerolog | 192.1 ns | 0 B | 0 | baseline |
| Aerr + Zerolog | 1916 ns | 1816 B | 26 | **10x slower** |
| Aerr + slog | 3049 ns | 1848 B | 29 | 16x slower |

**10 Fields + Stack:**
| Implementation | Time | Bytes | Allocs | vs Native |
|---------------|------|-------|--------|-----------|
| Native Zerolog | 211.1 ns | 0 B | 0 | baseline |
| Aerr + Zerolog | 2536 ns | 2481 B | 32 | **12x slower** |
| Aerr + slog | 3759 ns | 2497 B | 35 | 18x slower |

**Error Chain (3 levels):**
| Implementation | Time | Bytes | Allocs | vs Native |
|---------------|------|-------|--------|-----------|
| Native Zerolog | 178.2 ns | 0 B | 0 | baseline |
| Aerr + Zerolog | 2598 ns | 2682 B | 29 | **15x slower** |
| Aerr + slog | 3000 ns | 1881 B | 26 | 17x slower |

### Performance Analysis

**When to use each:**

- **Native Zerolog**: Maximum performance, zero allocations, manual field management
- **Aerr + Zerolog**: Structured error chains with automatic context merging, ~10x overhead
- **Aerr + slog**: Standard library integration, similar performance to Aerr + Zerolog

**Trade-offs:**
- aerr adds overhead for rich error context (codes, messages, attributes, stack traces)
- Error chain simplification (combining messages, merging attributes) has a cost
- Stack trace capture and formatting adds ~1-2 µs per error
- The convenience of automatic error structuring comes with performance cost

**Recommendation:**
- Use **native zerolog** for hot paths where every nanosecond counts
- Use **aerr + zerolog** when you want structured error chains with excellent performance
- Use **aerr + slog** for standard library compatibility and similar performance

### Run Benchmarks

```bash
# Navigate to benchmarks directory
cd benchmarks

# All benchmarks
go test -bench=. -benchmem -benchtime=3s

# Only zerolog comparisons
go test -bench="Zerolog|AerrZerolog" -benchmem -benchtime=3s

# Only slog benchmarks
go test -bench="^Benchmark[^Z]" -benchmem -benchtime=3s
```

## License

MIT
