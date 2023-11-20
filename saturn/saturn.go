package main

import (
	"flag"

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
)

func init() {
	flag.StringVar(&OptionConfigPath, "path", ".", "path to the config file 'saturn.yaml'")
	flag.UintVar(&OptionPort, "port", 4499, "port number for the registrar service")
	flag.StringVar(&OptionHost, "host", "", "host name for the registrar service")

}

func main() {
	var options gen.NodeOptions

	flag.Parse()

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
	options.Log.Level = gen.LogLevelDebug

	loggercolored, err := colored.CreateLogger(colored.Options{})
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
