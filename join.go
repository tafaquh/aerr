package aerr

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Join combines errs into a single error, following the Go 1.20+ standard
// library convention for aggregates: the result exposes its members through
// Unwrap() []error, so errors.Is and errors.As traverse into every branch.
//
// Nil elements are dropped. When nothing non-nil remains Join returns nil,
// and when exactly one element remains it is returned unchanged — no wrapper
// is allocated, so the caller's == comparisons and type assertions survive.
// In that single-error case the result carries whatever behavior the original
// had and need not implement Unwrap() []error; use [Errors] when a slice is
// wanted regardless of shape.
//
// Join attaches no code, message, attributes, or stack trace. Wrap the result
// with a [Builder] to add structure:
//
//	if err := aerr.Join(steps...); err != nil {
//		return aerr.Code("BATCH_FAILED").Message("import failed").Wrap(err)
//	}
//
// Flattening is deliberately conservative: an element that is itself an
// aggregate built by Join contributes its children rather than itself, so
// accumulating in a loop yields one flat list instead of an ever-deepening
// tree. Aggregates from other sources — including errors.Join results — are
// kept as single opaque elements, because only structures aerr built are
// aerr's to restructure. Extraction is the liberal counterpart: [Errors]
// reads any Unwrap() []error implementation.
func Join(errs ...error) error {
	var (
		single  error // the sole non-nil element, when there is exactly one
		nonNil  int   // non-nil elements, before flattening
		flatLen int   // elements after splicing aerr's own aggregates
	)
	for _, err := range errs {
		if err == nil {
			continue
		}
		nonNil++
		single = err
		if children, ok := spliceOf(err); ok {
			flatLen += len(children)
			continue
		}
		flatLen++
	}
	switch nonNil {
	case 0:
		return nil
	case 1:
		return single
	}

	flat := make([]error, 0, flatLen)
	for _, err := range errs {
		if err == nil {
			continue
		}
		if children, ok := spliceOf(err); ok {
			flat = append(flat, children...)
			continue
		}
		flat = append(flat, err)
	}
	return &joinError{errs: flat}
}

// spliceOf returns the children err contributes to a flattened join when err
// is one of aerr's own aggregates, and reports false otherwise. Both Join
// passes go through it so the counted capacity and the filled slice cannot
// disagree.
func spliceOf(err error) ([]error, bool) {
	j, ok := err.(*joinError)
	if !ok || j == nil {
		return nil, false
	}
	return j.errs, true
}

// joinError is the aggregate [Join] returns. It is unexported because its
// only contract is the standard one — error and Unwrap() []error — plus the
// renderers every aerr value has: fmt.Formatter, slog.LogValuer, and
// json.Marshaler.
//
// Three invariants hold for every value Join produces and are relied on by
// the methods below: errs holds at least two errors, none of them nil (which
// also satisfies the standard library's rule that an Unwrap() []error result
// must not contain nils), and none of them another *joinError.
type joinError struct {
	errs []error
}

// Error returns the members' messages joined with "; ". The separator keeps
// an aggregate on one line so it does not break log pipelines that treat a
// newline as a record boundary; %+v renders the expanded form.
func (j *joinError) Error() string {
	if j == nil {
		return ""
	}
	parts := make([]string, len(j.errs))
	for i, err := range j.errs {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "; ")
}

// Unwrap returns the aggregated errors, letting errors.Is and errors.As
// search every branch. The returned slice is the aggregate's own storage and
// must not be modified; use [Errors] for a copy callers may keep.
func (j *joinError) Unwrap() []error {
	if j == nil {
		return nil
	}
	return j.errs
}

