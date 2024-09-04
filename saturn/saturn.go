package main

import (
	"flag"
	"fmt"
	"runtime/debug"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/logger/colored"

	"ergo.services/tools/saturn/registrar"
)

var (
	OptionConfigPath string
	OptionPort       uint
	OptionHost       string
	OptionDebug      bool
	OptionVersion    bool
)

func init() {
	flag.StringVar(&OptionConfigPath, "path", ".", "path to the config file 'saturn.yaml'")
	flag.UintVar(&OptionPort, "port", 4499, "port number for the registrar service")
	flag.StringVar(&OptionHost, "host", "", "host name for the registrar service")
	flag.BoolVar(&OptionDebug, "debug", false, "enable debug mode")
	flag.BoolVar(&OptionVersion, "version", false, "print version")

}

func main() {
	var options gen.NodeOptions

	flag.Parse()

	if OptionVersion {
		fmt.Println(Version)
		return
	}

	regOptions := registrar.Options{
		ConfigPath:    OptionConfigPath,
		RegistrarPort: uint16(OptionPort),
		RegistrarHost: OptionHost,
	}
	apps := []gen.ApplicationBehavior{
		registrar.CreateApp(regOptions),
	}
	options.Applications = apps

	// use self-signed cert on start. will be replaced by the cert from the config
	cert, err := lib.GenerateSelfSignedCert(Version.String())
	if err != nil {
		panic(err)
	}
	options.CertManager = gen.CreateCertManager(cert)
	options.Log.DefaultLogger.Disable = true
	if OptionDebug {
		options.Log.Level = gen.LogLevelDebug
	}

	loggercolored, err := colored.CreateLogger(colored.Options{
		TimeFormat: time.DateTime,
	})
	if err != nil {
		panic(err)
	}
	options.Log.Loggers = append(
		options.Log.Loggers,
		gen.Logger{Name: "colored", Logger: loggercolored},
	)

	options.Network.Mode = gen.NetworkModeDisabled
	options.Version = Version

	node, err := ergo.StartNode("saturn@localhost", options)
	if err != nil {
		panic(err)
	}

	node.Wait()
}

func init() {

	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				Version.Commit = setting.Value
				break
			}
		}
	}
}
