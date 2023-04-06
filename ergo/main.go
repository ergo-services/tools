package main

import (
	"flag"
	"fmt"
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
	if len(OptionWithApp) > 0 {
		optionNode.Params["applications"] = OptionWithApp
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

	// node options - messages for networking
	msgs := []*Option{}
	optionType := Option{
		Name:      "types",
		Dir:       dir,
		Package:   optionNode.Name,
		Templates: typesTemplates,
		Params:    make(map[string]any),
		Children:  OptionWithMsg,
	}
	for _, msg := range OptionWithMsg {
		msgs = append(msgs, msg)
	}
	if err := generate(&optionType); err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}

	if err := generateGoMod(&optionNode); err != nil {
		fmt.Printf("error: can not generate go.mod file - %s", err)
		return
	}
	fmt.Println("Successfully completed.")
}
