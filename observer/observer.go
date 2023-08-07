package main

import (
	"flag"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

func main() {
	flag.Parse()
	opt := gen.NodeOptions{}
	opt.Log.Level = gen.LogLevelDebug
	opt.Applications = append(opt.Applications, observer.CreateApp(observer.Options{}))
	node, err := ergo.StartNode("observer@localhost", opt)
	if err != nil {
		panic(err)
	}
	node.Wait()
}
