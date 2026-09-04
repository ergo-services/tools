package a2027

import (
	"ergo.services/ergo/gen"
)

// The defect: Start returns at once, so the framework deletes the alias and calls
// Terminate before the parent can send anything.
type Degenerate struct {
	gen.MetaProcess
	ready bool
}

func (d *Degenerate) Init(process gen.MetaProcess) error {
	d.MetaProcess = process
	return nil
}

func (d *Degenerate) Start() error { // want `\[tier2\] \[A2027\] Start returns without blocking`
	d.ready = true
	return nil
}

func (d *Degenerate) HandleMessage(from gen.PID, message any) error { return nil }

func (d *Degenerate) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (d *Degenerate) Terminate(reason error) {}

// The idiom the rule protects: block on a channel Terminate closes.
type Loop struct {
	gen.MetaProcess
	done chan struct{}
}

func (l *Loop) Init(process gen.MetaProcess) error {
	l.MetaProcess = process
	l.done = make(chan struct{})
	return nil
}

func (l *Loop) Start() error {
	<-l.done
	return nil
}

func (l *Loop) HandleMessage(from gen.PID, message any) error { return nil }

func (l *Loop) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (l *Loop) Terminate(reason error) { close(l.done) }

// A call is enough to stay silent: the callee may be the blocking client this meta
// exists to host, and this pass does not pretend to know.
type Server struct {
	gen.MetaProcess
	serve func() error
}

func (s *Server) Init(process gen.MetaProcess) error {
	s.MetaProcess = process
	return nil
}

func (s *Server) Start() error {
	return s.serve()
}

func (s *Server) HandleMessage(from gen.PID, message any) error { return nil }

func (s *Server) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (s *Server) Terminate(reason error) {}
