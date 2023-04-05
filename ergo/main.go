package main

import (
	"flag"
	"fmt"
	"html/template"
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
	//
	//
	//	root = &item{
	//		tmpl: nodeTemplates,
	//		name: strings.ToLower(OptionInit),
	//		pkg:  "main",
	//	}
	//
	// // parse application options
	//
	//	if err := parse(root, OptionWithApp); err != nil {
	//		fmt.Printf("error: can't parse application options - %s\n", err)
	//		return
	//	}
	//
	// // parse supervisor options
	//
	//	if err := parse(); err != nil {
	//		fmt.Printf("error: can't parse supervisor options - %s\n", err)
	//		return
	//	}
	//
	// // parse actors
	//
	//	for _, actor := range actors {
	//		if err := parseActor(actor.list, actor.tmpl); err != nil {
	//			fmt.Printf("error: can't parse actor options - %s\n", err)
	//		}
	//	}
	//
	// // parse messages
	//
	//	if err := parseMsg(); err != nil {
	//		fmt.Printf("error: can't parse message options - %s\n", err)
	//		return
	//	}
	//
	//	data := nodeTemplateData{
	//		Package:       root.pkg,
	//		Name:          root.name,
	//		Cloud:         OptionWithCloud,
	//		Applications:  apps,
	//		RegisterTypes: len(OptionWithMsg) > 0,
	//	}
	//
	//	for _, c := range root.children {
	//		// do not add process if it belongs to app
	//		if c.app != "" {
	//			continue
	//		}
	//		data.Processes = append(data.Processes, c.name)
	//	}
	//
	// root.data = data
	//
	//	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
	//		fmt.Printf("error: %s\n", err)
	//		return
	//	}
	//
	// fmt.Printf("Generating project %q...\n", root.name)
	//
	//	if err := generateProject(root, dir); err != nil {
	//		fmt.Printf("error: %s\n", err)
	//		return
	//	}
	//
	// fmt.Println("Successfully completed.")
}

func parseApp() ([]string, error) {
	apps := []string{}
	//
	//	for _, app := range OptionWithApp {
	//		// make sure this name is capitalized
	//		app1 := strings.Title(app)
	//		if app != app1 {
	//			fmt.Printf("warning: Application name %q has been updated to %q", app, app1)
	//			app = app1
	//		}
	//		if strings.HasSuffix(app, "App") == false {
	//			fmt.Println("warning: We recomed using suffix 'App' for the application names")
	//		}
	//
	//		if err := validateName(app); err != nil {
	//			return apps, err
	//		}
	//		data := appTemplateData{
	//			Package: strings.ToLower(app),
	//			Name:    app,
	//		}
	//		appItem := &item{
	//			pkg:  strings.ToLower(app),
	//			app:  app,
	//			name: app,
	//			tmpl: appTemplates,
	//			data: data,
	//		}
	//
	//		apps = append(apps, app)
	//
	//		if _, exist := dict[app]; exist {
	//			return apps, fmt.Errorf("duplicate name: %q", app)
	//		}
	//		dict[app] = appItem
	//		root.children = append(root.children, appItem)
	//	}
	//
	return apps, nil
}

func parseSup() error {
	//	for _, sup := range OptionWithSup {
	//
	//		supItem := &item{
	//			tmpl: supTemplates,
	//			name: sup,
	//		}
	//		parent := root
	//
	//		// if it has parent process
	//		s := strings.Split(sup, ":")
	//		if len(s) > 2 {
	//			panic("wrong arg:" + sup)
	//		}
	//
	//		if len(s) == 2 {
	//			p, exist := dict[s[0]]
	//			if exist == false {
	//				return fmt.Errorf("unknown parent: %q", s[0])
	//			}
	//
	//			if strings.HasSuffix(s[1], "Sup") == false {
	//				fmt.Println("We recomed using suffix 'Sup' for the supervisor names")
	//			}
	//
	//			if err := validateName(s[1]); err != nil {
	//				return err
	//			}
	//			parent = p
	//			supItem.name = s[1]
	//		}
	//
	//		// check for duplicates
	//		if _, exist := dict[supItem.name]; exist {
	//			return fmt.Errorf("duplicate name: %q", supItem.name)
	//		}
	//
	//		supItem.app = parent.app
	//		supItem.pkg = parent.pkg
	//		data := appTemplateData{
	//			Package: parent.pkg,
	//			Name:    supItem.name,
	//		}
	//		supItem.data = data
	//		parent.children = append(parent.children, supItem)
	//		fmt.Println("sup: ", sup)
	//
	//		dict[supItem.name] = supItem
	//	}

	return nil
}

func parseActor(actors listOptions, tmpl []*template.Template) error {
	//	for _, act := range actors {
	//		// check for duplicates
	//		if _, exist := dict[act]; exist {
	//			return fmt.Errorf("duplicate name: %q", act)
	//		}
	//
	//		actItem := &item{
	//			tmpl: tmpl,
	//			name: act,
	//		}
	//		parent := root
	//		s := strings.Split(act, ":")
	//
	//		if len(s) > 2 {
	//			panic("wrong arg:" + act)
	//		}
	//		if len(s) == 2 {
	//			p, exist := dict[s[0]]
	//			if exist == false {
	//				return fmt.Errorf("unknown parent: %q", s[0])
	//			}
	//
	//			if err := validateName(s[1]); err != nil {
	//				return err
	//			}
	//			parent = p
	//			actItem.name = s[1]
	//		}
	//
	//		// check for duplicates
	//		if _, exist := dict[actItem.name]; exist {
	//			return fmt.Errorf("duplicate name: %q", actItem.name)
	//		}
	//		actItem.app = parent.app
	//		actItem.pkg = parent.pkg
	//		data := actorTemplateData{
	//			Package: parent.pkg,
	//			Name:    actItem.name,
	//		}
	//		actItem.data = data
	//		parent.children = append(parent.children, actItem)
	//		fmt.Println("actor: ", act)
	//
	//		dict[actItem.name] = actItem
	//	}
	//
	//	for _, msg := range OptionWithMsg {
	//		fmt.Println("msg: ", msg)
	//	}
	//
	return nil
}

func parseMsg() error {
	for _, msg := range OptionWithMsg {
		fmt.Println(msg)
	}

	return nil
}

func validateName(name string) error {
	return nil
}
