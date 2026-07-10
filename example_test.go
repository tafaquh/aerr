package aerr_test

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tafaquh/aerr"
)

// ExampleCode builds an error with a code, message, and attributes and reads
// them back through the accessors. Stack traces are omitted because their
// contents are non-deterministic.
func ExampleCode() {
	err := aerr.Code("NOT_FOUND").
		Message("user missing").
		With("id", 42).
		Err(nil)

	e, _ := aerr.AsAerr(err)
	fmt.Println(e.Code())
	fmt.Println(e.Error())
	fmt.Println(e.Attributes()["id"])
	// Output:
	// NOT_FOUND
	// user missing
	// 42
}

// ExampleBuilder_Wrap shows that wrapping joins the outer message with the
// cause's message using ": ".
func ExampleBuilder_Wrap() {
	cause := errors.New("connection refused")
	err := aerr.Code("DB").Message("query failed").Wrap(cause)

	fmt.Println(err)
	fmt.Println(errors.Is(err, cause))
	// Output:
	// query failed: connection refused
	// true
}

// ExampleHasCode reports whether any layer of a chain carries a code, even
// when an outer layer's code hides it.
func ExampleHasCode() {
	inner := aerr.Code("INNER").ErrMsg("inner failure")
	outer := aerr.Code("OUTER").Message("outer failure").Wrap(inner)

	fmt.Println(aerr.HasCode(outer, "OUTER"))
	fmt.Println(aerr.HasCode(outer, "INNER"))
	fmt.Println(aerr.HasCode(outer, "MISSING"))
	// Output:
	// true
	// true
	// false
}

// ExampleError_MarshalJSON shows the canonical JSON shape, with empty fields
// omitted and attributes nested under "attributes".
func ExampleError_MarshalJSON() {
	err := aerr.Code("E").Message("boom").With("k", "v").Err(nil)
	e, _ := aerr.AsAerr(err)

	out, _ := json.Marshal(e)
	fmt.Println(string(out))
	// Output: {"code":"E","message":"boom","attributes":{"k":"v"}}
}

// ExampleRedact wraps a sensitive attribute so it renders as the placeholder
// on every path — here JSON — while the original stays recoverable
// in-process via Value.
func ExampleRedact() {
	err := aerr.Code("AUTH").
		Message("login failed").
		With("user", "alice").
		With("password", aerr.Redact("hunter2")).
		Err(nil)
	e, _ := aerr.AsAerr(err)

	out, _ := json.Marshal(e)
	fmt.Println(string(out))
	fmt.Println(e.Attributes()["password"].(aerr.Redacted).Value())
	// Output:
	// {"code":"AUTH","message":"login failed","attributes":{"user":"alice","password":"[REDACTED]"}}
	// hunter2
}
