package registration_test

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"ergo.tools/argus/rules/registration"
)

func TestMainPlacement(t *testing.T) {
	dir, err := filepath.Abs("../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, dir, registration.Analyzer, "a2009/cmd")
}
