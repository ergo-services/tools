package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVettoolProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and shells out to the go command")
	}

	bin := filepath.Join(t.TempDir(), "argus")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s\n%s", err, out)
	}

	cmd := exec.Command("go", "vet", "-vettool="+bin, "./ergomodel/", "./rules/")
	out, err := cmd.CombinedOutput()
	text := string(out)

	if strings.Contains(text, "parsing JSON") {
		t.Errorf("the tool corrupted the unitchecker JSON stream:\n%s", text)
	}
	if strings.Contains(text, "panic:") {
		t.Errorf("the tool panicked under unitchecker:\n%s", text)
	}
	if err != nil {
		t.Errorf("go vet over a clean module must succeed: %s\n%s", err, text)
	}
}

func TestVettoolOverErgoModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and shells out to the go command")
	}

	target, err := filepath.Abs("../../actor/health")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "go.mod")); err != nil {
		t.Skipf("no sibling module at %s", target)
	}

	bin := filepath.Join(t.TempDir(), "argus")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s\n%s", err, out)
	}

	cmd := exec.Command("go", "vet", "-vettool="+bin, "./...")
	cmd.Dir = target
	out, _ := cmd.CombinedOutput()
	text := string(out)

	if strings.Contains(text, "parsing JSON") {
		t.Errorf("the tool corrupted the unitchecker JSON stream:\n%s", text)
	}
	if strings.Contains(text, "panic:") {
		t.Errorf("the tool panicked over real code:\n%s", text)
	}

	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "[tier") == false {
			continue
		}
		if strings.Contains(line, "/pkg/mod/") {
			t.Errorf("reported on a dependency in the module cache: %s", line)
		}
	}
}
