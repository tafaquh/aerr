module github.com/tafaquh/aerr/zap

// Pinned by the published aerr v1.0.0 go directive; lower to 1.21 when
// bumping the aerr requirement.
go 1.24.7

require (
	// Bump to the next aerr release to use structured Frames.
	github.com/tafaquh/aerr v1.0.0
	go.uber.org/zap v1.28.0
)

require go.uber.org/multierr v1.10.0 // indirect
