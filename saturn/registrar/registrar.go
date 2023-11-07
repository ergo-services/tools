package registrar

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/lib"
	"ergo.services/ergo/meta"
	"ergo.services/ergo/net/edf"

	"ergo.services/registrar/saturn"
)

func factoryRegistrar() gen.ProcessBehavior {
	return &Registrar{}
}

type Registrar struct {
	act.Actor

	token  string
	conns  map[gen.Alias]*connection
	chunks map[gen.Alias][]byte
}

type RegistrarArgs struct {
	Port  uint16
	Token string
}

const (
	connStateHandshake  int = 0
	connStateRegister   int = 1
	connStateRegistered int = 2
)

var (
	errIncorrectState  = fmt.Errorf("incorrect state")
	errIncorrectDigest = fmt.Errorf("incorrect digest")
	errTooSlow         = fmt.Errorf("too slow client")
)

type connection struct {
	state   int
	node    gen.Atom
	cluster string
	ip      net.Addr

	checkCancel gen.CancelFunc
}

type checkRegistration struct {
	ID gen.Alias
}

// Init invoked on a start this process.
func (r *Registrar) Init(args ...any) error {
	rargs := args[0].(RegistrarArgs)
	r.token = rargs.Token

	// create meta tcp server
	tcpOptions := meta.TCPOptions{
		Port:            rargs.Port,
		CertManager:     r.Node().CertManager(),
		KeepAlivePeriod: time.Second * 3,
	}

	metatcp, err := meta.CreateTCP(tcpOptions)
	if err != nil {
		r.Log().Error("unable to create TCP server meta-process: %s", err)
		return err
	}

	// spawn this meta process
	id, err := r.SpawnMeta(metatcp, gen.MetaOptions{})
	if err != nil {
		r.Log().Error("unable to spawn TCP server meta-process: %s", err)
		metatcp.Terminate(err)
		return err
	}

	r.conns = make(map[gen.Alias]*connection)
	r.chunks = make(map[gen.Alias][]byte)

	r.Log().Info("started registrar server on %s:%d (meta-process: %s)", tcpOptions.Host, tcpOptions.Port, id)
	return nil
}

