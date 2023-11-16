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
	Cluster string
	Node    gen.Atom
	Routes  []gen.Route
}

type StorageRegisterResult struct {
	Error  error
	Config map[string]any
	Nodes  []gen.Atom
}

type StorageUnregister struct {
	Cluster string
	Node    gen.Atom
}

type StorageRegisterApplication struct {
}

type StorageUnregisterApplication struct {
}

type StorageRegisterProxy struct {
}

type StorageUnregisterProxy struct {
}
