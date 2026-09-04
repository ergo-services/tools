package ergomodel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func stubPass(t *testing.T, cfg *Config) (*analysis.Pass, *Model) {
	t.Helper()

	fset := token.NewFileSet()
	dir := t.TempDir()
	path := dir + "/x.go"
	if err := os.WriteFile(path, []byte("package x\n\nvar V int\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	pass := &analysis.Pass{
		Fset:  fset,
		Files: []*ast.File{file},
		Pkg:   types.NewPackage("example.com/app/x", "x"),
		Report: func(analysis.Diagnostic) {
			t.Error("a warn tier finding must not go through pass.Report: that is what fails the vet action")
		},
	}
	m := newModel(pass, cfg)
	m.baseline = &baseline{}
	return pass, m
}

func capture(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	savedOut, savedErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	fn()

	os.Stdout, os.Stderr = savedOut, savedErr
	outW.Close()
	errW.Close()

	o, _ := io.ReadAll(outR)
	e, _ := io.ReadAll(errR)
	return string(o), string(e)
}

func TestWarnTierNeverWritesToStdout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tiers[2] = SeverityWarn
	pass, m := stubPass(t, cfg)

	stdout, stderr := capture(t, func() {
		m.Report(pass, pass.Files[0].Pos(),
			Finding{Rule: "A2011", Kind: KindShape, Tier: 2, ID: "example.com/app/x.Args"},
			"a warn tier finding")
	})

	if stdout != "" {
		t.Errorf("stdout must stay empty for the unitchecker JSON stream, got %q", stdout)
	}
	if strings.Contains(stderr, "a warn tier finding") == false {
		t.Errorf("the finding must reach stderr, got %q", stderr)
	}
	if strings.Contains(stderr, "[tier2] [A2011]") == false {
		t.Errorf("stderr line %q is missing the tier and rule prefix", stderr)
	}
}

func TestErrorTierGoesThroughReport(t *testing.T) {
	cfg := DefaultConfig()
	pass, m := stubPass(t, cfg)

	reported := 0
	var got analysis.Diagnostic
	pass.Report = func(d analysis.Diagnostic) {
		reported++
		got = d
	}

	stdout, stderr := capture(t, func() {
		m.Report(pass, pass.Files[0].Pos(),
			Finding{Rule: "A1001", Kind: KindShape, Tier: 1, ID: "example.com/app/x.Message"},
			"an error tier finding")
	})

	if reported != 1 {
		t.Fatalf("pass.Report called %d times, want 1", reported)
	}
	if stdout != "" || stderr != "" {
		t.Errorf("an error tier finding must not also be printed: stdout %q stderr %q", stdout, stderr)
	}
	if strings.Contains(got.Message, "[tier1] [A1001]") == false {
		t.Errorf("diagnostic %q is missing the tier and rule prefix", got.Message)
	}

	if len(got.SuggestedFixes) == 0 {
		t.Fatal("every finding must offer the recorded exception as a fix")
	}
	last := got.SuggestedFixes[len(got.SuggestedFixes)-1]
	if strings.Contains(last.Message, "A1001") == false {
		t.Errorf("the last fix should be the directive, got %q", last.Message)
	}
	if strings.Contains(string(last.TextEdits[0].NewText), "//argus:allow A1001 <reason>") == false {
		t.Errorf("the directive fix must carry a visible placeholder, got %q", last.TextEdits[0].NewText)
	}
}

func TestTierOffIsSilent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tiers[3] = SeverityOff
	pass, m := stubPass(t, cfg)

	stdout, stderr := capture(t, func() {
		m.Report(pass, pass.Files[0].Pos(),
			Finding{Rule: "A3005", Kind: KindDirective, Tier: 3, ID: "x"},
			"an off tier finding")
	})
	if stdout != "" || stderr != "" {
		t.Errorf("a tier that is off must produce nothing: stdout %q stderr %q", stdout, stderr)
	}
}

func TestFunnelCountsSuppressions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tiers[1] = SeverityError
	pass, m := stubPass(t, cfg)
	pass.Report = func(analysis.Diagnostic) {}

	m.baseline = &baseline{entries: map[string]BaselineEntry{
		baselineIndex("A1001", "shape", "example.com/app/x.Order"): {
			Rule: "A1001", Kind: "shape", ID: "example.com/app/x.Order",
		},
	}}
	capture(t, func() {
		m.Report(pass, pass.Files[0].Pos(),
			Finding{Rule: "A1001", Kind: KindShape, Tier: 1, ID: "example.com/app/x.Order"},
			"baselined")
	})

	line, ok := m.DebtLine()
	if ok == false {
		t.Fatal("a baselined finding must be counted as debt")
	}
	if strings.Contains(line, "1 by baseline") == false {
		t.Errorf("debt line %q must attribute the suppression to the baseline", line)
	}
}
