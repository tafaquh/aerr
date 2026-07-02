<div align="center">


<img src="./assets/aerr.png" alt="aerr" width="600"/>

**Lightweight error handling with structured logging and stack trace support for Go**

</div>

---

## Philosophy

Inspired by **"air"** (water in Indonesian), aerr embodies the natural flow of water from downstream to upstream. Just as water brings life along its journey, error information should flow seamlessly through your application layers—carrying vital context from where it originates to where it's resolved.

When errors flow smoothly with complete context (codes, messages, attributes, stack traces), engineers can diagnose and fix issues faster, bringing life back to their systems. aerr makes error tracking effortless, transforming debugging from a painful search into a clear path to resolution.

> *"Like water flowing upstream, errors should carry the important context they've collected along their journey."*

## Features

- **Structured errors** - An error code, a combined message, ordered key/value attributes, and an optional stack trace in one value.
- **Opt-in stack traces** - Capture is off by default and enabled per error with `StackTrace()`, so you pay for it only where you want it.
- **Automatic chain merging** - Wrapping flattens the chain: messages join, the outermost code wins, attributes merge, and the deepest stack is kept.
- **Structured logging** - Implements `slog.LogValuer`; stack traces are emitted as structured data, not opaque strings.
- **Multiple sinks, one value** - The same error prints with `%+v`, marshals with `json.Marshal`, and logs through slog, zerolog, or zap.
- **High performance** - Fields are structured once at creation, so repeated logging amortizes to zero allocations with the zerolog adapter.
- **Small dependency surface** - The core module uses only the standard library; the zerolog and zap adapters are separate modules with their own dependencies.

## Installation

```bash
go get github.com/tafaquh/aerr
```

Optional logging adapters (separate modules):

