package main

import (
	"flag"
	"fmt"
)

var (
	OptionInit      string
	OptionPath      string
	OptionWithApp   string
	OptionWithSup   string
	OptionWithActor string
	OptionWithWeb   string
	OptionWithTCP   string
	OptionWithUDP   string
	OptionWithSaga  string
	OptionWithStage string
	OptionWithPool  string
	OptionWithMsg   string
	OptionWithCloud string
)

func init() {
	flag.StringVar(&OptionInit, "init", "", "initialize project with given name")
	flag.StringVar(&OptionPath, "path", ".", "project location")
	flag.StringVar(&OptionWithApp, "with-app", "", "add application")
	flag.StringVar(&OptionWithSup, "with-sup", "", "add supervisor")
	flag.StringVar(&OptionWithActor, "with-actor", "", "add actor")
	flag.StringVar(&OptionWithWeb, "with-web", "", "add Web-server")
	flag.StringVar(&OptionWithTCP, "with-tcp", "", "add TCP-server")
	flag.StringVar(&OptionWithUDP, "with-udp", "", "add UDP-server")
	flag.StringVar(&OptionWithSaga, "with-saga", "", "add Saga")
	flag.StringVar(&OptionWithStage, "with-stage", "", "add Stage")
	flag.StringVar(&OptionWithPool, "with-pool", "", "add Pool of workers")
	flag.StringVar(&OptionWithMsg, "with-msg", "", "add message for the networking")
	flag.StringVar(&OptionWithCloud, "with-cloud", "", "enable cloud with given cluster name")

}
func main() {
	flag.Parse()

	fmt.Println("Hello")
}
