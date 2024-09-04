package registrar

import (
	"ergo.services/ergo/gen"
)

const (
	NameRegistrar gen.Atom = "registrar"
	NameConfig    gen.Atom = "config"
	NameStorage   gen.Atom = "storage"

	ENV_CONFIG_PATH     gen.Env = "config_path"
	ENV_REGISTRAR_PORT  gen.Env = "registrar_port"
	ENV_REGISTRAR_HOST  gen.Env = "registrar_host"
	ENV_REGISTRAR_TOKEN gen.Env = "registrar_token"
)

type StorageRegister struct {
	Cluster string
	Node    gen.Atom
	Routes  []gen.Route
}

type StorageRegisterResult struct {
	Error        error
	Config       map[string]any
	Nodes        []gen.Atom
	Applications []gen.ApplicationRoute
}

type StorageUnregister struct {
	Cluster string
	Node    gen.Atom
}

type StorageRegisterApplication struct {
	Cluster string
	Route   gen.ApplicationRoute
}

type StorageUnregisterApplication struct {
	Cluster string
	Name    gen.Atom
	Node    gen.Atom
}

type StorageRegisterProxy struct {
}

type StorageUnregisterProxy struct {
}
