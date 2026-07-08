module github.com/tafaquh/aerr/examples

go 1.23

require (
	github.com/rs/zerolog v1.35.1
	github.com/tafaquh/aerr v1.1.0
	github.com/tafaquh/aerr/zerolog v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/sys v0.29.0 // indirect
)

replace (
	github.com/tafaquh/aerr => ../
	github.com/tafaquh/aerr/zerolog => ../zerolog
)
