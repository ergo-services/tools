package registrar

import "ergo.services/ergo/gen"

const (
	NameRegistrar gen.Atom = "registrar"
	NameConfig    gen.Atom = "config"
	NameStorage   gen.Atom = "storage"

	ENV_CONFIG_PATH    gen.Env = "config_path"
	ENV_REGISTRAR_PORT gen.Env = "registrar_port"
)

type StorageRegister struct {
	Node   gen.Atom
	Routes []gen.Route
}

type StorageRegisterResult struct {
	Error  error
	Event  gen.Event
	Config map[string]any
}

type StorageUnregister struct {
	Node gen.Atom
}
