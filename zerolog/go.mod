module github.com/tafaquh/aerr/zerolog

// Pinned by the published aerr v1.0.0 go directive; lower to 1.21 when
// bumping the aerr requirement to the next release (which declares 1.21).
go 1.24.7

// v1.0.0 required the parent module at a pre-release pseudo-version whose
// API no longer existed, hidden by a replace directive that consumers do
// not inherit — `go get github.com/tafaquh/aerr/zerolog@v1.0.0` did not
// compile on its own.
retract v1.0.0

require (
	github.com/rs/zerolog v1.34.0
	github.com/tafaquh/aerr v1.0.0
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	golang.org/x/sys v0.12.0 // indirect
)
