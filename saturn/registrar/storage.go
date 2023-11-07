package registrar

import (
	"path/filepath"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func factoryStorage() gen.ProcessBehavior {
	return &storage{
		config:   make(map[string]any),
		clusters: make(map[string]cluster),
	}
}

type cluster struct {
	nodes        map[gen.Atom][]gen.Route            // node => routes
	applications map[gen.Atom][]gen.ApplicationRoute // application => routes
	proxies      map[gen.Atom][]gen.ProxyRoute       // to => routes
}

type storage struct {
	act.Actor

	fc     *koanf.Koanf
	config map[string]any

	clusters map[string]cluster
}

func (s *storage) Init(args ...any) error {
	var path string
	if v, found := s.Env(ENV_CONFIG_PATH); found {
		s.Log().Debug("got PATH env: %v", v)
	}
	conf := koanf.New(":")
	if err := conf.Load(file.Provider(filepath.Join(path, "saturn.yaml")), yaml.Parser()); err != nil {
		panic(err)
	}

	s.fc = conf
	return nil
}

func (s *storage) HandleMessage(from gen.PID, message any) error {
	s.Log().Info("got unknown message from %s: %#v", from, message)
	return nil
}

func (s *storage) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	s.Log().Info("got request from %s with reference %s", from, ref)
	return gen.Atom("pong"), nil
}

func (s *storage) Terminate(reason error) {
	s.Log().Info("terminated with reason: %s", reason)
}

func (s *storage) HandleMessageName(name gen.Atom, from gen.PID, message any) error {
	return nil
}
