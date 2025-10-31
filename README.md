# aerr

Simple error logging with stack traces for Go.

## Features

- **Automatic stack trace capture** - Every error captures where it was created with function names
- **Flattened error chains** - Error chains as arrays instead of deeply nested objects
- **Structured logging** - Stack traces as structured data, not strings
- **Method chaining** - Fluent API for both errors and logging
- **slog integration** - First-class support for Go's structured logging
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
    "message": "failed to query user",
    "code": "DB_ERROR",
    "error": "connection timeout",
    "data": {
      "user_id": "123",
      "table": "users"
    },
    "stacktrace": [
      "main.main (/path/to/file.go:34)",
      "runtime.main (/path/to/runtime/proc.go:250)"
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

### Error Wrapping - Flattened Chains

When you wrap errors, they're stored as a **flat array** instead of deeply nested objects:

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

Output shows **flat array** instead of nested objects:
```json
{
  "err": {
    "errors": [
      {
        "code": "SERVICE_ERROR",
        "message": "user service failed",
        "data": {"operation": "GetUser"}
      },
      {
        "code": "REPOSITORY_ERROR",
        "message": "failed to find user in repository",
        "data": {"user_id": "12345"}
      },
      {
        "code": "DB_ERROR",
        "message": "database query failed",
        "data": {"query": "SELECT * FROM users"},
        "stacktrace": [
          "main.QueryDatabase (/path/to/db.go:42)",
          "main.FindUser (/path/to/repo.go:28)"
        ]
      },
      {
        "error": "connection timeout"
      }
    ]
  }
}
```

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

Stack traces include function names for easy debugging:

```json
"stacktrace": [
  "main.QueryDatabase (/home/user/project/db.go:75)",
  "main.FindUserRepository (/home/user/project/repo.go:52)",
  "main.GetUserService (/home/user/project/service.go:39)",
  "main.HandleUserRequest (/home/user/project/handler.go:26)"
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

Output shows all errors in a **flat array** with full context:
```json
{
  "time": "2025-10-31T11:21:32.150424577+07:00",
  "level": "ERROR",
  "msg": "request failed",
  "err": {
    "errors": [
      {
        "code": "CONTROLLER_ERROR",
        "data": {
          "endpoint": "/api/users/12345",
          "method": "GET"
        },
        "message": "failed to handle user request"
      },
      {
        "code": "SERVICE_ERROR",
        "data": {
          "operation": "GetUser",
          "service": "UserService"
        },
        "message": "user service failed"
      },
      {
        "code": "REPOSITORY_ERROR",
        "data": {
          "table": "users",
          "user_id": "12345"
        },
        "message": "failed to find user in repository"
      },
      {
        "code": "DB_ERROR",
        "data": {
          "args": ["12345"],
          "driver": "postgres",
          "query": "SELECT * FROM users WHERE id = ?"
        },
        "message": "database query failed",
        "stacktrace": [
          "main.QueryDatabase (/home/user/aerr/examples/nested_layers.go:75)",
          "main.FindUserRepository (/home/user/aerr/examples/nested_layers.go:52)",
          "main.GetUserService (/home/user/aerr/examples/nested_layers.go:39)",
          "main.HandleUserRequest (/home/user/aerr/examples/nested_layers.go:26)",
          "main.main (/home/user/aerr/examples/nested_layers.go:18)"
        ]
      },
      {
        "error": "connection timeout after 5s"
      }
    ]
  }
}
```

## Why aerr?

**Simple** - Builder pattern that chains naturally with your code.

**Automatic** - Implements `slog.LogValuer` for automatic structured logging.

**Flat Error Chains** - Error chains are arrays, not deeply nested objects. Much easier to read and process!

**Rich Stack Traces** - Includes function names in stack traces for quick debugging.

**Flexible** - Add error codes, custom fields, and control stack traces.

**Compatible** - Works with standard `errors.Is()`, `errors.As()`, and `errors.Unwrap()`.

## License

MIT
