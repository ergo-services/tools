package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	cases := []struct {
		args     []string
		flags    []string
		patterns []string
	}{
		{[]string{"./..."}, []string{}, []string{"./..."}},
		{[]string{"-c", "1", "./..."}, []string{"-c", "1"}, []string{"./..."}},
		{[]string{"-argusmodel.config", "./argus.yml", "./..."},
			[]string{"-argusmodel.config", "./argus.yml"}, []string{"./..."}},
		{[]string{"-argusmodel.config=./argus.yml", "-json", "./a/...", "./b"},
			[]string{"-argusmodel.config=./argus.yml", "-json"}, []string{"./a/...", "./b"}},
		{[]string{"-include-tests", "--", "./..."}, []string{"-include-tests", "--"}, []string{"./..."}},
	}
	for _, c := range cases {
		flags, patterns := splitArgs(c.args)
		if len(flags) == 0 {
			flags = []string{}
		}
		if reflect.DeepEqual(flags, c.flags) == false || reflect.DeepEqual(patterns, c.patterns) == false {
			t.Errorf("splitArgs(%q) = %q, %q; want %q, %q", c.args, flags, patterns, c.flags, c.patterns)
		}
	}
}

func TestAbsolutePaths(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "work")
	abs := filepath.Join(root, "argus.yml")
	cases := []struct {
		flags []string
		want  []string
	}{
		{[]string{"-argusmodel.config=./argus.yml"}, []string{"-argusmodel.config=" + abs}},
		{[]string{"-argusmodel.config", "argus.yml", "-json"}, []string{"-argusmodel.config=" + abs, "-json"}},
		{[]string{"--cpuprofile=" + abs}, []string{"-cpuprofile=" + abs}},
		{[]string{"-include-tests", "-c", "2"}, []string{"-include-tests", "-c", "2"}},
		{[]string{"-argusmodel.config="}, []string{"-argusmodel.config="}},
	}
	for _, c := range cases {
		if got := absolutePaths(c.flags, root); reflect.DeepEqual(got, c.want) == false {
			t.Errorf("absolutePaths(%q) = %q, want %q", c.flags, got, c.want)
		}
	}
}

func TestPlanRuns(t *testing.T) {
	nested := filepath.Join("billing", "ledger")
	modules := []string{"api", "billing", nested, "web"}
	cases := []struct {
		patterns []string
		want     []moduleRun
	}{
		{[]string{"./..."}, []moduleRun{
			{"api", []string{"./..."}},
			{"billing", []string{"./..."}},
			{nested, []string{"./..."}},
			{"web", []string{"./..."}},
		}},
		{[]string{"./billing/..."}, []moduleRun{
			{"billing", []string{"./..."}},
			{nested, []string{"./..."}},
		}},
		{[]string{"./billing/ledger/store/..."}, []moduleRun{
			{nested, []string{"./store/..."}},
		}},
		{[]string{"./billing/cmd", "./billing/cmd"}, []moduleRun{
			{"billing", []string{"./cmd"}},
		}},
		{[]string{"./nothing/..."}, nil},
	}
	for _, c := range cases {
		if got := planRuns(modules, c.patterns); reflect.DeepEqual(got, c.want) == false {
			t.Errorf("planRuns(%q) = %+v, want %+v", c.patterns, got, c.want)
		}
	}
}

func TestStderrFilterCollapsesSkippedPackages(t *testing.T) {
	var buf bytes.Buffer
	f := &stderrFilter{w: &buf, header: "# ./billing", seen: map[string]bool{}}
	for _, line := range []string{
		"/src/billing/store/store.go:17:5: undefined: Config",
		"argusmodel: analysis skipped due to errors in package",
		"argusA1001: failed prerequisites: argusmodel@example.com/billing/store",
		"argusA1002: failed prerequisites: argusmodel@example.com/billing/store",
		"argusA3005: failed prerequisites: argusA1001@example.com/billing/store, argusmodel@example.com/billing/store",
		"/src/billing/api.go:9:2: [tier2] [A2029] HandleMessage returns the error of Send()",
	} {
		f.line(line)
	}
	f.close()

	want := strings.Join([]string{
		"# ./billing",
		"/src/billing/store/store.go:17:5: undefined: Config",
		"/src/billing/api.go:9:2: [tier2] [A2029] HandleMessage returns the error of Send()",
		"argus: skipped example.com/billing/store: the error above stopped the analysis there",
	}, "\n") + "\n"
	if got := buf.String(); got != want {
		t.Errorf("filtered output:\n%s\nwant:\n%s", got, want)
	}

	var quiet bytes.Buffer
	silent := &stderrFilter{w: &quiet, header: "# ./web", seen: map[string]bool{}}
	silent.close()
	if quiet.Len() != 0 {
		t.Errorf("a module with nothing to say must print nothing, not even its header: %q", quiet.String())
	}
}

func TestWalksModulesOutsideAModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and shells out to the go command")
	}

	bin := filepath.Join(t.TempDir(), "argus")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s\n%s", err, out)
	}

	write := func(root, path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(dir string) (string, error) {
		cmd := exec.Command(bin, "./...")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	clean := t.TempDir()
	write(clean, "one/go.mod", "module example.com/one\n\ngo 1.21\n")
	write(clean, "one/one.go", "package one\n\nfunc One() int { return 1 }\n")
	write(clean, "two/go.mod", "module example.com/two\n\ngo 1.21\n")
	write(clean, "two/two.go", "package two\n\nfunc Two() int { return 2 }\n")
	write(clean, "testdata/broken/go.mod", "module example.com/broken\n\ngo 1.21\n")
	write(clean, "testdata/broken/broken.go", "package broken\n\nfunc Broken() int { return missing }\n")
	if out, err := run(clean); err != nil || out != "" {
		t.Errorf("a walk over two clean modules must succeed silently, and skip testdata: %v\n%s", err, out)
	}

	broken := t.TempDir()
	write(broken, "svc/go.mod", "module example.com/svc\n\ngo 1.21\n")
	write(broken, "svc/svc.go", "package svc\n\nfunc Broken() int { return missing }\n")
	out, err := run(broken)
	if err == nil {
		t.Errorf("a module that does not type-check must fail the run:\n%s", out)
	}
	if strings.Contains(out, "failed prerequisites") || strings.Contains(out, "analysis skipped") {
		t.Errorf("the per-rule noise must be collapsed:\n%s", out)
	}
	if strings.Contains(out, "undefined: missing") == false {
		t.Errorf("the type error itself must come through:\n%s", out)
	}
	if n := strings.Count(out, "argus: skipped "); n != 1 {
		t.Errorf("want exactly one line naming what was skipped, got %d:\n%s", n, out)
	}
	if strings.HasPrefix(out, "# ./svc\n") == false {
		t.Errorf("the module's output must open with its header:\n%s", out)
	}

	empty := t.TempDir()
	out, err = run(empty)
	if err == nil {
		t.Errorf("a directory with no module must fail:\n%s", out)
	}
	if lines := strings.Count(strings.TrimSpace(out), "\n") + 1; lines != 1 {
		t.Errorf("a directory with no module must answer in one line, got %d:\n%s", lines, out)
	}
}
