package ergomodel

import (
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestForeignAcrossDrivers(t *testing.T) {
	cases := []struct {
		name   string
		module *analysis.Module
		want   bool
	}{
		{"main module", &analysis.Module{Path: "example.com/app", Main: true}, false},
		{"workspace module", &analysis.Module{Path: "example.com/lib", Main: true}, false},
		{"main module, older go vet", &analysis.Module{Path: "example.com/app"}, false},
		{"requirement", &analysis.Module{Path: "github.com/redis/go-redis/v9", Version: "v9.17.1"}, true},
		{"replaced requirement", &analysis.Module{
			Path: "ergo.services/ergo", Version: "v1.999.0",
			Replace: &analysis.Module{Path: "../ergo"},
		}, true},
		{"no module, an editable source", &analysis.Module{}, false},
		{"no module information at all", nil, false},
	}
	for _, c := range cases {
		pass, m := stubPass(t, DefaultConfig())
		pass.Module = c.module
		if got := m.foreign(); got != c.want {
			t.Errorf("%s: foreign() = %v, want %v", c.name, got, c.want)
		}
	}
}
