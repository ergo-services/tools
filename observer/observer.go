package main

import (
	"flag"
	"fmt"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/node"
)

var (
	OptionNodeName     string
	OptionNodeCookie   string
	OptionObserverPort uint
)

func init() {
	flag.StringVar(&OptionNodeName, "name", "observer@localhost", "Observer node name")
	flag.StringVar(&OptionNodeCookie, "cookie", lib.RandomString(32), "a secret cookie for the network messaging")
	flag.UintVar(&OptionObserverPort, "port", uint(observer.DefaultPort), "Web UI port number")
}

func main() {
	flag.Parse()
	options := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior{
			observer.CreateApp(observer.Options{
				Standalone: true,
				Port:       uint16(OptionObserverPort),
			}),
		},
	}

	options.Log.Level = gen.LogLevelTrace
	options.Network.Cookie = OptionNodeCookie
	options.Network.InsecureSkipVerify = true
	options.Network.Mode = gen.NetworkModeHidden

	name := fmt.Sprintf("observer-%s@localhost", lib.RandomString(6))
	n, err := node.Start(gen.Atom(name), options, ergo.FrameworkVersion)
	if err != nil {
		panic(err)
	}
	n.Wait()
}
