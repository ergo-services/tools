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
	OptionWithMsg   listOptions

	OptionWithLogger listOptions

	OptionWithObserver bool

	loggers map[string]string
)

func init() {
	flag.Var(&OptionInit, "init", "Node name. Available params: tls, module")
	flag.StringVar(&OptionPath, "path", ".", "Set location")

	flag.Var(&OptionWithApp, "with-app", "Add Application. Available params: mode")
	flag.Var(&OptionWithSup, "with-sup", "Add Supervisor. Available params: type, strategy")

	flag.Var(&OptionWithActor, "with-actor", "Add actor")
	flag.Var(&OptionWithWeb, "with-web", "Add Web-server. Available params: host, port, tls, websocket")
	flag.Var(&OptionWithTCP, "with-tcp", "Add TCP-server. Available params: host, port, tls")
	flag.Var(&OptionWithUDP, "with-udp", "Add UDP-server. Available params: host, port")
	flag.Var(&OptionWithPool, "with-pool", "Add Pool of workers. Available params: size")

	flag.Var(&OptionWithMsg, "with-msg", "Add message for the networking")

	flag.BoolVar(&OptionWithObserver, "with-observer", false, "Add Observer application")

	flag.Var(&OptionWithLogger, "with-logger", "Add logger. See https://github.com/ergo-services/logger for available loggers")
	loggers = map[string]string{
		"colored": "ergo.services/logger/colored",
		"rotate":  "ergo.services/logger/rotate",
	}
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

	// check if observer has been enabled
	ext_applications := listOptions{}
	if OptionWithObserver {
		optionNode.Params["observer"] = true
		observer := &Option{
			Name:   "App",
			LoName: "observer",
			Params: make(map[string]any),
		}
		observer.Params["import"] = "ergo.services/application/observer"
		observer.Params["args"] = "observer.Options{}"
		ext_applications = append(ext_applications, observer)
	}

	if len(ext_applications) > 0 {
		optionNode.Params["ext_applications"] = ext_applications
	}

	if OptionWithLogger != nil {
		for i := range OptionWithLogger {
			m, exist := loggers[OptionWithLogger[i].Name]
			if exist == false {
				fmt.Printf("error: unknown logger name %q\n", OptionWithLogger[i].Name)
				return
			}
			OptionWithLogger[i].Params["import"] = m
		}
		optionNode.Params["loggers"] = OptionWithLogger
	}

	if len(OptionWithMsg) > 0 {
		optionNode.Params["types"] = true
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
	optionReadme.Params["loggers"] = OptionWithLogger

	optionReadme.Params["optionObserver"] = OptionWithObserver

	optionReadme.Params["optionTypes"] = "false"
	if len(OptionWithMsg) > 0 {
		optionReadme.Params["optionTypes"] = "true"
	}

	cmdargs := []string{}
	for _, arg := range os.Args {
		if strings.ContainsAny(arg, ",") {
			arg = fmt.Sprintf("\"%s\"", arg)
		}
		cmdargs = append(cmdargs, arg)
	}
	optionReadme.Params["args"] = strings.Join(cmdargs, " ")

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
