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
	if e == nil {
		io.WriteString(s, "<nil>")
		return
	}
	switch verb {
	case 'v':
		if s.Flag('+') {
			e.formatDetailed(s)
			return
		}
		io.WriteString(s, e.msg)
	case 's':
		io.WriteString(s, e.msg)
	case 'q':
		fmt.Fprintf(s, "%q", e.msg)
	default:
		fmt.Fprintf(s, "%%!%c(*aerr.Error=%s)", verb, e.msg)
	}
}

func (e *Error) formatDetailed(w io.Writer) {
	io.WriteString(w, e.msg)
	if e.code != "" {
		io.WriteString(w, "\ncode: ")
		io.WriteString(w, e.code)
	}
	if len(e.attrs) > 0 {
		io.WriteString(w, "\nattributes:")
		for _, a := range e.attrs {
			fmt.Fprintf(w, "\n    %s=%v", a.key, a.val)
		}
	}
	if traces := e.Traces(); len(traces) > 0 {
		io.WriteString(w, "\nstacktrace:")
		for _, fr := range traces {
			io.WriteString(w, "\n    ")
			io.WriteString(w, fr)
		}
	}
}
