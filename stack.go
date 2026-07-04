package aerr

import (
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
)

const stackMaxDepth = 32

// selfPkgPrefix matches function names belonging to this package (e.g.
// "github.com/tafaquh/aerr.(*Builder).Err"). The trailing dot keeps
// subpackages and external test packages out of the match.
const selfPkgPrefix = "github.com/tafaquh/aerr."

// stdlibDir is the directory containing the standard library sources as
// reported by this binary's runtime, derived once from the location of a
// known stdlib function. Frames whose file lives under it are stdlib
// (runtime.*, testing.*, net/http, encoding/json, ...) and are filtered
// from rendered traces. Deriving the root from what the runtime itself
// reports avoids the deprecated runtime.GOROOT and works wherever Go is
// installed.
//
// Under -trimpath the runtime reports stdlib files by their relative path
// (e.g. "strings/strings.go"), so this anchor cannot be resolved and the
// result is ""; skipFrame then classifies frames by function name instead
// (see isStdlibFunc).
var stdlibDir = func() string {
	pc := reflect.ValueOf(strings.Contains).Pointer()
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return ""
	}
	file, _ := fn.FileLine(pc)
	// file is ".../src/strings/strings.go"; two dirs up is the src root.
	dir := path.Dir(path.Dir(file))
	if dir == "." || dir == "/" {
		return ""
	}
	return dir + "/"
}()

// captureStack collects PCs starting at the user-facing call site. skip
// counts the frames between runtime.Callers and that call site, including
// runtime.Callers itself and captureStack.
func captureStack(skip int) []uintptr {
	var pcs [stackMaxDepth]uintptr
	n := runtime.Callers(skip, pcs[:])
	if n == 0 {
		return nil
	}
	out := make([]uintptr, n)
	copy(out, pcs[:n])
	return out
}

// renderTraces converts raw PCs into "file:line (func)" strings, dropping
// stdlib and aerr-internal frames so users only see their own code.
// Returns nil when nothing remains.
func renderTraces(pcs []uintptr) []string {
	if len(pcs) == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs)
	out := make([]string, 0, len(pcs))
	var buf []byte
	for {
		frame, more := frames.Next()
		if !skipFrame(frame) {
			buf = appendFrame(buf[:0], frame)
			out = append(out, string(buf))
		}
		if !more {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// skipFrame reports whether a frame should be hidden from rendered traces:
// frames with no symbol, aerr's own internals, and the standard library
// (matched by file path, so user modules keep their frames regardless of
// how their module path is spelled).
func skipFrame(f runtime.Frame) bool {
	if f.Function == "" {
		return true
	}
	if strings.HasPrefix(f.Function, selfPkgPrefix) {
		return true
	}
	if stdlibDir != "" {
		return strings.HasPrefix(f.File, stdlibDir)
	}
	// Fallback when the stdlib anchor could not be resolved (a -trimpath
	// build reports relative stdlib paths, so stdlibDir is ""). Classify
	// by function name instead: package main is always user code, and a
	// stdlib import path's first segment carries no dot.
	if strings.HasPrefix(f.Function, "main.") {
		return false
	}
	return isStdlibFunc(f.Function)
}

// isStdlibFunc classifies a frame by its fully qualified function name,
// used only when stdlibDir could not be resolved (see stdlibDir). A
// function name has the form "<import-path>.<func>", where the import
// path may contain slashes and the func part may contain dots (methods
// read "(*Type).Method"). The first path segment is the text up to the
// first '/', or up to the first '.' when there is no slash. A stdlib
// import path's first segment carries no dot ("runtime", "net", "encoding"
// for net/http and encoding/json), whereas a module path opens with a
// domain segment like "github.com".
//
// This name heuristic cannot tell a locally-developed module whose path
// is a single dotless word (e.g. `module myapp`) from stdlib, so such a
// module's frames are mis-dropped under -trimpath; give the module a
// dotted or multi-segment path to keep them. package main is handled by
// the caller and never reaches here.
func isStdlibFunc(fn string) bool {
	seg, _, hasSlash := strings.Cut(fn, "/")
	if !hasSlash {
		seg, _, _ = strings.Cut(fn, ".")
	}
	return !strings.Contains(seg, ".")
}

// Frame is one entry of a captured stack trace in structured form, for
// exporters (Sentry, OpenTelemetry, ...) that need file/line/function
// separately instead of the rendered strings from Traces.
type Frame struct {
	// File is the source file path as reported by the runtime.
	File string
	// Line is the 1-based line number within File.
	Line int
	// Function is the fully qualified function name
	// (e.g. "github.com/user/project/pkg.(*Type).Method").
	Function string
}

// Frames returns the captured stack as structured frames, applying the
// same user-code filtering as Traces. It returns nil when no stack was
// captured. Unlike Traces the result is built on every call, so callers
// should retain it rather than re-invoke in hot paths.
func (e *Error) Frames() []Frame {
	if e == nil || len(e.pcs) == 0 {
		return nil
	}
	frames := runtime.CallersFrames(e.pcs)
	out := make([]Frame, 0, len(e.pcs))
	for {
		f, more := frames.Next()
		if !skipFrame(f) {
			out = append(out, Frame{File: f.File, Line: f.Line, Function: f.Function})
		}
		if !more {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendFrame(buf []byte, f runtime.Frame) []byte {
	buf = append(buf, f.File...)
	buf = append(buf, ':')
	buf = strconv.AppendInt(buf, int64(f.Line), 10)
	buf = append(buf, ' ', '(')
	buf = append(buf, f.Function...)
	buf = append(buf, ')')
	return buf
}
