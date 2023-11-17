package registrar

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"github.com/fsnotify/fsnotify"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func factoryStorage() gen.ProcessBehavior {
	return &storage{
		clusters: make(map[string]*cluster),
		config:   make(map[string]any),
	}
}

type cluster struct {
	name string

	nodeRoutes        map[gen.Atom][]gen.Route
	applicationRoutes map[gen.Atom][]gen.ApplicationRoute
	proxyRoutes       map[gen.Atom][]gen.ProxyRoute
	config            map[string]any
}

type configUpdate struct {
	all     bool
	node    string
	cluster string
	item    string
	value   any
}

type storage struct {
	act.Actor

	configFile string
	watcher    *fsnotify.Watcher
	clusters   map[string]*cluster
	config     map[string]any
}

func (s *storage) Init(args ...any) error {
	var path string

	if v, found := s.Env(ENV_CONFIG_PATH); found {
		s.Log().Debug("got CONFIG PATH env: %v", v)
		path, _ = v.(string)
	}
	s.configFile = filepath.Join(path, "saturn.yaml")

	if err := s.readConfig(false); err != nil {
		return err
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	s.watcher = watcher
	go s.watchConfig()
	return nil
}

func (s *storage) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case StorageUnregister:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to unregister node, unknown cluster: %q", m.Cluster)
			return nil
		}
		if err := cl.unregisterNode(m.Node); err != nil {
			s.Log().Error("unable to unregister node: %s", err)
			return nil
		}
		return nil

	case StorageRegisterApplication:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to register app, unknown cluster: %q", m.Cluster)
			return nil
		}
		cl.registerApp(m.Route)
		return nil

	case StorageUnregisterApplication:
		cl, exist := s.clusters[m.Cluster]
		if exist == false {
			s.Log().Error("unable to register app, unknown cluster: %q", m.Cluster)
			return nil
		}
		cl.unregisterApp(m.Name, m.Node)
		return nil

	case StorageRegisterProxy:
		s.Log().Error("StorageRegisterProxy is unsupported yet")
		return nil

	case StorageUnregisterProxy:
		s.Log().Error("StorageUnregisterProxy is unsupported yet")
		return nil

	case readConfig:
		s.readConfig(true)
		return nil
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
			name:              reg.Cluster,
			nodeRoutes:        make(map[gen.Atom][]gen.Route),
			applicationRoutes: make(map[gen.Atom][]gen.ApplicationRoute),
			proxyRoutes:       make(map[gen.Atom][]gen.ProxyRoute),
			config:            make(map[string]any),
		}
		cl.init()
		s.clusters[reg.Cluster] = cl
	}

	if err := cl.registerNode(reg.Node, reg.Routes); err != nil {
		result.Error = err
		return result, nil
	}

	result.Config = s.getConfig(reg.Node, reg.Cluster)
	result.Nodes = cl.listPeers(reg.Node)
	result.Applications = cl.listApps(reg.Node)

	return result, nil
}

func (s *storage) Terminate(reason error) {
	s.Log().Info("terminated with reason: %s", reason)
	s.watcher.Close()
}

