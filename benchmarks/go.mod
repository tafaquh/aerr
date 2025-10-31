module github.com/tafaquh/aerr/benchmarks

go 1.24.7

require (
	github.com/rs/zerolog v1.34.0
	github.com/tafaquh/aerr v0.0.0-20251031045827-82173c23ef50
	github.com/tafaquh/aerr/zerolog v0.0.0-00010101000000-000000000000
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sys v0.12.0 // indirect
)

replace (
	github.com/tafaquh/aerr => ../
	github.com/tafaquh/aerr/zerolog => ../zerolog
)
