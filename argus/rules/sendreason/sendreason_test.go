package sendreason_test

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/ergomodel"
	"ergo.tools/argus/rules/sendreason"
)

func TestFindingKeys(t *testing.T) {
	dir, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	if err := ergomodel.Analyzer.Flags.Set("keys", "true"); err != nil {
		t.Fatal(err)
	}
	defer ergomodel.Analyzer.Flags.Set("keys", "false")
	analysistest.Run(t, dir, sendreason.Analyzer, "a2029keys")
}
