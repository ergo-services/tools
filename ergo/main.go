package main

import (
	"flag"
	"fmt"
	"html/template"
	"os"
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

	root = &item{
		tmpl: nodeTemplates,
	}
	dict   = make(map[string]*item)
	actors = []actor{}
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

	actors = []actor{
		actor{OptionWithActor, actorTemplates},
		actor{OptionWithWeb, webTemplates},
		actor{OptionWithTCP, tcpTemplates},
		actor{OptionWithUDP, udpTemplates},
		actor{OptionWithSaga, sagaTemplates},
		actor{OptionWithStage, stageTemplates},
		actor{OptionWithPool, poolTemplates},
		actor{OptionWithRaft, raftTemplates},
	}

	if OptionInit == "" {
		fmt.Println("error: project name is empty")
		return
	}

	dir := path.Join(OptionPath, OptionInit)
	if _, err := os.Stat(dir); err == nil {
		fmt.Println("error: '" + dir + "' is already exist")
		return
	}

	// parse application options
	apps, err := parseApp()
	if err != nil {
		fmt.Printf("error: can't parse application options - %s\n", err)
		return
	}

	// parse supervisor options
	if err := parseSup(); err != nil {
		fmt.Printf("error: can't parse supervisor options - %s\n", err)
		return
	}

	// parse the rest
	for _, actor := range actors {
		if err := parseActor(actor.list, actor.tmpl); err != nil {
			fmt.Printf("error: can't parse actor options - %s\n", err)
		}
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}

	root.name = OptionInit
	data := nodeTemplateData{
		Name:          root.name,
		Cloud:         OptionWithCloud,
		Applications:  apps,
		RegisterTypes: len(OptionWithMsg) > 0,
	}
	for _, c := range root.children {
		data.Processes = append(data.Processes, c.name)
	}
	root.data = data

	fmt.Printf("Generating project %q...\n", OptionInit)
	if err := generateProject(root, dir); err != nil {
		fmt.Printf("error: %s\n", err)
		return
	}
	fmt.Println("Successfully completed.")
}

func parseApp() ([]string, error) {
	apps := []string{}
	for _, app := range OptionWithApp {
		// make sure this name is capitalized
		app1 := strings.Title(app)
		if app != app1 {
			fmt.Printf("warning: Application name %q has been updated to %q", app, app1)
			app = app1
		}
		if strings.HasSuffix(app, "App") == false {
			fmt.Println("warning: We recomed using suffix 'App' for the application names")
		}

		if err := validateName(app); err != nil {
			return apps, err
		}
		appItem := &item{
			app:  app,
			name: app,
			tmpl: appTemplates,
		}

		apps = append(apps, app)

		if _, exist := dict[app]; exist {
			return apps, fmt.Errorf("duplicate name: %q", app)
		}
		dict[app] = appItem
	}

	return apps, nil
}

func parseSup() error {
	for _, sup := range OptionWithSup {
		// check for duplicates
		if _, exist := dict[sup]; exist {
			return fmt.Errorf("duplicate name: %q", sup)
		}

		supItem := &item{
			tmpl: supTemplates,
			name: sup,
		}
		parent := root

		// if it has parent process
		s := strings.Split(sup, ":")
		if len(s) > 2 {
			panic("wrong arg:" + sup)
		}

		if len(s) == 2 {
			p, exist := dict[s[0]]
			if exist == false {
				return fmt.Errorf("unknown parent: %q", s[0])
			}

			if strings.HasSuffix(s[1], "Sup") == false {
				fmt.Println("We recomed using suffix 'Sup' for the supervisor names")
			}

			if err := validateName(s[1]); err != nil {
				return err
			}
			parent = p
			supItem.name = s[1]
		}

		supItem.app = parent.app
		parent.children = append(parent.children, supItem)
		fmt.Println("sup: ", sup)

		dict[supItem.name] = supItem
	}

	return nil
}

func parseActor(actors listOptions, tmpl []*template.Template) error {
	for _, act := range actors {
		// check for duplicates
		if _, exist := dict[act]; exist {
			return fmt.Errorf("duplicate name: %q", act)
		}

		actItem := &item{
			tmpl: tmpl,
			name: act,
		}
		parent := root
		s := strings.Split(act, ":")

		if len(s) > 2 {
			panic("wrong arg:" + act)
		}
		if len(s) == 2 {
			p, exist := dict[s[0]]
			if exist == false {
				return fmt.Errorf("unknown parent: %q", s[0])
			}

			if err := validateName(s[1]); err != nil {
				return err
			}
			parent = p
			actItem.name = s[1]
		}
		actItem.app = parent.app
		parent.children = append(parent.children, actItem)
		fmt.Println("actor: ", act)

		dict[actItem.name] = actItem
	}

	for _, msg := range OptionWithMsg {
		fmt.Println("msg: ", msg)
	}
	return nil
}

func validateName(name string) error {
	return nil
}
