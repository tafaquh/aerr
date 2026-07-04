# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.0] - 2026-07-05

### Added

- `Frames() []Frame` returns the captured stack as structured
  `{File, Line, Function}` records for exporters such as Sentry and
  OpenTelemetry, alongside the rendered `Traces() []string`.
- `HasCode(err, code)` reports whether any aerr layer in a chain carries a
  code, checking every layer individually (including through `errors.Join`
  trees) rather than only the inherited outer code.
- `fmt.Formatter` support on `*Error`: `%s`/`%v`/`%q` render the combined
  message, and `%+v` prints a multi-line detail block with code,
  attributes, and stack (pkg/errors convention).
- `MarshalJSON` on `*Error`, producing the same `{code, message,
  attributes, stacktrace}` shape as the log integrations, never failing on
  unmarshalable or typed-nil attribute values.
- Printf-style constructors: `Messagef`, `Errorf`, and `Wrapf`, plus
  `(*Builder).Messagef`.
- New zap adapter module `github.com/tafaquh/aerr/zap` with `Field(err)`
  (a drop-in for `zap.Error` that renders aerr errors as a nested object,
  falls back to `zap.Error` for plain errors, and to `zap.Skip` on nil) and
  `Object(err)` for callers choosing their own key. No process globals.
- zerolog adapter gains `Register()` (installs aerr rendering into
  `zerolog.ErrorMarshalFunc`, chaining to any previously configured
  marshaler for non-aerr errors) and `Object(err)` for zero-global,
  per-call rendering.
- Package-level godoc (`doc.go`) and this changelog.

### Changed

- Stack-trace lines now use the editor-clickable format
  `file:line (function)` instead of the previous
  `file.(function):line`.
- Frame filtering is by file path: standard-library frames (runtime,
  testing, net/http, encoding/json, ...) and aerr internals are dropped,
  while user code is always kept regardless of how its module path is
  spelled (slashless local module paths included).
- Stack capture is opt-in via `StackTrace()`; errors do not capture a
  trace unless asked.
- Chain merging is now deterministic and documented: messages join with
  `": "`, the outermost code wins (inheriting an inner code only when
  unset), attributes merge outer-first, and metadata is absorbed from the
  nearest inner `*Error` even through `fmt.Errorf` `%w` wrappers. Captured
  stacks are capped at 32 frames.

### Fixed

- `Wrap`/`Err` no longer panic on typed-nil error values held in
  attributes or causes; such values render as `<nil>`/`null`.
- The deepest stack in a chain is now always the one kept: when the
  wrapped chain already carries a trace it is inherited and an outer
  `StackTrace()` is a no-op, so traces point at the origin and are never
  duplicated.
- The zerolog adapter module now compiles standalone. Its `v1.0.0` tag —
  which required a parent-module pre-release API that no longer existed and
  was hidden behind a `replace` directive consumers do not inherit — is
  retracted.
- `MarshalJSON` no longer panics on an attribute whose `json.Marshaler`
  implementation panics; such values degrade to a `"<panic: ...>"`
  placeholder, matching the guard already applied to panicking `Error()`
  values and both logging adapters.
- Stack-trace frame filtering stays correct under `-trimpath` builds:
  standard-library frames from multi-segment packages (`net/http`,
  `encoding/json`, …) are no longer leaked into rendered traces when the
  stdlib source anchor cannot be resolved.
- The zap adapter degrades an unencodable attribute value (a channel or
  func the JSON encoder rejects) to its `fmt` form instead of aborting the
  log object, so the remaining attributes and the stacktrace still render.
- Corrected the `aerr` package doc comment, which wrongly attributed the
  zerolog integration to `slog.LogValuer` and omitted the zap adapter;
  `doc.go` is now the single canonical package comment.

### Performance

- Rendered stack traces are computed once and cached on the `*Error`
  (race-safe), so repeated logging of the same error symbolizes PCs only
  once.
- The zerolog adapter writes attributes through zerolog's typed appenders
  and the stack through a typed `[]string` field, cutting the 10-field
  logging path from ~20 allocations to zero; error-chain logging is now
  zero-allocation as well.
- The zap adapter writes through zapcore's typed appenders, avoiding the
  reflection path for common attribute types.

### Deprecated

- `AerrStackMarshaler` in the zerolog adapter: the stack is already
  rendered inside the error object, so installing it and calling `.Stack()`
  would duplicate the trace. `Register` no longer installs it.

### Internal

- Test coverage raised to 99.6%, including fuzz tests for the merge and
  JSON paths.
- CI, linting, and dependabot configuration added.

[Unreleased]: https://github.com/tafaquh/aerr/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/tafaquh/aerr/compare/v1.0.0...v1.1.0
