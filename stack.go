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
// from rendered traces. Comparing file paths self-consistently against
// what the runtime reports keeps the check correct under -trimpath and
// avoids the deprecated runtime.GOROOT.
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
	// Fallback when the stdlib anchor could not be resolved: function
	// names without a module path slash are stdlib, except package main.
	if strings.HasPrefix(f.Function, "main.") {
		return false
	}
	return !strings.Contains(f.Function, "/")
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
