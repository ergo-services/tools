package registrar

import (
	"fmt"
	"path/filepath"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func factoryStorage() gen.ProcessBehavior {
	return &storage{
		clusters: make(map[string]*cluster),
	}
}

type cluster struct {
	name string

	config map[string]any

	nodeRoutes        map[gen.Atom][]gen.Route
	applicationRoutes map[gen.Atom][]gen.ApplicationRoute
	proxyRoutes       map[gen.Atom][]gen.ProxyRoute
}

type storage struct {
	act.Actor

	fc *koanf.Koanf

	clusters map[string]*cluster
}

func (s *storage) Init(args ...any) error {
	var path string

	if v, found := s.Env(ENV_CONFIG_PATH); found {
		s.Log().Debug("got PATH env: %v", v)
		path, _ = v.(string)
	}

	conf := koanf.New(":")
	if err := conf.Load(file.Provider(filepath.Join(path, "saturn.yaml")), yaml.Parser()); err != nil {
		panic(err)
	}
	s.fc = conf

	return nil
}

func (s *storage) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case StorageUnregister:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to unregister node, uknown cluster: %q", m.Cluster)
			return nil
		}
		if err := cl.unregisterNode(m.Node); err != nil {
			s.Log().Error("unable to unregister node: %s", err)
			return nil
		}
		if len(cl.nodeRoutes) == 0 {
			delete(s.clusters, m.Cluster)
		}
		return nil

	case StorageRegisterApplication:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to register app, uknown cluster: %q", m.Cluster)
			return nil
		}
		cl.registerApp(m.Route)
		return nil

	case StorageUnregisterApplication:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to register app, uknown cluster: %q", m.Cluster)
			return nil
		}
		cl.unregisterApp(m.Name, m.Node)
		return nil

	case StorageRegisterProxy:
		panic("todo")

	case StorageUnregisterProxy:
		panic("todo")

	}
	s.Log().Error("got unknown message from %s: %#v", from, message)
	return nil
}

func (s *storage) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	var result StorageRegisterResult

	reg, ok := request.(StorageRegister)
	if ok == false {
		s.Log().Error("got unknown request from %s with reference %s", from, ref)
		return gen.ErrIncorrect, nil
	}

	cl, exist := s.clusters[reg.Cluster]
	if exist == false {
		cl = &cluster{
			name: reg.Cluster,
		}
		cl.init()
		s.clusters[reg.Cluster] = cl
	}

	if err := cl.registerNode(reg.Node, reg.Routes); err != nil {
		result.Error = err
		return result, nil
	}

	result.Config = cl.getConfig(reg.Node)
	result.Nodes = cl.listPeers(reg.Node)
	result.Applications = cl.listApps(reg.Node)

	return result, nil
}

func (s *storage) Terminate(reason error) {
	s.Log().Info("terminated with reason: %s", reason)
}

func (cl *cluster) init() {
	cl.config = make(map[string]any)
	cl.nodeRoutes = make(map[gen.Atom][]gen.Route)
	cl.applicationRoutes = make(map[gen.Atom][]gen.ApplicationRoute)
	cl.proxyRoutes = make(map[gen.Atom][]gen.ProxyRoute)
}

func (cl *cluster) getConfig(node gen.Atom) map[string]any {
	config := make(map[string]any)

	clusterPrefix := fmt.Sprintf("cluster:%s", cl.name)
	nodePrefix := fmt.Sprintf("node:%s", node)

	for k, v := range cl.config {
		if strings.HasPrefix(k, clusterPrefix) || strings.HasPrefix(k, nodePrefix) {
			config[k] = v
			continue
		}
	}
	return config
}

func (cl *cluster) listPeers(node gen.Atom) []gen.Atom {
	var nodes []gen.Atom
	for p, _ := range cl.nodeRoutes {
		if p == node {
			continue
		}
		nodes = append(nodes, p)
	}
	return nodes
}

func (cl *cluster) listApps(skip gen.Atom) []gen.ApplicationRoute {
	var routes []gen.ApplicationRoute
	for _, appRoutes := range cl.applicationRoutes {
		for _, route := range appRoutes {
			if route.Node == skip {
				continue
			}
			routes = append(routes, route)
		}
	}
	return routes
}

func (cl *cluster) registerNode(node gen.Atom, routes []gen.Route) error {
	_, taken := cl.nodeRoutes[node]
	if taken {
		return gen.ErrTaken
	}
	cl.nodeRoutes[node] = routes
	return nil
}

func (cl *cluster) unregisterNode(node gen.Atom) error {
	_, found := cl.nodeRoutes[node]
	if found == false {
		return gen.ErrUnknown
	}

	delete(cl.nodeRoutes, node)

	// remove application route
	updatesApp := make(map[gen.Atom][]gen.ApplicationRoute)
	for app, routes := range cl.applicationRoutes {
		updatedRoutes := []gen.ApplicationRoute{}
		for i := range routes {
			if routes[i].Node == node {
				continue
			}
			updatedRoutes = append(updatedRoutes, routes[i])
		}
		if len(updatedRoutes) == len(routes) {
			continue
		}
		updatesApp[app] = updatedRoutes
	}
	for k, v := range updatesApp {
		cl.applicationRoutes[k] = v
	}

	// remove proxy route
	updatesProxy := make(map[gen.Atom][]gen.ProxyRoute)
	for name, routes := range cl.proxyRoutes {
		updatedRoutes := []gen.ProxyRoute{}
		for i := range routes {
			if routes[i].Proxy == node {
				continue
			}
			updatedRoutes = append(updatedRoutes, routes[i])
		}
		if len(updatedRoutes) == len(routes) {
			continue
		}
		updatesProxy[name] = updatedRoutes
	}
	for k, v := range updatesProxy {
		cl.proxyRoutes[k] = v
	}

	return nil
}

func (cl *cluster) registerApp(route gen.ApplicationRoute) {
	routes := cl.applicationRoutes[route.Name]
	routes = append(routes, route)
	cl.applicationRoutes[route.Name] = routes
}

func (cl *cluster) unregisterApp(name gen.Atom, node gen.Atom) {
	routes := cl.applicationRoutes[name]
	for i := range routes {
		if routes[i].Node != node {
			continue
		}

		routes[0] = routes[i]
		routes = routes[1:]
		cl.applicationRoutes[name] = routes
		return
	}
}
