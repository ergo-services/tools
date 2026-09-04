package registrar

import (
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
)

func CreateApp(options Options) gen.ApplicationBehavior {
	return &RegistrarApp{options: options}
}

type Options struct {
	ConfigPath    string
	RegistrarPort uint16
	RegistrarHost string
}

type RegistrarApp struct {
	app.Application
	options Options
}

// Load invoked on loading application using method ApplicationLoad of gen.Node interface.
func (a *RegistrarApp) Load(args ...any) (gen.ApplicationSpec, error) {

	env := make(map[gen.Env]any)
	env[ENV_CONFIG_PATH] = a.options.ConfigPath
	env[ENV_REGISTRAR_PORT] = a.options.RegistrarPort
	env[ENV_REGISTRAR_HOST] = a.options.RegistrarHost

	return gen.ApplicationSpec{
		Name:        "registrar_app",
		Description: "Service Discovery and Configuration Management for your cluster",
		Env:         env,
		Mode:        gen.ApplicationModeTransient,
		Group: []gen.ApplicationMemberSpec{
			{
				Name:    "registrar_sup",
				Factory: factoryRegistrarSup,
			},
		},
	}, nil
}
