package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"ergo.services/application/observer"
	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/node"
	"ergo.services/logger/colored"
)

var (
	OptionDefaultCookie string
	OptionObserverPort  uint
	OptionObserverHost  string
	OptionDebug         bool
	OptionVersion       bool
	cookie              string
)

func init() {
	flag.StringVar(&OptionDefaultCookie, "cookie", "", "default cookie for making connection")
	flag.UintVar(&OptionObserverPort, "port", uint(observer.DefaultPort), "web UI port number")
	flag.StringVar(&OptionObserverHost, "host", "localhost", "web UI hostname")
	flag.BoolVar(&OptionDebug, "debug", false, "enable debug mode")
	flag.BoolVar(&OptionVersion, "version", false, "print observer version")
}

func main() {
	flag.Parse()
	options := gen.NodeOptions{
		Applications: []gen.ApplicationBehavior{
			observer.CreateApp(observer.Options{
				Standalone: true,
				Port:       uint16(OptionObserverPort),
				Host:       OptionObserverHost,
			}),
		},
	}

	if envCookie := os.Getenv("COOKIE"); envCookie != "" {
		cookie = envCookie
		OptionDefaultCookie = envCookie
	}

	if OptionDebug {
		options.Log.Level = gen.LogLevelDebug
	}
	options.Network.Cookie = lib.RandomString(32)
	options.Network.InsecureSkipVerify = true
	options.Network.Mode = gen.NetworkModeHidden

	// disable all network features
	options.Network.Flags.Enable = true

	// replace default logger by 'colored'
	options.Log.DefaultLogger.Disable = true
	loggercolored, err := colored.CreateLogger(colored.Options{
		TimeFormat: time.DateTime,
	})
	if err != nil {
		panic(err)
	}
	options.Log.Loggers = append(options.Log.Loggers, gen.Logger{Name: "cl", Logger: loggercolored})

	observer.Version.Name = "Observer Tool"
	options.Version = observer.Version

	if OptionVersion {
		fmt.Println(options.Version)
		return
	}

	name := fmt.Sprintf("observer-%s@localhost", lib.RandomString(6))
	n, err := node.Start(gen.Atom(name), options, ergo.FrameworkVersion)
	if err != nil {
		panic(err)
	}
	if OptionDefaultCookie != cookie {
		n.Log().Warning("it is more secure to use COOKIE environment variable to set default cookie")
	}
	n.Log().Info("open http://%s:%d to inspect nodes", OptionObserverHost, OptionObserverPort)
	n.Wait()
}
