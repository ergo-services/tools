package callbackblocking_test

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/rules/callbackblocking"
)

func TestChannelWaitsStayInTheirModule(t *testing.T) {
	dir, err := filepath.Abs("../../testdata/mod/a1002")
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, dir, callbackblocking.Analyzer, "example.com/service")
}
