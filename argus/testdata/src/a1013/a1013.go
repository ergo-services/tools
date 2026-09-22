package a1013

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type step struct{}

type Turn struct {
	act.Actor
	pending int
	done    bool
}

// the shape that took a node down: the helper re-enters the decision on "nothing happened",
// and the decision is pure in a state the helper did not change
func (t *Turn) HandleMessage(from gen.PID, message any) error {
	return t.advance()
}

func (t *Turn) advance() error {
	if t.done == true {
		return nil
	}
	return t.dispatch()
}

func (t *Turn) dispatch() error {
	if t.pending == 0 {
		return t.advance() // want `\[tier1\] \[A1013\] dispatch calls advance, which is already running on this callback's stack, so HandleMessage re-enters itself`
	}
	t.pending--
	return nil
}

// direct self recursion is the same defect with one edge
func (t *Turn) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, t.drain()
}

func (t *Turn) drain() error {
	if t.pending == 0 {
		return nil
	}
	t.pending--
	return t.drain() // want `A1013.*drain calls drain, which is already running`
}

// the canonical fix: the next round arrives through the mailbox, so the stack does not grow
type Fixed struct {
	act.Actor
	pending int
}

func (f *Fixed) HandleMessage(from gen.PID, message any) error {
	return f.advance()
}

func (f *Fixed) advance() error {
	if f.pending == 0 {
		return f.Send(f.PID(), step{})
	}
	f.pending--
	return nil
}

// a helper chain with no cycle is silent
type Plain struct {
	act.Actor
	n int
}

func (p *Plain) HandleMessage(from gen.PID, message any) error {
	return p.first()
}

func (p *Plain) first() error  { return p.second() }
func (p *Plain) second() error { p.n++; return nil }

// recursion over the payload on a type that is not the behavior is a different shape
type node struct {
	kids []*node
}

func (n *node) count() int {
	total := 1
	for _, kid := range n.kids {
		total += kid.count()
	}
	return total
}

type Walker struct {
	act.Actor
	root *node
}

func (w *Walker) HandleMessage(from gen.PID, message any) error {
	w.root.count()
	return nil
}

// a recorded exception is silent
type Recorded struct {
	act.Actor
	left int
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	return r.step()
}

func (r *Recorded) step() error {
	if r.left == 0 {
		return nil
	}
	r.left--
	//argus:allow A1013 left is decremented on every pass and the depth is the batch size
	return r.step()
}
