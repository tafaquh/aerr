module github.com/tafaquh/aerr/zap

// Floor matches the root module's go directive (aerr v1.1.0 declares go 1.21).
go 1.21

require (
	github.com/tafaquh/aerr v1.1.0
	go.uber.org/zap v1.28.0
)

require go.uber.org/multierr v1.10.0 // indirect