// Format implements fmt.Formatter.
//
//	%s, %v   the members' messages joined with "; " (same as Error())
//	%q       that same single line, quoted
//	%+v      multi-line detail: a count header followed by one bullet per
//	         member, each rendered with %+v and indented to align
func (j *joinError) Format(s fmt.State, verb rune) {
	// A fmt.State write has no error-return channel to the fmt caller, so
	// write results are intentionally discarded (see format.go for the
	// same convention).
	if j == nil {
		_, _ = io.WriteString(s, "<nil>")
		return
	}
	switch verb {
	case 'v':
		if s.Flag('+') {
			j.formatDetailed(s)
			return
		}
		_, _ = io.WriteString(s, j.Error())
	case 's':
		_, _ = io.WriteString(s, j.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", j.Error())
	default:
		_, _ = fmt.Fprintf(s, "%%!%c(*aerr.joinError=%s)", verb, j.Error())
	}
}

// formatDetailed writes the expanded form: a header, then one "- " bullet per
// member (see writeMembers).
func (j *joinError) formatDetailed(w io.Writer) {
	_, _ = fmt.Fprintf(w, "%d errors occurred:", len(j.errs))
	writeMembers(w, j.errs)
}

// writeMembers writes one "- " bullet per member. Each member is rendered
// with %+v and every newline inside that rendering is re-indented so
// continuation lines — an *Error's code, attributes, and stack — align under
// the bullet's text.
func writeMembers(w io.Writer, members []error) {
	for _, err := range members {
		detail := strings.ReplaceAll(fmt.Sprintf("%+v", err), "\n", "\n      ")
		_, _ = fmt.Fprintf(w, "\n    - %s", detail)
	}
}

// LogValue implements slog.LogValuer, producing a group with the combined
// message and every member under errors, each with its own code,
// attributes, and stack trace — the same shape [Error.LogValue] gives an
// *Error that wraps an aggregate, without a code or attributes of its own.
func (j *joinError) LogValue() slog.Value {
	if j == nil {
		return slog.Value{}
	}
	return slog.GroupValue(
		slog.String("message", errorText(j)),
		slog.Any("errors", errorList(j.errs)),
	)
}

// MarshalJSON implements json.Marshaler, producing the combined message and
// every member under "errors" (see [Error.MarshalJSON] for the member shape).
// Like the *Error encoding it never fails and never panics.
func (j *joinError) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return appendErrorJSON(make([]byte, 0, 256), j), nil
}

// errorList carries an aggregate's members into a slog record. slog has no
// array-of-groups kind, so the list renders through aerr's JSON encoder as
// the same "errors" array json.Marshal and the logging adapters produce.
type errorList []error

// MarshalJSON implements json.Marshaler, used by slog.JSONHandler.
func (l errorList) MarshalJSON() ([]byte, error) {
	return appendErrorsJSON(make([]byte, 0, 128), l), nil
}

// MarshalText implements encoding.TextMarshaler, so slog.TextHandler writes
// the members as one line of JSON instead of a multi-line %+v dump that
// would split the record.
func (l errorList) MarshalText() ([]byte, error) {
	return l.MarshalJSON()
}

// Errors returns err's members as a slice, reading any aggregate that follows
// the standard convention — one built by [Join], one from errors.Join, or any
// other type implementing Unwrap() []error.
//
// A nil error yields nil, and an error that is not an aggregate yields a
// one-element slice holding it, so callers can range over the result without
// first testing the shape. The slice is a fresh copy the caller may retain
// and modify; the members themselves are shared.
func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if agg, ok := err.(interface{ Unwrap() []error }); ok {
		members := agg.Unwrap()
		out := make([]error, len(members))
		copy(out, members)
		return out
	}
	return []error{err}
}

// JoinInto joins err into the error pointed to by into and reports whether
// err was non-nil. It is the accumulator form of [Join], for loops that must
// keep going after a failure:
//
//	var err error
//	for _, item := range items {
//		if aerr.JoinInto(&err, process(item)) {
//			continue
//		}
//		// ...
//	}
//	return err
//
// A nil err leaves *into untouched and returns false, so the common no-error
// iteration costs nothing. Because Join flattens its own aggregates, an
// accumulation of any length stays a single flat list.
//
// JoinInto panics when into is nil. Do not use it in a defer: deferred call
// arguments are evaluated at defer time, so the error would be captured
// before the deferred work has run. Use [JoinFunc] there.
func JoinInto(into *error, err error) bool {
	if into == nil {
		panic("aerr: JoinInto: into pointer must not be nil")
	}
	if err == nil {
		return false
	}
	*into = Join(*into, err)
	return true
}

// JoinFunc calls fn and joins a non-nil result into the error pointed to by
// into. Because fn runs when the call runs, JoinFunc is the form to use in a
// defer — it captures cleanup failures that would otherwise be dropped. This
// requires a named return value:
//
//	func write(path string) (err error) {
//		f, err := os.Create(path)
//		if err != nil {
//			return err
//		}
//		defer aerr.JoinFunc(&err, f.Close)
//		// ...
//	}
//
// A failing close then joins onto whatever the body returned, so the caller
// sees "primary failure; close failure" instead of only one of the two.
//
// JoinFunc panics when into is nil, checked before fn is invoked.
func JoinFunc(into *error, fn func() error) {
	if into == nil {
		panic("aerr: JoinFunc: into pointer must not be nil")
	}
	if err := fn(); err != nil {
		*into = Join(*into, err)
	}
}
