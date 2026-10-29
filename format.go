package aerr

import (
	"fmt"
	"io"
)

// Format implements fmt.Formatter.
//
//	%s, %v   the combined message (same as Error())
//	%q       the combined message, quoted
//	%+v      multi-line detail: message, code, attributes, and the stack
//	         trace when one was captured (pkg/errors convention)
func (e *Error) Format(s fmt.State, verb rune) {
	// A fmt.State write has no error-return channel to the fmt caller, so
	// write results are intentionally discarded throughout this file.
	if e == nil {
		_, _ = io.WriteString(s, "<nil>")
		return
	}
	switch verb {
	case 'v':
		if s.Flag('+') {
			e.formatDetailed(s)
			return
		}
		_, _ = io.WriteString(s, e.msg)
	case 's':
		_, _ = io.WriteString(s, e.msg)
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", e.msg)
	default:
		_, _ = fmt.Fprintf(s, "%%!%c(*aerr.Error=%s)", verb, e.msg)
	}
}

func (e *Error) formatDetailed(w io.Writer) {
	_, _ = io.WriteString(w, e.msg)
	if e.code != "" {
		_, _ = io.WriteString(w, "\ncode: ")
		_, _ = io.WriteString(w, e.code)
	}
	if len(e.attrs) > 0 {
		_, _ = io.WriteString(w, "\nattributes:")
		for _, a := range e.attrs {
			_, _ = fmt.Fprintf(w, "\n    %s=%v", a.key, a.val)
		}
	}
	if traces := e.Traces(); len(traces) > 0 {
		_, _ = io.WriteString(w, "\nstacktrace:")
		for _, fr := range traces {
			_, _ = io.WriteString(w, "\n    ")
			_, _ = io.WriteString(w, fr)
		}
	}
}