// HandleMessage receives a message on a new connection, data packet, or disconnection.
// To serve the new TCP connection, the meta-process of the TCP server spawns the new meta-process.
func (r *Registrar) HandleMessage(from gen.PID, message any) error {
	switch m := message.(type) {
	case meta.MessageTCPConnect:
		r.Log().Debug("new connection with: %s (serving meta-process: %s)", m.RemoteAddr, m.ID)
		cancel, err := r.SendAfter(r.PID(), checkRegistration{ID: m.ID}, time.Second*3)
		if err != nil {
			r.SendExitMeta(m.ID, err)
			return nil
		}
		r.conns[m.ID] = &connection{
			ip:          m.RemoteAddr,
			checkCancel: cancel,
		}

	case meta.MessageTCPDisconnect:
		r.Log().Debug("terminated connection (serving meta-process: %s)", m.ID)
		if conn, found := r.conns[m.ID]; found {
			if err := r.Send(NameStorage, StorageUnregister{conn.node}); err != nil {
				r.Log().Error("unable to unregister node in the storage: %s", err)
				// try one more time in a second
				r.SendAfter(NameStorage, StorageUnregister{conn.node}, time.Second)
			}
			delete(r.conns, m.ID)
			delete(r.chunks, m.ID)
			r.Log().Debug("unregistered node %s in cluster %q (serving meta-process: %s)", conn.node, conn.cluster, m.ID)
		}

	case meta.MessageTCP:
		conn, found := r.conns[m.ID]
		if found == false {
			return nil
		}
		chunk := r.chunks[m.ID]
		chunk = append(chunk, m.Data...)
		r.chunks[m.ID] = chunk

		if len(chunk) < 5 {
			// wait more data
			return nil
		}
		if chunk[0] != saturn.Proto {
			r.SendExitMeta(m.ID, fmt.Errorf("unknown proto: %d", chunk[0]))
			return nil
		}
		if chunk[1] != saturn.ProtoVersion {
			r.SendExitMeta(m.ID, fmt.Errorf("unknown proto: %d", chunk[0]))
			return nil
		}
		l := int(binary.BigEndian.Uint16(chunk[2:4]))
		if 4+l > len(chunk) {
			// wait more data
			return nil
		}

		v, ch, err := edf.Decode(chunk[4:], edf.Options{})
		if err != nil {
			r.SendExitMeta(m.ID, err)
			return nil
		}
		r.chunks[m.ID] = ch

		switch sm := v.(type) {
		case saturn.MessageHandshake:
			if conn.state != connStateHandshake {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if err := r.handleHandshake(m.ID, sm); err != nil {
				r.SendExitMeta(m.ID, err)
				return nil
			}
			conn.state = connStateRegister
			r.Log().Debug("handshaked (serving meta-process: %s)", m.ID)

		case saturn.MessageRegister:
			if conn.state != connStateRegister {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if err := r.handleRegister(m.ID, sm); err != nil {
				r.SendExitMeta(m.ID, err)
				return nil
			}
			conn.state = connStateRegistered
			conn.cluster = sm.Cluster
			conn.node = sm.Node
			conn.checkCancel() // cancel timer

		case saturn.MessageRegisterProxy:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if sm.Route.Proxy != conn.node {
				r.Log().Error("unable to register proxy route: node name mismatch (exp: %s, got: %s)",
					conn.node, sm.Route.Proxy)
				return nil
			}
			r.handleRegisterProxy(sm.Route)

		case saturn.MessageUnregisterProxy:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if sm.Route.Proxy != conn.node {
				r.Log().Error("unable to unregister proxy route: node name mismatch (exp: %s, got: %s)",
					conn.node, sm.Route.Proxy)
				return nil
			}
			r.handleUnregisterProxy(sm.Route)

		case saturn.MessageRegisterApplication:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if sm.Route.Node != conn.node {
				r.Log().Error("unable to register application: node name mismatch (exp: %s, got: %s)",
					conn.node, sm.Route.Node)
				return nil
			}
			r.handleRegisterApplication(sm.Route)

		case saturn.MessageUnregisterApplication:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			r.handleUnregisterApplication(sm.Name, conn.node)

		case saturn.MessageResolve:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if err := r.handleResolve(m.ID, sm); err != nil {
				r.SendExitMeta(m.ID, err)
			}

		case saturn.MessageResolveProxy:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if err := r.handleResolveProxy(m.ID, sm); err != nil {
				r.SendExitMeta(m.ID, err)
			}

		case saturn.MessageResolveApplication:
			if conn.state != connStateRegistered {
				r.SendExitMeta(m.ID, errIncorrectState)
				return nil
			}
			if err := r.handleResolveApplication(m.ID, sm); err != nil {
				r.SendExitMeta(m.ID, err)
			}

		default:
			r.Log().Error("unknown message %#v", v)
		}

	case checkRegistration:
		conn, found := r.conns[m.ID]
		if found == false {
			return nil
		}
		if conn.state != connStateRegistered {
			r.SendExitMeta(m.ID, errTooSlow)
			return nil
		}

	default:
		r.Log().Debug("got unknown message from %s: %#v", from, message)
	}
	return nil
}

func (r *Registrar) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	r.Log().Info("got request from %s with reference %s", from, ref)
	return gen.Atom("pong"), nil
}

func (r *Registrar) HandleEvent(event gen.MessageEvent) error {
	r.Log().Info("received event %s", event)
	return nil
}

// Terminate invoked on a termination process
func (r *Registrar) Terminate(reason error) {
	r.Log().Info("terminated with reason: %s", reason)
	if err := r.Node().NetworkStop(); err != nil {
		r.Log().Error("unable to stop network stack: %s", err)
	}
}

func (r *Registrar) handleHandshake(mp gen.Alias, message saturn.MessageHandshake) error {
	hash := sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", message.Salt, r.token)))

	if message.Digest != fmt.Sprintf("%x", hash.Sum(nil)) {
		return errIncorrectDigest
	}

	hash = sha256.New()
	hash.Write([]byte(fmt.Sprintf("%s:%s", message.Digest, r.token)))
	result := saturn.MessageHandshakeResult{
		Digest: fmt.Sprintf("%x", hash.Sum(nil)),
	}

	buf := lib.TakeBuffer()

	buf.Allocate(4)
	buf.B[0] = saturn.Proto
	buf.B[1] = saturn.ProtoVersion

	if err := edf.Encode(result, buf, edf.Options{}); err != nil {
		return err
	}

	binary.BigEndian.PutUint16(buf.B[2:4], uint16(buf.Len()-4))
	reply := meta.MessageTCP{
		ID:   mp,
		Data: buf.B,
	}
	return r.SendAlias(mp, reply)
}

func (r *Registrar) handleRegister(mp gen.Alias, message saturn.MessageRegister) error {

	// TODO try to register this node

	r.Log().Debug("registered node %s in cluster %q (serving meta-process: %s)", message.Node, message.Cluster, mp)
	for _, appRoute := range message.Routes.Applications {
		if appRoute.Node != message.Node {
			r.Log().Error("unable to register application: node name mismatch (exp: %s, got: %s)",
				message.Node, appRoute.Node)
			continue
		}
		r.handleRegisterApplication(appRoute)
	}
	result := saturn.MessageRegisterResult{}

	buf := lib.TakeBuffer()

	buf.Allocate(4)
	buf.B[0] = saturn.Proto
	buf.B[1] = saturn.ProtoVersion

	if err := edf.Encode(result, buf, edf.Options{}); err != nil {
		return err
	}

	binary.BigEndian.PutUint16(buf.B[2:4], uint16(buf.Len()-4))
	reply := meta.MessageTCP{
		ID:   mp,
		Data: buf.B,
	}
	return r.SendAlias(mp, reply)
}

func (r *Registrar) handleRegisterProxy(route gen.ProxyRoute) error {
	r.Log().Debug("registered proxy route to %s via node %s", route.To, route.Proxy)
	// no reply for this message
	return nil
}

func (r *Registrar) handleUnregisterProxy(route gen.ProxyRoute) error {
	r.Log().Debug("unregistered proxy route to %s via node %s", route.To, route.Proxy)
	// no reply for this message
	return nil
}

func (r *Registrar) handleRegisterApplication(route gen.ApplicationRoute) {

	r.Log().Debug("registered application %s on node %s", route.Name, route.Node)
	// no reply for this message
	return

}

func (r *Registrar) handleUnregisterApplication(name gen.Atom, node gen.Atom) {
	r.Log().Debug("unregistered application %s on node %s", name, node)
	// no reply for this message
	return
}

func (r *Registrar) handleResolve(id gen.Alias, message saturn.MessageResolve) error {
	return nil

}

func (r *Registrar) handleResolveProxy(id gen.Alias, message saturn.MessageResolveProxy) error {
	return nil

}

func (r *Registrar) handleResolveApplication(id gen.Alias, message saturn.MessageResolveApplication) error {
	return nil

}