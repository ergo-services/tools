package a2014a

import (
	"errors"

	"ergo.services/ergo/act"
	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
)

type store interface {
	Close() error
}

type conn struct{ open bool }

func (c *conn) Close() error { return nil }

func dial(addr string) (*conn, error) {
	if addr == "" {
		return nil, errors.New("no address")
	}
	return &conn{open: true}, nil
}

// The canonical form: the connection is assigned after the first thing that can fail,
// and Terminate closes it unconditionally.
type Worker struct {
	act.Actor
	name string
	link *conn
}

func (w *Worker) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	link, err := dial("peer:1234")
	if err != nil {
		return err
	}
	w.link = link
	return nil
}

func (w *Worker) Terminate(reason error) {
	w.link.Close() // want `\[tier2\] \[A2014a\] Terminate dereferences link, which Init assigns only after its first fallible step`
}

// The guard is the fix, and the rule has to be silent on it.
type Guarded struct {
	act.Actor
	link *conn
}

func (g *Guarded) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	link, err := dial("peer:1234")
	if err != nil {
		return err
	}
	g.link = link
	return nil
}

func (g *Guarded) Terminate(reason error) {
	if g.link != nil {
		g.link.Close()
	}
}

// A field assigned before anything can fail is set on every path that reaches
// Terminate at all.
type Early struct {
	act.Actor
	link *conn
}

func (e *Early) Init(args ...any) error {
	e.link = &conn{}
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	return nil
}

func (e *Early) Terminate(reason error) {
	e.link.Close()
}

// An Init that cannot return early before its final statement leaves nothing
// conditionally unassigned.
type Infallible struct {
	act.Actor
	link *conn
}

func (i *Infallible) Init(args ...any) error {
	i.link = &conn{}
	return nil
}

func (i *Infallible) Terminate(reason error) {
	i.link.Close()
}

// An interface field faults the same way.
type Interfaced struct {
	act.Actor
	backend store
}

func (i *Interfaced) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	i.backend = &conn{}
	return nil
}

func (i *Interfaced) Terminate(reason error) {
	i.backend.Close() // want `A2014a.*Terminate dereferences backend`
}

// A channel receive and a send both fault on a nil channel.
type Piped struct {
	act.Actor
	done chan struct{}
}

func (p *Piped) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	p.done = make(chan struct{})
	return nil
}

func (p *Piped) Terminate(reason error) {
	close(p.done)
}

// Passing a field somewhere is not a dereference in this frame: a nil pointer argument
// is the callee's problem and the callee is judged on its own.
type Passing struct {
	act.Actor
	link *conn
}

func (p *Passing) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	p.link = &conn{}
	return nil
}

func (p *Passing) Terminate(reason error) {
	release(p.link)
}

func release(c *conn) {
	if c != nil {
		c.Close()
	}
}

// A value field cannot be nil, so it cannot fault.
type Valued struct {
	act.Actor
	count int
	pid   gen.PID
}

func (v *Valued) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	v.count = 1
	return nil
}

func (v *Valued) Terminate(reason error) {
	_ = v.count
	_ = v.pid.Node
}

// A map write on a nil map panics.
type Mapped struct {
	act.Actor
	index map[string]int
}

func (m *Mapped) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	m.index = map[string]int{}
	return nil
}

func (m *Mapped) Terminate(reason error) {
	m.index["terminated"] = 1 // want `A2014a.*Terminate dereferences index`
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	link *conn
}

func (r *Recorded) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	r.link = &conn{}
	return nil
}

func (r *Recorded) Terminate(reason error) {
	//argus:allow A2014a this actor is spawned only by a parent that always passes args
	r.link.Close()
}

// Guarding the last field Init built covers the earlier ones, because Init assigns in
// order: if late is non nil, Init reached the statement after the one that set first.
type Ordered struct {
	act.Actor
	first *conn
	last  *conn
}

func (o *Ordered) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	o.first = &conn{}
	o.last = &conn{}
	return nil
}

func (o *Ordered) Terminate(reason error) {
	if o.last != nil {
		o.first.Close()
		o.last.Close()
	}
}

// The reverse order is not covered: guarding the field Init set first says nothing
// about the one it may never have reached.
type Reversed struct {
	act.Actor
	first *conn
	last  *conn
}

func (r *Reversed) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	r.first = &conn{}
	if r.first == nil {
		return errors.New("dial failed")
	}
	r.last = &conn{}
	return nil
}

func (r *Reversed) Terminate(reason error) {
	if r.first != nil {
		r.last.Close() // want `A2014a.*Terminate dereferences last`
	}
}

// A meta is excluded: SpawnMeta returns as soon as Init reports an error, and the
// goroutine that would eventually call Terminate is never started. This is the shape
// of the framework's own meta/port.go.
type Port struct {
	gen.MetaProcess
	cmd *conn
	in  store
	out store
}

func (p *Port) Init(process gen.MetaProcess) error {
	p.cmd = &conn{}
	link, err := dial("stdin")
	if err != nil {
		return err
	}
	p.in = link
	link, err = dial("stdout")
	if err != nil {
		return err
	}
	p.out = link
	return nil
}

func (p *Port) Start() error { return nil }

func (p *Port) HandleMessage(from gen.PID, message any) error { return nil }

