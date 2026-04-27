package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const version = "3.1.0"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	var err error
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("ergo version %s\n", version)
		fmt.Println("docs: https://docs.ergo.services/tools/ergo")

	case "help", "--help", "-h":
		printUsage()

	case "init":
		err = cmdInit(args[1:])

	case "add":
		err = cmdAdd(args[1:])

	case "generate":
		err = cmdGenerate(args[1:])

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", args[0])
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`ergo - boilerplate generator for Ergo Framework
https://docs.ergo.services/tools/ergo

Usage:
  ergo <command> [arguments]

Commands:
  init <NodeName> <module>
        Create a new project. Generates ergo.yaml, go.mod and all
        boilerplate files. NodeName is the Erlang-style node name
        (e.g. MyNode), module is the Go module path (e.g. github.com/org/repo).

  add actor [--pool] <[Parent:]Name>
        Add an actor to the project. Parent is the name of an existing
        supervisor or application. --pool generates a pool actor with
        a companion worker type.

  add supervisor [--type <type>] [--strategy <strategy>] <[Parent:]Name>
        Add a supervisor. Parent is an existing app or supervisor.
        --type: one_for_one (default), all_for_one, rest_for_one,
                simple_one_for_one (children spawned dynamically at runtime)
        --strategy: transient (default), permanent, temporary

  add app [--mode <mode>] <Name>
        Add a new application.
        --mode: transient (default), permanent, temporary

  add message --field name:type [--field ...] <Name>
        Add an EDF network message type. Field types are standard Go types,
        e.g. string, int, []byte, gen.Alias, gen.PID.

  generate [ergo.yaml]
        Regenerate all *_gen.go files from ergo.yaml.
        User-owned .go files are never overwritten.
        Defaults to ./ergo.yaml in the current or any parent directory.

  version
        Print version information.

  help
        Print this help message.

Examples:
  ergo init MyNode github.com/myorg/mynode
  ergo add supervisor MyNodeApp:WorkerSup --type one_for_one
  ergo add actor WorkerSup:MyWorker
  ergo add actor --pool WorkerSup:MyPool
  ergo add message MessageConnect --field ID:gen.Alias --field Addr:string
`)
}

// cmdInit implements "ergo init <NodeName> <module>".
func cmdInit(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: ergo init <NodeName> <module>")
	}
	nodeName := args[0]
	module := args[1]

	// derive output directory from module name (last path element)
	parts := strings.Split(module, "/")
	dirName := parts[len(parts)-1]
	outputDir, err := filepath.Abs(dirName)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// build default project
	appName := nodeName + "App"
	supName := nodeName + "Sup"
	actorName := nodeName + "Actor"

	proj := &Project{
		Node: NodeSpec{
			Name:   nodeName,
			Module: module,
			Host:   "localhost",
			Network: NetworkSpec{
				TLS:    false,
				Cookie: "",
			},
			Apps: []AppSpec{
				{
					Name: appName,
					Mode: "transient",
					Children: []ChildSpec{
						{
							Sup:       supName,
							Type:      "one_for_one",
							Strategy:  "transient",
							Intensity: 2,
							Period:    5,
							Children: []ChildSpec{
								{Actor: actorName},
							},
						},
					},
				},
			},
		},
	}

	yamlPath := filepath.Join(outputDir, "ergo.yaml")
	if err := writeProject(yamlPath, proj); err != nil {
		return err
	}

	if err := Generate(proj, outputDir); err != nil {
		return err
	}

	fmt.Printf("Project created in %s\n", outputDir)
	fmt.Printf("Run: cd %s && go run ./cmd\n", dirName)
	return nil
}

