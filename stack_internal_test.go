package aerr

import (
	"runtime"
	"strings"
	"testing"
)

func TestStdlibDirResolved(t *testing.T) {
	if stdlibDir == "" {
		t.Fatal("stdlibDir not resolved; skipFrame is running on the fallback heuristic")
	}
	if !strings.HasSuffix(stdlibDir, "/") {
		t.Errorf("stdlibDir must end with '/': %q", stdlibDir)
	}
}

func TestSkipFrame(t *testing.T) {
	cases := []struct {
		name  string
		frame runtime.Frame
		skip  bool
	}{
		{
			name:  "no symbol",
			frame: runtime.Frame{},
			skip:  true,
		},
		{
			name: "aerr internal",
			frame: runtime.Frame{
				Function: "github.com/tafaquh/aerr.(*Builder).ErrMsg",
				File:     "/home/u/aerr/builder.go",
			},
			skip: true,
		},
		{
			name: "aerr external test package is user code",
			frame: runtime.Frame{
				Function: "github.com/tafaquh/aerr_test.TestSomething",
				File:     "/home/u/aerr/aerr_test.go",
			},
			skip: false,
		},
		{
			name: "user code with domain module path",
			frame: runtime.Frame{
				Function: "github.com/user/project/services.(*Service).Handle",
				File:     "/home/u/project/services/service.go",
			},
			skip: false,
		},
		{
			name: "user code in slashless local module",
			frame: runtime.Frame{
				Function: "myapp.LoadConfig",
				File:     "/home/u/myapp/config.go",
			},
			skip: false,
		},
		{
			name: "package main",
			frame: runtime.Frame{
				Function: "main.main",
				File:     "/home/u/app/main.go",
			},
			skip: false,
		},
		{
			name: "stdlib single-segment (runtime)",
			frame: runtime.Frame{
				Function: "runtime.goexit",
				File:     stdlibDir + "runtime/asm_amd64.s",
			},
			skip: true,
		},
		{
			name: "stdlib multi-segment (net/http)",
			frame: runtime.Frame{
				Function: "net/http.(*conn).serve",
				File:     stdlibDir + "net/http/server.go",
			},
			skip: true,
		},
		{
			name: "stdlib testing",
			frame: runtime.Frame{
				Function: "testing.tRunner",
				File:     stdlibDir + "testing/testing.go",
			},
			skip: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipFrame(tc.frame); got != tc.skip {
				t.Errorf("skipFrame(%q / %q) = %v, want %v",
					tc.frame.Function, tc.frame.File, got, tc.skip)
			}
		})
	}
}

func TestRenderTracesNilWhenAllFiltered(t *testing.T) {
	// PCs resolving only to stdlib/aerr frames must render as nil, matching
	// the Traces() doc ("nil when none was captured").
	if got := renderTraces(nil); got != nil {
		t.Errorf("renderTraces(nil) = %v, want nil", got)
	}
}
