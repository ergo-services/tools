package registrar

import (
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
	options Options
}

// Load invoked on loading application using method ApplicationLoad of gen.Node interface.
func (app *RegistrarApp) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {

	env := make(map[gen.Env]any)
	env[ENV_CONFIG_PATH] = app.options.ConfigPath
	env[ENV_REGISTRAR_PORT] = app.options.RegistrarPort

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

// Start invoked once the application started
func (app *RegistrarApp) Start(mode gen.ApplicationMode) {}

// Terminate invoked once the application stopped
func (app *RegistrarApp) Terminate(reason error) {}