// cmdAdd implements "ergo add <kind> ...".
func cmdAdd(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ergo add <actor|supervisor|app|message> ...")
	}
	kind := args[0]
	rest := args[1:]

	yamlPath, err := findYAML(".")
	if err != nil {
		return err
	}
	outputDir := filepath.Dir(yamlPath)

	proj, err := readProject(yamlPath)
	if err != nil {
		return err
	}

	switch kind {
	case "actor":
		if err := addActor(proj, rest); err != nil {
			return err
		}
	case "supervisor", "sup":
		if err := addSupervisor(proj, rest); err != nil {
			return err
		}
	case "app":
		if err := addApp(proj, rest); err != nil {
			return err
		}
	case "message":
		if err := addMessage(proj, rest); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown add kind: %s", kind)
	}

	if err := writeProject(yamlPath, proj); err != nil {
		return err
	}

	return Generate(proj, outputDir)
}

// addActor parses "ergo add actor [--pool] <[Parent:]Name>" and updates the project.
func addActor(proj *Project, args []string) error {
	isPool := hasBoolFlag(args, "--pool")

	// find first non-flag argument
	nameArg := ""
	for _, a := range args {
		if strings.HasPrefix(a, "--") == false {
			nameArg = a
			break
		}
	}
	if nameArg == "" {
		return fmt.Errorf("usage: ergo add actor [--pool] <[Parent:]Name>")
	}
	parentName, name := splitParent(nameArg)
	return addActorToProject(proj, parentName, name, isPool)
}

// addSupervisor parses "ergo add supervisor <[Parent:]Name> [--type t] [--strategy s]".
func addSupervisor(proj *Project, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ergo add supervisor <[Parent:]Name> [--type one_for_one] [--strategy transient]")
	}
	parentName, name := splitParent(args[0])
	supTyp := flagValue(args[1:], "--type", "one_for_one")
	strategy := flagValue(args[1:], "--strategy", "transient")

	spec := ChildSpec{
		Sup:       name,
		Type:      supTyp,
		Strategy:  strategy,
		Intensity: 2,
		Period:    5,
	}
	return addSupToProject(proj, parentName, spec)
}

// addApp parses "ergo add app <Name> [--mode transient]".
func addApp(proj *Project, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ergo add app <Name> [--mode transient]")
	}
	name := args[0]
	mode := flagValue(args[1:], "--mode", "transient")

	spec := AppSpec{
		Name: name,
		Mode: mode,
	}
	return addAppToProject(proj, spec)
}

// addMessage parses "ergo add message <Name> --field name:type ...".
func addMessage(proj *Project, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: ergo add message <Name> --field name:type [--field name:type ...]")
	}
	name := args[0]
	fields := flagValues(args[1:], "--field")

	var fieldMaps []map[string]string
	for _, f := range fields {
		parts := strings.SplitN(f, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format %q: expected name:type", f)
		}
		fieldMaps = append(fieldMaps, map[string]string{parts[0]: parts[1]})
	}

	spec := MessageSpec{
		Name:   name,
		Fields: fieldMaps,
	}
	return addMessageToProject(proj, spec)
}

// cmdGenerate implements "ergo generate [ergo.yaml]".
func cmdGenerate(args []string) error {
	yamlPath := "ergo.yaml"
	if len(args) > 0 {
		yamlPath = args[0]
	}

	absPath, err := filepath.Abs(yamlPath)
	if err != nil {
		return err
	}

	proj, err := readProject(absPath)
	if err != nil {
		return err
	}

	outputDir := filepath.Dir(absPath)
	return Generate(proj, outputDir)
}

// findYAML searches for ergo.yaml starting from dir and walking up.
func findYAML(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		candidate := filepath.Join(abs, "ergo.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return "", fmt.Errorf("ergo.yaml not found in %s or any parent directory", dir)
}

// splitParent splits "Parent:Name" into (parent, name).
// If there is no colon, parent is empty.
func splitParent(s string) (parent string, name string) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return "", s
	}
	return s[:idx], s[idx+1:]
}

// hasBoolFlag returns true if flag is present in args.
func hasBoolFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// flagValue returns the value of a named flag, or defaultVal if not present.
func flagValue(args []string, flag string, defaultVal string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return defaultVal
}

// flagValues returns all values for a repeated flag.
func flagValues(args []string, flag string) []string {
	var result []string
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			result = append(result, args[i+1])
		}
	}
	return result
}