func (p *Port) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (p *Port) Terminate(reason error) {
	if p.cmd != nil {
		p.in.Close()
		p.out.Close()
	}
}

// An application is excluded: the node never calls its Terminate after Init fails or
// times out, and finalize calls it only for an incarnation whose Init succeeded.
type Service struct {
	app.Application
	client *conn
}

func (s *Service) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	link, err := dial("broker")
	if err != nil {
		return err
	}
	s.client = link
	return nil
}

func (s *Service) Terminate(reason error) {
	s.client.Close()
}

// One field is one defect however often Terminate touches it: reported once, at the
// first dereference.
type Repeated struct {
	act.Actor
	link *conn
}

func (r *Repeated) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	r.link = &conn{}
	return nil
}

func (r *Repeated) Terminate(reason error) {
	_ = r.link.open // want `A2014a.*Terminate dereferences link`
	r.link.Close()
	_ = r.link.open
}

type lease struct {
	link  *conn
	until int64
}

func (l *lease) release() {}

// held is set only from a handler, so on a half built receiver it is false: what runs
// under it never meets the nil lease, and neither does anything behind a condition
// that is false at the zero value of a field only a handler writes. Each guard form
// covers its own late field, so each is checked on its own.
type Holder struct {
	act.Actor
	pub    *lease
	stats  *lease
	queue  *lease
	backup *lease
	held   bool
	terms  int
	peers  []gen.PID
	peer   *conn
}

func (h *Holder) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	h.pub = &lease{}
	h.stats = &lease{}
	h.queue = &lease{}
	h.backup = &lease{}
	return nil
}

func (h *Holder) HandleMessage(from gen.PID, message any) error {
	h.held = true
	h.terms++
	h.peers = append(h.peers, from)
	h.peer = &conn{}
	return nil
}

func (h *Holder) Terminate(reason error) {
	if h.held {
		h.pub.release()
		h.pub.link.Close()
	}
	if h.terms > 0 && h.stats.until > 0 {
		h.stats.release()
	}
	if len(h.peers) != 0 {
		h.queue.release()
	}
	if h.peer == nil {
		return
	}
	h.backup.release()
}

// The finding moves to the first dereference the guard does not cover.
type HolderLater struct {
	act.Actor
	pub  *lease
	held bool
}

func (h *HolderLater) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	h.pub = &lease{}
	return nil
}

func (h *HolderLater) HandleMessage(from gen.PID, message any) error {
	h.held = true
	return nil
}

func (h *HolderLater) Terminate(reason error) {
	if h.held {
		h.pub.release()
	}
	h.pub.until = 0 // want `A2014a.*Terminate dereferences pub`
}

// A condition the zero value satisfies says nothing about how far Init got.
type ZeroHolds struct {
	act.Actor
	pub  *lease
	held bool
}

func (z *ZeroHolds) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	z.pub = &lease{}
	return nil
}

func (z *ZeroHolds) HandleMessage(from gen.PID, message any) error {
	z.held = true
	return nil
}

func (z *ZeroHolds) Terminate(reason error) {
	if z.held == false {
		z.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

// A guard field Init can reach is no guard: here Init sets it through a helper before
// the step that fails.
type InitSets struct {
	act.Actor
	pub  *lease
	held bool
}

func (s *InitSets) Init(args ...any) error {
	s.claim()
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	s.pub = &lease{}
	return nil
}

func (s *InitSets) claim() { s.held = true }

func (s *InitSets) Terminate(reason error) {
	if s.held {
		s.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

// Neither is a field the factory sets before the process exists.
type FactorySets struct {
	act.Actor
	pub  *lease
	held bool
}

func factoryFactorySets() gen.ProcessBehavior { return &FactorySets{held: true} }

func (f *FactorySets) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	f.pub = &lease{}
	return nil
}

func (f *FactorySets) Terminate(reason error) {
	if f.held {
		f.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

// Nor one Terminate sets itself: Terminate runs on the half built receiver too.
type SelfSets struct {
	act.Actor
	pub     *lease
	closing bool
}

func (s *SelfSets) Init(args ...any) error {
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	s.pub = &lease{}
	return nil
}

func (s *SelfSets) Terminate(reason error) {
	s.closing = true
	if s.closing {
		s.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

// Nor one whose writer is handed out as a value, since whoever holds it can call it.
type Handed struct {
	act.Actor
	pub    *lease
	held   bool
	onTick func()
}

func (h *Handed) Init(args ...any) error {
	h.onTick = h.mark
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	h.pub = &lease{}
	return nil
}

func (h *Handed) mark() { h.held = true }

func (h *Handed) Terminate(reason error) {
	if h.held {
		h.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

// Nor anything once Init hands its receiver to code the analysis cannot follow.
type Escaping struct {
	act.Actor
	pub  *lease
	held bool
}

func (e *Escaping) Init(args ...any) error {
	register(e)
	if len(args) == 0 {
		return errors.New("no arguments")
	}
	e.pub = &lease{}
	return nil
}

func (e *Escaping) HandleMessage(from gen.PID, message any) error {
	e.held = true
	return nil
}

func (e *Escaping) Terminate(reason error) {
	if e.held {
		e.pub.release() // want `A2014a.*Terminate dereferences pub`
	}
}

func register(b gen.ProcessBehavior) {}
