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
	OptionInit      string
	OptionPath      string
	OptionWithApp   listOptions
	OptionWithSup   listOptions
	OptionWithActor listOptions
	OptionWithWeb   listOptions
	OptionWithTCP   listOptions
	OptionWithUDP   listOptions
	OptionWithSaga  listOptions
	OptionWithStage listOptions
	OptionWithPool  listOptions
	OptionWithMsg   listOptions
	OptionWithCloud string

	root = &item{
		tmpl: nodeTemplate,
	}
	dict   = make(map[string]*item)
	actors = []actor{}
)

func init() {
	fmt.Println(nodeTemplateErr)

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

	actors = []actor{
		actor{OptionWithActor, actorTemplate},
		actor{OptionWithWeb, webTemplate},
		actor{OptionWithTCP, tcpTemplate},
		actor{OptionWithUDP, udpTemplate},
		actor{OptionWithSaga, sagaTemplate},
		actor{OptionWithStage, stageTemplate},
		actor{OptionWithPool, poolTemplate},
	}

	flag.Var(&OptionWithMsg, "with-msg", "add message for the networking")
	flag.StringVar(&OptionWithCloud, "with-cloud", "", "enable cloud with given cluster name")

}

func main() {
	flag.Parse()

	if OptionInit == "" {
		fmt.Println("error: project name is empty")
		return
	}

	root.name = OptionInit
	root.data = nodeTemplateData{Name: root.name, Cloud: OptionWithCloud}

	dir := path.Join(OptionPath, OptionInit)
	if _, err := os.Stat(dir); err == nil {
		fmt.Println("error: '" + dir + "' is already exist")
		return
	}

	if err := parseOptions(); err != nil {
		fmt.Println("error: can't parse options - %s", err)
		return
	}

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("Generating project %q...\n", OptionInit)
	if err := generateProject(dir); err != nil {
		fmt.Printf("error: %s", err)
		return
	}
	fmt.Println("Successfully completed.")
}

func parseOptions() error {
	if err := parseApp(); err != nil {
		return err
	}

	if err := parseSup(); err != nil {
		return err
	}

	for _, actor := range actors {
		if err := parseActor(actor.list, actor.tmpl); err != nil {
			return err
		}
	}

	return nil
}

func parseApp() error {
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
			return err
		}
		appItem := &item{
			app:  app,
			name: app,
			tmpl: appTemplate,
		}

		if _, exist := dict[app]; exist {
			return fmt.Errorf("duplicate name: %q", app)
		}
		root.children = append(root.children, appItem)
		dict[app] = appItem
	}

	return nil
}

func parseSup() error {
	for _, sup := range OptionWithSup {
		s := strings.Split(sup, ":")
		if len(s) > 2 {
			panic("wrong arg:" + sup)
		}
		supItem := &item{
			tmpl: supTemplate,
			name: sup,
		}

		if len(s) == 2 {
			parent, exist := dict[s[0]]
			if exist == false {
				return fmt.Errorf("unknown parent: %q", s[0])
			}
			if strings.HasSuffix(s[1], "Sup") == false {
				fmt.Println("We recomed using suffix 'Sup' for the supervisor names")
			}
			if err := validateName(s[1]); err != nil {
				return err
			}
			supItem.app = parent.app
			supItem.name = s[1]
			parent.children = append(parent.children, supItem)
		}

		fmt.Println("sup: ", sup)
		if _, exist := dict[sup]; exist {
			return fmt.Errorf("duplicate name: %q", sup)
		}
		dict[sup] = supItem
	}

	return nil
}

func parseActor(actors listOptions, tmpl *template.Template) error {
	for _, act := range actors {
		s := strings.Split(act, ":")

		if len(s) > 2 {
			panic("wrong arg:" + act)
		}
		if len(s) == 2 {
			fmt.Println("actor: ", s[1], "in app/sup:", s[0])
			continue
		}
		fmt.Println("actor: ", act)
	}

	for _, msg := range OptionWithMsg {
		fmt.Println("msg: ", msg)
	}
	return nil
}

func validateName(name string) error {
	return nil
}
