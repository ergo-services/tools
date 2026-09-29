package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"ergo.tools/argus/rules"
)

const drivenEnv = "ARGUS_DRIVEN"

type moduleRun struct {
	dir      string
	patterns []string
}

func drive() (int, bool) {
	if os.Getenv(drivenEnv) != "" {
		return 0, false
	}
	flags, patterns := splitArgs(os.Args[1:])
	if len(patterns) == 0 {
		return 0, false
	}
	for _, p := range patterns {
		if strings.HasSuffix(p, ".cfg") {
			return 0, false
		}
	}
	for _, f := range flags {
		if name, _, _ := strings.Cut(strings.TrimLeft(f, "-"), "="); name == "V" || name == "flags" {
			return 0, false
		}
	}

	if relative(patterns) == false || inModule() {
		return runDriven(os.Args[1:], ""), true
	}

	root, err := os.Getwd()
	if err != nil {
		return runDriven(os.Args[1:], ""), true
	}
	runs := planRuns(findModules(root), patterns)
	if len(runs) == 0 {
		fmt.Fprintf(os.Stderr,
			"argus: %s is not inside a Go module, and no module below it matches %s; run argus in a module, or pass -C <dir>\n",
			root, strings.Join(patterns, " "))
		return 1, true
	}
	flags = absolutePaths(flags, root)
	code := 0
	for _, r := range runs {
		args := append([]string{"-C", filepath.Join(root, r.dir)}, flags...)
		args = append(args, r.patterns...)
		code = worse(code, runDriven(args, "# ./"+filepath.ToSlash(r.dir)))
	}
	return code, true
}

func splitArgs(args []string) ([]string, []string) {
	valued := map[string]bool{
		"c": true, "debug": true, "cpuprofile": true, "memprofile": true,
		"trace": true, "tags": true,
	}
	for _, a := range rules.Suite() {
		a.Flags.VisitAll(func(f *flag.Flag) {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
				return
			}
			valued[a.Name+"."+f.Name] = true
		})
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			return args[:i+1], args[i+1:]
		}
		if strings.HasPrefix(arg, "-") == false || arg == "-" {
			break
		}
		name := strings.TrimLeft(arg, "-")
		if strings.Contains(name, "=") == false && valued[name] {
			i += 2
			continue
		}
		i++
	}
	if i > len(args) {
		i = len(args)
	}
	return args[:i], args[i:]
}

func absolutePaths(flags []string, root string) []string {
	paths := map[string]bool{
		"argusmodel.config": true, "cpuprofile": true, "memprofile": true, "trace": true,
	}
	out := make([]string, 0, len(flags))
	for i := 0; i < len(flags); i++ {
		arg := flags[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if strings.HasPrefix(arg, "-") == false || paths[name] == false {
			out = append(out, arg)
			continue
		}
		if hasValue == false {
			if i+1 >= len(flags) {
				out = append(out, arg)
				continue
			}
			i++
			value = flags[i]
		}
		if value != "" && filepath.IsAbs(value) == false {
			value = filepath.Join(root, value)
		}
		out = append(out, "-"+name+"="+value)
	}
	return out
}

func relative(patterns []string) bool {
	for _, p := range patterns {
		if p != "." && strings.HasPrefix(p, "./") == false {
			return false
		}
	}
	return true
}

func inModule() bool {
	out, err := exec.Command("go", "env", "GOMOD", "GOWORK").Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != os.DevNull && line != "off" {
			return true
		}
	}
	return false
}

func findModules(root string) []string {
	var out []string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		if rel, err := filepath.Rel(root, filepath.Dir(path)); err == nil {
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func planRuns(modules []string, patterns []string) []moduleRun {
	var runs []moduleRun
	for _, mod := range modules {
		var pats []string
		add := func(p string) {
			for _, have := range pats {
				if have == p {
					return
				}
			}
			pats = append(pats, p)
		}
		for _, p := range patterns {
			base, recursive := strings.CutSuffix(p, "/...")
			base = filepath.Clean(base)
			switch {
			case within(mod, base):
				if recursive {
					add("./...")
				} else if mod == base {
					add(".")
				}
			case within(base, mod):
				if innermost(modules, base) != mod {
					continue
				}
				rel, err := filepath.Rel(mod, base)
				if err != nil {
					continue
				}
				q := "./" + filepath.ToSlash(rel)
				if recursive {
					q += "/..."
				}
				add(q)
			}
		}
		if len(pats) > 0 {
			runs = append(runs, moduleRun{dir: mod, patterns: pats})
		}
	}
	return runs
}

func innermost(modules []string, dir string) string {
	best := ""
	for _, mod := range modules {
		if within(dir, mod) && len(mod) > len(best) {
			best = mod
		}
	}
	return best
}

func within(child, parent string) bool {
	return parent == "." || child == parent ||
		strings.HasPrefix(child, parent+string(filepath.Separator))
}

func runDriven(args []string, header string) int {
	self, err := os.Executable()
	if err != nil {
		self = os.Args[0]
	}
	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), drivenEnv+"=1")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "argus: %s\n", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "argus: %s\n", err)
		return 1
	}

	out := &stderrFilter{w: os.Stderr, header: header, seen: map[string]bool{}}
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		out.line(scanner.Text())
	}
	out.close()
	return exitCode(cmd.Wait())
}

type stderrFilter struct {
	w       io.Writer
	header  string
	started bool
	skipped []string
	seen    map[string]bool
}

func (f *stderrFilter) emit(s string) {
	if f.started == false && f.header != "" {
		fmt.Fprintln(f.w, f.header)
	}
	f.started = true
	fmt.Fprintln(f.w, s)
}

func (f *stderrFilter) line(s string) {
	name, rest, ok := strings.Cut(s, ": ")
	if ok && strings.HasPrefix(name, "argus") && strings.ContainsAny(name, " \t/") == false {
		if rest == "analysis skipped due to errors in package" {
			return
		}
		if failed, isPrereq := strings.CutPrefix(rest, "failed prerequisites: "); isPrereq {
			for _, item := range strings.Split(failed, ", ") {
				_, pkg, found := strings.Cut(item, "@")
				if found && f.seen[pkg] == false {
					f.seen[pkg] = true
					f.skipped = append(f.skipped, pkg)
				}
			}
			return
		}
	}
	f.emit(s)
}

func (f *stderrFilter) close() {
	if len(f.skipped) == 0 {
		return
	}
	list := strings.Join(f.skipped, ", ")
	if len(f.skipped) > 3 {
		list = fmt.Sprintf("%s and %d more", strings.Join(f.skipped[:3], ", "), len(f.skipped)-3)
	}
	f.emit(fmt.Sprintf("argus: skipped %s: the error above stopped the analysis there", list))
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	fmt.Fprintf(os.Stderr, "argus: %s\n", err)
	return 1
}

func worse(a, b int) int {
	switch {
	case a != 0 && a != 3:
		return a
	case b != 0 && b != 3:
		return b
	case a == 3 || b == 3:
		return 3
	}
	return 0
}