```bash
go get github.com/tafaquh/aerr/zerolog
go get github.com/tafaquh/aerr/zap
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

Output (keys appear in the order `message`, `code`, `attributes`, `stacktrace`):
```json
{
  "time": "2026-07-02T14:18:02Z",
  "level": "ERROR",
  "msg": "operation failed",
  "err": {
    "message": "failed to query user: connection timeout",
    "code": "DB_ERROR",
    "attributes": {
      "user_id": "123",
      "table": "users"
    },
    "stacktrace": [
      "/app/main.go:19 (main.main)"
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

A builder finalizes with `Err` (records a cause), `Wrap` (wraps another error, returning nil when it is nil), or `ErrMsg` (uses a plain string as the cause). Finalizing **copies** the builder's state into the issued error, so a builder can be kept and reused as a template. A `*Builder` is not safe for concurrent use; the issued `*Error` is immutable and safe to log from multiple goroutines (its trace rendering is cached race-free).

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

When you wrap errors, they are merged into a **single flat structure**:

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

The merge rules are deterministic:

- **Single code** — the outermost code wins; an inner code is inherited only when the outer builder set none.
- **Combined message** — the outer message and the full cause message are joined with `": "`, so it reads outermost-first (`user service failed: failed to find user in repository: database query failed: connection timeout`).
- **Merged attributes** — outer attributes win; inner attributes are appended when their key is not already present, preserving order.
- **Deepest stacktrace** — the trace from the origin is kept (see below).
- **Works through `%w`** — metadata (code, attributes, stack) is absorbed from the nearest inner `*Error` in the chain **even behind non-aerr wrappers** such as `fmt.Errorf("...: %w", inner)`.

### Control Stack Traces

Stack capture is **opt-in**. An error captures a trace only when `StackTrace()` is called on its builder:

```go
// With stack trace
err := aerr.Code("ERR001").
    Message("something failed").
    StackTrace().
    Err(cause)

// Without stack trace (don't call StackTrace())
err := aerr.Code("ERR002").
    Message("validation failed").
    Err(nil)
```

**The deepest stack wins.** This holds for both `Err` and `Wrap`: when the wrapped chain already carries a trace, it is inherited and an outer `StackTrace()` becomes a no-op. Each chain therefore captures at most once, and the trace always points at where the error originated. Captured stacks are capped at 32 frames.

### Stack Trace Format

Traces are rendered as `file:line (function)` — the leading `file:line` makes each entry clickable in most editors and terminals:

```json
"stacktrace": [
  "/app/repository/user.go:75 (github.com/acme/app/repository.(*UserRepo).Find)",
  "/app/service/user.go:52 (github.com/acme/app/service.(*UserService).Get)",
  "/app/main.go:26 (main.main)"
]
```

Frames are filtered so you only see your own code. Filtered out:

- **The Go standard library**, matched by source-file path (so `runtime`, `testing`, `net/http`, `encoding/json`, and friends never appear).
- **aerr's own internals.**

User code is **always** kept, regardless of how its module path is spelled — locally-developed modules with slashless (`main`, `myapp`) paths are included, not mistaken for stdlib.

## API

### Building an error

- `Code(code string) *Builder` — start with an error code
- `Message(msg string) *Builder` — start with a message
- `Messagef(format string, args ...any) *Builder` — start with a printf-style message
- `StackTrace() *Builder` — start with stack capture enabled
- `ErrMsg(msg string) error` — one-shot shortcut for `Message(msg).Err(nil)`
- `Errorf(format string, args ...any) error` — printf-style one-shot error
- `Wrapf(err error, format string, args ...any) error` — printf-style one-shot wrap (returns nil when err is nil)
- `(*Builder).Code(code) *Builder` — set the error code
- `(*Builder).Message(msg) *Builder` — set the message
- `(*Builder).Messagef(format, args...) *Builder` — set a printf-style message
- `(*Builder).StackTrace() *Builder` — enable stack capture (off by default)
- `(*Builder).With(key string, value any) *Builder` — add an attribute; re-using a key overwrites its value in place
- `(*Builder).Err(cause error) error` — finalize, optionally wrapping a cause
- `(*Builder).ErrMsg(msg string) error` — finalize with a plain-text cause
- `(*Builder).Wrap(err error) error` — finalize wrapping another error; returns nil if err is nil

A `*Builder` is not safe for concurrent use. Finalizing copies its state, so a builder may be reused as a template afterwards (from one goroutine). The returned `*Error` is immutable and safe to share or log from multiple goroutines.

### Inspecting an error

- `AsAerr(err error) (*Error, bool)` — extract an `*Error` from anywhere in a chain (including `errors.Join` trees); a typed-nil `*Error` does not count as a match
- `HasCode(err error, code string) bool` — check every aerr layer of a chain for a code
- `(*Error).Error() string` — the combined message
- `(*Error).Unwrap() error` — the wrapped cause (works with `errors.Is` / `errors.As`)
- `(*Error).Code() string` — the error code
- `(*Error).NumAttrs() int` — number of attributes
- `(*Error).RangeAttrs(func(key string, value any) bool)` — iterate attributes without allocating
- `(*Error).Attributes() map[string]any` — snapshot attributes as a freshly-allocated map
- `(*Error).Traces() []string` — the filtered stack trace (rendered once, cached)
- `(*Error).Frames() []Frame` — structured `{File, Line, Function}` frames for exporters

### Printing with `%+v`

`*Error` implements `fmt.Formatter`. `%s`, `%v`, and `%q` print the combined message; `%+v` prints a multi-line detail block:

```go
fmt.Printf("%+v\n", err)
```

```text
failed to query user: connection timeout
code: DB_ERROR
attributes:
    user_id=123
    table=users
stacktrace:
    /app/main.go:17 (main.build)
    /app/main.go:21 (main.main)
```

### Marshaling with `json.Marshal`

`*Error` implements `json.Marshaler`, producing the same shape the log integrations emit (empty fields omitted). Values implementing `error` render as their message, and values `encoding/json` rejects degrade to their `fmt` representation instead of failing the whole error:

```go
b, _ := json.Marshal(err) // err is an *aerr.Error
```

```json
{
  "code": "DB_ERROR",
  "message": "failed to query user: connection timeout",
  "attributes": {
    "user_id": "123",
    "table": "users"
  },
  "stacktrace": [
    "/app/main.go:17 (main.build)",
    "/app/main.go:21 (main.main)"
  ]
}
```

### Inspecting codes with `HasCode`

`HasCode` walks every aerr layer of a chain — including through `errors.Join` trees — so it sees codes that an outer error did not inherit:

```go
if aerr.HasCode(err, "DB_ERROR") {
    // some layer in the chain carried DB_ERROR
}
```

### Printf-style constructors

```go
aerr.Errorf("bad id %d", id)                     // one-shot error, no cause
aerr.Wrapf(cause, "op %s failed", op)            // one-shot wrap
aerr.Messagef("retry %d/%d", n, max).Code("X").Err(cause) // builder start
```

### Structured frames for exporters

`Frames()` returns the filtered stack as structured records for pushing into Sentry, OpenTelemetry, or any exporter that wants file/line/function separately rather than pre-rendered strings:

```go
type Frame struct {
    File     string
    Line     int
    Function string
}

if e, ok := aerr.AsAerr(err); ok {
    for _, f := range e.Frames() {
        span.RecordStackFrame(f.File, f.Line, f.Function)
    }
}
```

Unlike `Traces()`, the result is rebuilt on every call, so retain it rather than re-invoking in hot paths.

## Complete Example - Multi-Layer Application

The runnable program lives in [`examples/main.go`](examples/main.go). Run it in both modes:

```bash
cd examples
go run main.go            # slog output
go run main.go -zerolog   # zerolog output
```

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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

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

Output (attributes merge outer-first; the deepest stack — from `QueryDatabase` — is the one kept, so `FindUserRepository`'s `StackTrace()` is a no-op):
```json
{
  "time": "2026-07-02T14:15:47Z",
  "level": "ERROR",
  "msg": "request failed",
  "err": {
    "message": "failed to handle user request: user service failed: failed to find user in repository: database query failed: connection timeout after 5s",
    "code": "CONTROLLER_ERROR",
    "attributes": {
      "endpoint": "/api/users/12345",
      "method": "GET",
      "service": "UserService",
      "operation": "GetUser",
      "user_id": "12345",
      "table": "users",
      "query": "SELECT * FROM users WHERE id = ?",
      "args": ["12345"],
      "driver": "postgres"
    },
    "stacktrace": [
      "/app/examples/main.go:105 (main.QueryDatabase)",
      "/app/examples/main.go:82 (main.FindUserRepository)",
      "/app/examples/main.go:69 (main.GetUserService)",
      "/app/examples/main.go:56 (main.HandleUserRequest)",
      "/app/examples/main.go:33 (main.runWithSlog)",
      "/app/examples/main.go:22 (main.main)"
    ]
  }
}
```

### Using zerolog

Register the adapter once in `main`, then use the standard zerolog API. Note that `.Err(err)` writes under zerolog's default error key, `"error"`:

```go
package main

import (
	"os"

	"github.com/rs/zerolog"
	aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

func main() {
	// Enable aerr rendering once, from main (same convention as
	// zerolog's own pkgerrors helper).
	aerrzerolog.Register()

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	err := HandleUserRequest("12345")
	if err != nil {
		// Standard zerolog API - aerr renders the full structured payload.
		logger.Error().Err(err).Msg("request failed")
	}
}

// (Same HandleUserRequest / GetUserService / FindUserRepository / QueryDatabase as above)
```

Output (same merge; keys appear as `code` then `message`; the stack is already inside the `error` object, so calling `.Stack()` is unnecessary and would only duplicate it):
```json
{
  "level": "error",
  "error": {
    "code": "CONTROLLER_ERROR",
    "message": "failed to handle user request: user service failed: failed to find user in repository: database query failed: connection timeout after 5s",
    "attributes": {
      "endpoint": "/api/users/12345",
      "method": "GET",
      "service": "UserService",
      "operation": "GetUser",
      "user_id": "12345",
      "table": "users",
      "query": "SELECT * FROM users WHERE id = ?",
      "args": ["12345"],
      "driver": "postgres"
    },
    "stacktrace": [
      "/app/examples/main.go:105 (main.QueryDatabase)",
      "/app/examples/main.go:82 (main.FindUserRepository)",
      "/app/examples/main.go:69 (main.GetUserService)",
      "/app/examples/main.go:56 (main.HandleUserRequest)",
      "/app/examples/main.go:47 (main.runWithZerolog)",
      "/app/examples/main.go:20 (main.main)"
    ]
  },
  "time": "2026-07-02T14:15:48+07:00",
  "message": "request failed"
}
```

## Zerolog Integration

The zerolog adapter lives in the separate `github.com/tafaquh/aerr/zerolog` module.

```bash
go get github.com/tafaquh/aerr/zerolog
```

> **Note:** the published `v1.0.0` tag of this module is **retracted** — it did not compile standalone. Install with the next tag once released (`v1.1.0`).

There are two ways to use it, and neither requires calling `.Stack()`: an aerr error's trace is always rendered inside the error object.

**1. Register once (process-wide).** `Register()` assigns `zerolog.ErrorMarshalFunc`, chaining to whatever marshaler was active before it so non-aerr errors keep their previous rendering. Call it once from `main`, not from library code.

```go
import (
    "os"

    "github.com/rs/zerolog"
    "github.com/tafaquh/aerr"
    aerrzerolog "github.com/tafaquh/aerr/zerolog"
)

func main() {
    aerrzerolog.Register()

    logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

    err := aerr.Code("DB_ERROR").
        Message("query failed").
        StackTrace().
        With("user_id", "123").
        Err(nil)

    // Standard zerolog API - renders under the "error" key.
    logger.Error().Err(err).Msg("operation failed")
}
```

**2. Zero globals (per call).** Render a single error explicitly with `Object`, choosing your own key:

```go
logger.Error().Object("err", aerrzerolog.Object(err)).Msg("operation failed")
```

## Zap Integration

The zap adapter lives in the separate `github.com/tafaquh/aerr/zap` module.

```bash
go get github.com/tafaquh/aerr/zap
```

zap has no process-global error marshaler to register, so the integration is a field constructor — no globals, fully idiomatic zap.

**1. Drop-in field.** `Field(err)` renders under the key `"error"`: a nested object (code, message, attributes, stacktrace) when the chain carries an `*aerr.Error`, `zap.Error` otherwise, and `zap.Skip` (a no-op field) when `err` is nil. It is a safe drop-in wherever you would pass `zap.Error`.

```go
import (
    "go.uber.org/zap"
    aerrzap "github.com/tafaquh/aerr/zap"
)

logger.Error("request failed", aerrzap.Field(err))
```

**2. Your own key.** Compose the raw marshaler with `zap.Object`:

```go
logger.Error("request failed", zap.Object("err", aerrzap.Object(err)))
```

## Why aerr?

**Simple** - Builder pattern that chains naturally with your code.

**Automatic** - Implements `slog.LogValuer`, `json.Marshaler`, and `fmt.Formatter`, so one value logs, marshals, and prints.

**Flat output** - Single code, combined message, merged attributes, one stack. Much easier to read than a nested chain.

**Rich stack traces** - Editor-clickable `file:line (function)` frames with stdlib noise filtered out.

**High performance** - Fields are structured once at creation; repeated logging through the zerolog adapter is zero-allocation.

**Compatible** - Works with standard `errors.Is()`, `errors.As()`, and `errors.Unwrap()`.

## Performance Benchmarks

Measured on an Intel Core Ultra 9 185H (WSL2) with
`go test -bench=. -benchmem -benchtime=1s`. Numbers vary by machine; run
them yourself (see below).

### The core idea

aerr structures an error's fields **once, at creation**. A logging adapter
then replays that pre-structured payload, so **repeated logging of the same
error amortizes to zero allocations** with zerolog. The trade-off is paid up
front at creation, and once more the first time a stack-carrying error is
logged (a single trace render, then cached).

The benchmarks below log a **prebuilt** error repeatedly, which is exactly
the shape that benefits from this amortization. A raw logger, by contrast,
re-appends every field on **each** call — so with many fields or deep
chains, aerr + zerolog can end up *faster* than raw zerolog:

**Simple error (one field):**
| Logger         | Time      | B/op  | Allocs |
|----------------|-----------|-------|--------|
| Raw zerolog    | 72.7 ns   | 0     | 0      |
| aerr + zerolog | 119.3 ns  | 0     | 0      |
| Raw zap        | 368.8 ns  | 0     | 0      |
| aerr + zap     | 521.5 ns  | 64    | 1      |
| aerr + slog    | 720.6 ns  | 208   | 2      |
| logrus         | 1232 ns   | 873   | 19     |

**Ten fields:**
| Logger         | Time      | B/op  | Allocs |
|----------------|-----------|-------|--------|
| aerr + zerolog | 121.8 ns  | 0     | 0      |
| Raw zerolog    | 268.2 ns  | 0     | 0      |
| aerr + zap     | 892.6 ns  | 80    | 2      |
| Raw zap        | 1148 ns   | 705   | 1      |
| aerr + slog    | 2281 ns   | 624   | 3      |
| logrus         | 5180 ns   | 3985  | 52     |

With ten fields, **aerr + zerolog (122 ns) is faster than raw zerolog
(268 ns)** because the fields are already structured — zerolog re-encodes
them from scratch on every call.

**Error chain (3 levels with fields):**
| Logger         | Time      | B/op  | Allocs |
|----------------|-----------|-------|--------|
| aerr + zerolog | 167.5 ns  | 0     | 0      |
| Raw zerolog    | 676.1 ns  | 192   | 5      |
| aerr + zap     | 1006 ns   | 112   | 3      |
| Raw zap        | 1519 ns   | 641   | 6      |
| aerr + slog    | 2943 ns   | 665   | 6      |
| logrus         | 4802 ns   | 2665  | 45     |

On chains, aerr + zerolog is **~4× faster** than raw zerolog while doing the
chain merge (message join, code selection, attribute dedup, stack
propagation) for you. The raw-logger chain benchmarks build the chain with
`fmt.Errorf` + `%w` and append fields by hand.

### Full aerr numbers

**aerr + slog:**
```
BenchmarkDisabled                 25.75 ns/op     48 B/op    1 allocs/op
BenchmarkSimpleError             720.6  ns/op    208 B/op    2 allocs/op
BenchmarkSimpleErrorWithStack   1409    ns/op    424 B/op    5 allocs/op
BenchmarkErrorWith10Fields      2281    ns/op    624 B/op    3 allocs/op
BenchmarkErrorWith10Fields+Stack 3081   ns/op    841 B/op    6 allocs/op
BenchmarkErrorChain             2943    ns/op    665 B/op    6 allocs/op
BenchmarkErrorChainDeep         2143    ns/op    632 B/op    6 allocs/op
```

**aerr + zerolog (all zero-alloc):**
```
BenchmarkAerrZerologSimple             119.3 ns/op    0 B/op    0 allocs/op
BenchmarkAerrZerologWith10Fields       121.8 ns/op    0 B/op    0 allocs/op
BenchmarkAerrZerologWith10Fields+Stack 126.9 ns/op    0 B/op    0 allocs/op
BenchmarkAerrZerologErrorChain         167.5 ns/op    0 B/op    0 allocs/op
```

### Creation cost (the up-front price)

```
BenchmarkErrorCreation          146.7 ns/op    352 B/op    4 allocs/op
BenchmarkErrorCreationWithStack 805.5 ns/op    384 B/op    5 allocs/op
```

Creating an error costs ~147 ns / 4 allocs; adding `StackTrace()` raises
that to ~805 ns / 5 allocs (the cost of walking and copying the PCs). A
level-filtered (disabled) log is ~26 ns.

### When to use what

- **Raw zerolog / raw zap** — absolute lowest latency when you manage
  fields by hand and log each error exactly once. Best for the hottest,
  simplest paths.
- **aerr + zerolog** — structured errors with codes, chain merging, and
  stack traces, at zero allocations per log and *faster than raw zerolog*
  once an error carries several fields or a chain. Best default for rich
  error handling on hot paths.
- **aerr + slog** — standard-library-only structured error handling
  (no external logging dependency).
- **aerr + zap** — structured errors for existing zap codebases.
- **logrus** — only if you are already invested in its ecosystem.

Caveats worth stating plainly: error **creation** and the **first** log of a
stack-carrying error carry the amortized costs above, and code that builds an
error and logs it exactly once sees less of the repeated-logging benefit than
these benchmarks show.

### Run Benchmarks

```bash
cd benchmarks

# All benchmarks
go test -bench=. -benchmem -benchtime=1s

# Compare all loggers (simple, 10 fields, error chain)
go test -bench="Simple|With10Fields|ErrorChain" -benchmem -benchtime=1s

# Only zerolog comparisons
go test -bench="Zerolog|AerrZerolog" -benchmem -benchtime=1s

# Only zap benchmarks
go test -bench="Zap" -benchmem -benchtime=1s
```

## Compatibility

- **Core module (`github.com/tafaquh/aerr`)** — requires **Go 1.21** or
  newer.
- **Adapters (`.../zerolog`, `.../zap`)** — temporarily require a **1.24.7**
  toolchain: their `go.mod` `go` directive is pinned by the published aerr
  `v1.0.0`, and drops to 1.21 once they are rebuilt against the next aerr
  release.
- Works with the standard `errors.Is`, `errors.As`, and `errors.Unwrap`,
  including `errors.Join` trees.

## Versioning & Releases

This project follows [Semantic Versioning](https://semver.org/). Once a
version is tagged its public API is frozen under the
[Go 1 compatibility promise](https://go.dev/doc/go1compat) for the module's
major version.

Planned next tags:

- `aerr v1.1.0` — the core API documented here.
- `zerolog/v1.1.0` — activates the retraction of the broken `zerolog/v1.0.0`
  and requires `aerr v1.1.0`.
- `zap/v1.0.0` — the first working zap adapter release, after bumping its
  aerr requirement.

## License

MIT