func (s *storage) readConfig(notify bool) error {

	s.Log().Debug("read config %s", s.configFile)
	conf := koanf.New(":")
	if err := conf.Load(file.Provider(s.configFile), yaml.Parser()); err != nil {
		s.Log().Error("unable to read config file %q: %s", s.configFile, err)
		return err
	}

	saturnPrefix := fmt.Sprintf("Saturn:")
	commonClusterPrefix := fmt.Sprintf("Clusters:")
	clusterPrefix := fmt.Sprintf("Cluster@")

	for _, key := range conf.Keys() {
		// "Saturn:..."
		if strings.HasPrefix(key, saturnPrefix) {
			v := conf.Get(key)
			if v == nil {
				s.Log().Warning("value of %q is empty", key)
				continue
			}
			value, _ := s.Node().Env(gen.Env(key))
			if v == value {
				// no changes
				continue
			}
			s.Node().SetEnv(gen.Env(key), v)
			if value != nil {
				s.Log().Warning("value of %q has changed. restart service to take effect", key)
			}
			continue
		}

		if strings.HasPrefix(key, commonClusterPrefix) == false {
			s.Log().Error("unknown config item %q. ignored", key)
			continue
		}

		// "Clusters:..."

		fields := strings.Split(key, ":")
		// field[0] == "Clusters"

		if strings.HasPrefix(fields[1], clusterPrefix) {
			// field[1] has prefix "Cluster@..."
			clname := strings.TrimPrefix(fields[1], clusterPrefix)
			cl, exist := s.clusters[clname]
			if exist == false {
				cl = &cluster{
					name:              clname,
					nodeRoutes:        make(map[gen.Atom][]gen.Route),
					applicationRoutes: make(map[gen.Atom][]gen.ApplicationRoute),
					proxyRoutes:       make(map[gen.Atom][]gen.ProxyRoute),
					config:            make(map[string]any),
				}
				s.clusters[clname] = cl
			}

			itemKey := strings.TrimPrefix(key, fmt.Sprintf("%s:%s:", fields[0], fields[1]))
			nodeName := "*"
			if len(fields) > 2 && strings.Contains(fields[2], "@") {
				nodeName = fields[2]
				itemKey = strings.TrimPrefix(key, fmt.Sprintf("%s:%s:%s:", fields[0], fields[1], fields[2]))
			}
			configKey := fmt.Sprintf("%s:%s:%s", clname, nodeName, itemKey)

			newValue := s.configValue(key, conf, cl.config[configKey])
			if newValue != nil {
				cl.config[configKey] = newValue
				s.Log().Debug("new value for %s in cluster %q: %v", configKey, cl.name, newValue)
				if notify {
					// notify nodes in the cl.name cluster
					update := configUpdate{cluster: cl.name, node: nodeName, item: configKey, value: newValue}
					s.Send(NameRegistrar, update)
				}
			}
			continue
		}

		itemKey := strings.TrimPrefix(key, commonClusterPrefix)
		nodeName := "*"
		if len(fields) > 1 && strings.Contains(fields[1], "@") {
			nodeName = fields[1]
			itemKey = strings.TrimPrefix(key, fmt.Sprintf("%s:%s:", fields[0], fields[1]))
		}
		configKey := fmt.Sprintf("%s:%s", nodeName, itemKey)

		newValue := s.configValue(key, conf, s.config[configKey])
		if newValue != nil {
			s.config[configKey] = newValue
			s.Log().Debug("new value for %s (all clusters): %v", configKey, newValue)
			if notify {
				// notify nodes in all clusters
				update := configUpdate{all: true, node: nodeName, item: configKey, value: newValue}
				s.Send(NameRegistrar, update)
			}
		}

	}

	return nil
}

func (s *storage) configValue(key string, conf *koanf.Koanf, current any) any {
	if path.Ext(key) == ".file" {
		v := conf.String(key)
		if v == "" {
			s.Log().Warning("value of %q is empty, ignored", key)
			return nil
		}

		file_new, err := os.ReadFile(v)
		if err != nil {
			s.Log().Error("unable to read file %s: %s", v, err)
			return nil
		}

		vcurrent := s.config[key]
		if current, _ := vcurrent.([]byte); bytes.Compare(current, file_new) == 0 {
			// no changes. ignore
			return nil
		}

		return file_new

	}
	v := conf.Get(key)
	if v == nil {
		s.Log().Warning("value of %q is empty, ignored", key)
		return nil
	}

	if v == current {
		// no changes. ignore
		return nil
	}
	return v
}

type readConfig struct{}

func (s *storage) watchConfig() {
	// this func is invoked within a new goroutine
	defer s.Log().Debug("config file watcher exited")

	configDir, _ := filepath.Split(s.configFile)
	if err := s.watcher.Add(configDir); err != nil {
		s.Log().Error("unable to watch config file %s: %s", s.configFile, err)
		return
	}

	for {

		select {
		case event, ok := <-s.watcher.Events:
			if ok == false {
				return
			}
			if filepath.Clean(event.Name) != filepath.Clean(s.configFile) {
				continue
			}

			if event.Op == fsnotify.Chmod {
				s.Send(s.PID(), readConfig{})
			}

		case err, ok := <-s.watcher.Errors:
			if ok == false {
				return
			}
			s.Log().Error("watcher got error: %s", err)
			return
		}
	}
}

func (s *storage) getConfig(node gen.Atom, cluster string) map[string]any {
	config := make(map[string]any)

	//	for k, v := range s.config {
	//	}
	return config
}

//
// cluster
//

func (cl *cluster) init() {
	cl.nodeRoutes = make(map[gen.Atom][]gen.Route)
	cl.applicationRoutes = make(map[gen.Atom][]gen.ApplicationRoute)
	cl.proxyRoutes = make(map[gen.Atom][]gen.ProxyRoute)
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
