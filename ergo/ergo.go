package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	"ergo.services/tools/ergo/templates"
)

var (
	OptionInit listOptions
	OptionPath string

	OptionWithApp   listOptions
	OptionWithSup   listOptions
	OptionWithActor listOptions
	OptionWithWeb   listOptions
	OptionWithTCP   listOptions
	OptionWithUDP   listOptions
	OptionWithPool  listOptions

	OptionWithMsg      listOptions
	OptionWithObserver listOptions
	OptionWithCloud    listOptions
)

func init() {
	flag.Var(&OptionInit, "init", "Node name. Available params: ssl, module")
	flag.StringVar(&OptionPath, "path", ".", "Set location")

	flag.Var(&OptionWithApp, "with-app", "Add Application. The name must be capitalized.")
	flag.Var(&OptionWithSup, "with-sup", "Add Supervisor. Available params: type, restart")

	flag.Var(&OptionWithActor, "with-actor", "Add actor")
	flag.Var(&OptionWithWeb, "with-web", "Add Web-server. Available params: host, port, ssl")
	flag.Var(&OptionWithTCP, "with-tcp", "Add TCP-server. Available params: host, port, ssl")
	flag.Var(&OptionWithUDP, "with-udp", "Add UDP-server. Available params: host, port")
	flag.Var(&OptionWithPool, "with-pool", "Add Pool of workers")

	flag.Var(&OptionWithMsg, "with-msg", "Add message for the networking")
	flag.Var(&OptionWithObserver, "with-observer", "Add Observer application")
	flag.Var(&OptionWithCloud, "with-cloud", "Enable cloud with given cluster name")
}

func main() {
	flag.Parse()

	if len(OptionInit) == 0 {
		fmt.Println("error: node name is empty")
		return
	}
	optionNode := OptionInit[0]
	optionNode.Package = "main"
	optionNode.Templates = templates.Node
	if _, exist := optionNode.Params["module"]; exist == false {
		optionNode.Params["module"] = optionNode.LoName
	}
	mod := path.Base(optionNode.Params["module"].(string))
	optionNode.Params["module-name"] = mod
	dir := path.Join(OptionPath, mod)
	optionNode.Dir = dir

	list := []*listOptions{
		OptionWithActor.WithTemplates(templates.Actor).WithDir(dir),
		OptionWithWeb.WithTemplates(templates.Web).WithDir(dir),
		OptionWithTCP.WithTemplates(templates.TCP).WithDir(dir),
		OptionWithUDP.WithTemplates(templates.UDP).WithDir(dir),
		OptionWithPool.WithTemplates(templates.Pool).WithDir(dir),
		OptionWithSup.WithTemplates(templates.Sup).WithDir(dir),

		// must be here due to traversing over the children
		// and updating the file location
		OptionWithApp.WithTemplates(templates.App).WithDir(dir).WithAppDir("apps"),
	}

	fmt.Printf("Generating project %q...\n", dir)
	for _, l := range list {
		for _, option := range *l {
			if err := generate(option); err != nil {
				fmt.Printf("error: %s\n", err)
				return
			}
			if option.Parent == nil && option.IsApp == false {
				// must be started by node
				optionNode.Children = append(optionNode.Children, option)
			}
		}
	}

	// node options - applications
	if len(OptionWithApp) > 0 {
		optionNode.Params["applications"] = OptionWithApp
	}
	// node options - cloud
	if OptionWithCloud != nil {
		optionNode.Params["cloud"] = OptionWithCloud
	}
	// node options - observer
	if OptionWithCloud != nil {
		optionNode.Params["observer"] = OptionWithObserver
	}
	// register types (messages for networking)
	if len(OptionWithMsg) > 0 {
		optionType := &Option{
			Name:      "types",
			Dir:       dir,
			Package:   mod,
			Templates: templates.Types,
			Params:    make(map[string]any),
			Children:  OptionWithMsg,
		}
		if err := generate(optionType); err != nil {
			fmt.Printf("error: %s\n", err)
			return
		}

		// enable RegisterTypes() call
		optionNode.Params["types"] = true
	}
	if err := generate(optionNode); err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	// README.md
	optionReadme := Option{
		Name:             "README.md",
		Dir:              dir,
		Templates:        templates.Readme,
		Params:           make(map[string]any),
		KeepOriginalName: true,
		SkipGoFormat:     true,
	}
	optionReadme.Params["applications"] = OptionWithApp
	optionReadme.Params["processes"] = optionNode.Children
	optionReadme.Params["project"] = optionNode.Name
	optionReadme.Params["types"] = OptionWithMsg

	optionReadme.Params["optionCloud"] = "false"
	if len(OptionWithCloud) > 0 {
		optionReadme.Params["optionCloud"] = "true"
	}
	optionReadme.Params["optionTypes"] = "false"
	if len(OptionWithMsg) > 0 {
		optionReadme.Params["optionTypes"] = "true"
	}
	optionReadme.Params["args"] = strings.Join(os.Args, " ")

	if err := generate(&optionReadme); err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	if err := generateGoMod(optionNode); err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	fmt.Println("Successfully completed.")
}
