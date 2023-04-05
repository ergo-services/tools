package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

var (
	OptionInit string
	OptionPath string

	OptionWithApp   listOptions
	OptionWithSup   listOptions
	OptionWithActor listOptions
	OptionWithWeb   listOptions
	OptionWithTCP   listOptions
	OptionWithUDP   listOptions
	OptionWithSaga  listOptions
	OptionWithStage listOptions
	OptionWithPool  listOptions
	OptionWithRaft  listOptions

	OptionWithMsg   listOptions
	OptionWithCloud string
)

func init() {
	flag.StringVar(&OptionInit, "init", "", "initialize project with given name")
	flag.StringVar(&OptionPath, "path", ".", "project location")

	flag.Var(&OptionWithApp, "with-app", "add application")
	flag.Var(&OptionWithSup, "with-sup", "add supervisor")

	flag.Var(&OptionWithActor, "with-actor", "add actor")
	flag.Var(&OptionWithWeb, "with-web", "add Web-server")
	flag.Var(&OptionWithTCP, "with-tcp", "add TCP-server")
	flag.Var(&OptionWithUDP, "with-udp", "add UDP-server")
	flag.Var(&OptionWithSaga, "with-saga", "add Saga")
	flag.Var(&OptionWithStage, "with-stage", "add Stage")
	flag.Var(&OptionWithPool, "with-pool", "add Pool of workers")
	flag.Var(&OptionWithRaft, "with-raft", "add Raft")

	flag.Var(&OptionWithMsg, "with-msg", "add message for the networking")
	flag.StringVar(&OptionWithCloud, "with-cloud", "", "enable cloud with given cluster name")
}

func main() {
	flag.Parse()

	if OptionInit == "" {
		fmt.Println("error: project name is empty")
		return
	}
	dir := path.Join(OptionPath, strings.ToLower(OptionInit))
	list := []*listOptions{
		OptionWithActor.WithTemplates(actorTemplates).WithDir(dir),
		OptionWithWeb.WithTemplates(webTemplates).WithDir(dir),
		OptionWithTCP.WithTemplates(tcpTemplates).WithDir(dir),
		OptionWithUDP.WithTemplates(udpTemplates).WithDir(dir),
		OptionWithSaga.WithTemplates(sagaTemplates).WithDir(dir),
		OptionWithStage.WithTemplates(stageTemplates).WithDir(dir),
		OptionWithPool.WithTemplates(poolTemplates).WithDir(dir),
		OptionWithRaft.WithTemplates(raftTemplates).WithDir(dir),
		OptionWithSup.WithTemplates(supTemplates).WithDir(dir),
		OptionWithApp.WithTemplates(appTemplates).WithDir(dir).WithAppDir("apps"),
	}

	optionNode := Option{
		Name:      strings.ToLower(OptionInit),
		Dir:       dir,
		Package:   "main",
		Templates: nodeTemplates,
		Params:    make(map[string]any),
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
	apps := []*Option{}
	for _, app := range OptionWithApp {
		apps = append(apps, app)
	}
	if len(apps) > 0 {
		optionNode.Params["applications"] = apps
	}
	// node options - cloud
	if OptionWithCloud != "" {
		optionNode.Params["cloud"] = OptionWithCloud
	}
	// node options - register types
	if len(OptionWithMsg) > 0 {
		optionNode.Params["register"] = true
	}
	if err := generate(&optionNode); err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	if err := os.Chdir(optionNode.Dir); err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	fmt.Printf("   generating %q\n", "go.mod")
	cmd := exec.Command("go", "mod", "init", optionNode.Name)
	if err := cmd.Run(); err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	fmt.Printf("   generating %q\n", "go.sum")
	cmd = exec.Command("go", "mod", "tidy")
	if err := cmd.Run(); err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	os.Chdir(currentDir)
	fmt.Println("Successfully completed.")
}
