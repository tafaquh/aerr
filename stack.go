package aerr

import (
	"runtime"
	"strconv"
	"strings"
)

const stackMaxDepth = 32

// captureStack collects PCs starting at the user-facing caller of the
// *Builder method that invoked it. The hard-coded skip of 3 covers
// runtime.Callers, captureStack, and the *Builder method itself.
func captureStack() []uintptr {
	var pcs [stackMaxDepth]uintptr
	n := runtime.Callers(3, pcs[:])
	if n == 0 {
		return nil
	}
	out := make([]uintptr, n)
	copy(out, pcs[:n])
	return out
}

// renderTraces converts raw PCs into "file.(func):line" strings, dropping
// stdlib and test-runner frames so users only see their own code.
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
	return out
}

// skipFrame drops frames that don't belong to user code. The heuristic:
// function names without a "/" are stdlib (since user code carries a
// module import path), with the special case for package main. This avoids
// depending on runtime.GOROOT, which is deprecated and unreliable across
// machines.
func skipFrame(f runtime.Frame) bool {
	fn := f.Function
	if fn == "" {
		return true
	}
	if strings.HasPrefix(fn, "main.") {
		return false
	}
	return !strings.Contains(fn, "/")
}

func appendFrame(buf []byte, f runtime.Frame) []byte {
	buf = append(buf, f.File...)
	buf = append(buf, '.', '(')
	buf = append(buf, f.Function...)
	buf = append(buf, ')', ':')
	buf = strconv.AppendInt(buf, int64(f.Line), 10)
	return buf
}
