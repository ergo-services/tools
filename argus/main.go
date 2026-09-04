package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/multichecker"

	"ergo.tools/argus/ergomodel"
	"ergo.tools/argus/rules"
)

func main() {

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help":
			help(os.Args[2:])
			return
		case "baseline":
			baseline()
			return
		}
	}
	chdir()
	multichecker.Main(rules.Suite()...)
}

func baseline() {

	index := map[string]int{}
	sites := map[string]bool{}
	var entries []ergomodel.BaselineEntry

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		entry, position, ok := ergomodel.ParseKeyedDiagnostic(scanner.Text())
		if ok == false {
			continue
		}
		key := entry.Rule + "\x00" + entry.Kind + "\x00" + entry.ID

		site := key + "\x00" + position
		if sites[site] {
			continue
		}
		sites[site] = true

		if at, seen := index[key]; seen {
			entries[at].Count++
			continue
		}
		index[key] = len(entries)
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "argus: read stdin: %s\n", err)
		os.Exit(2)
	}

	out, err := ergomodel.WriteBaseline(entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "argus: write baseline: %s\n", err)
		os.Exit(2)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "argus: write baseline: %s\n", err)
		os.Exit(2)
	}
	total := 0
	for _, e := range entries {
		total += e.Count
	}
	fmt.Fprintf(os.Stderr, "argus: %d accepted findings across %d entries\n", total, len(entries))
}

func chdir() {
	if len(os.Args) < 2 {
		return
	}
	dir := ""
	strip := 0
	switch {
	case os.Args[1] == "-C" || os.Args[1] == "--C":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "argus: -C requires a directory")
			os.Exit(2)
		}
		dir, strip = os.Args[2], 2
	case strings.HasPrefix(os.Args[1], "-C="):
		dir, strip = strings.TrimPrefix(os.Args[1], "-C="), 1
	default:
		return
	}
	if err := os.Chdir(dir); err != nil {
		fmt.Fprintf(os.Stderr, "argus: -C %s: %s\n", dir, err)
		os.Exit(2)
	}
	os.Args = append(os.Args[:1], os.Args[1+strip:]...)
}

func help(args []string) {
	suite := rules.Rules()

	if len(args) > 0 {
		want := strings.ToLower(args[0])
		for _, a := range suite {
			if strings.ToLower(a.Name) == want || strings.Contains(strings.ToLower(a.Name), want) {
				fmt.Printf("%s\n\n%s\n\nMore: %s\n", a.Name, a.Doc, a.URL)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "argus: unknown rule %q\n", args[0])
		os.Exit(2)
	}

	fmt.Println("argus checks Ergo actor model invariants.")
	fmt.Println()
	fmt.Println("Rules in this build:")
	for _, a := range suite {
		title := a.Doc
		if i := strings.IndexByte(title, '\n'); i >= 0 {
			title = title[:i]
		}
		fmt.Printf("  %-14s %s\n", a.Name, title)
	}
	fmt.Println("\nargus help <rule>   full documentation for one rule")
	fmt.Println("argus baseline      read a keyed run on stdin, write a baseline file")
	fmt.Println("go vet -vettool=$(which argus) ./...")
}
