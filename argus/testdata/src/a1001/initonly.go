package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

// Boot fills its index in Init. Init runs once, before any other callback, so its
// writes are over before a send from anywhere else can happen. A send inside Init is
// a different matter: Init goes on writing after it.
type Boot struct {
	act.Actor
	index   map[string]string
	live    map[string]string
	later   map[string]string
	pending func()
	peer    gen.PID
}

func (b *Boot) Init(args ...any) error {
	b.index = map[string]string{}
	b.index["a"] = "1"
	b.fill()

	index := b.index
	b.Send(b.peer, MessageMap{Meta: index}) // want `\[tier1\] \[A1001\] a1001.MessageMap .*b.index, mutated in place`
	b.index["b"] = "2"

	// a closure left behind by Init runs whenever it is called, long after Init
	b.pending = func() { b.later["x"] = "y" }
	return nil
}

// Only Init calls this, directly, so it runs inside Init too.
func (b *Boot) fill() { b.index["c"] = "3" }

func (b *Boot) HandleMessage(from gen.PID, message any) error {
	index := b.index
	b.Send(b.peer, MessageMap{Meta: index}) // want `\[tier2\] \[A1001\] a1001.MessageMap .*b.index, only ever replaced whole`

	live := b.live
	b.Send(b.peer, MessageMap{Meta: live}) // want `\[tier1\] \[A1001\] a1001.MessageMap .*b.live, mutated in place`

	later := b.later
	b.Send(b.peer, MessageMap{Meta: later}) // want `\[tier1\] \[A1001\] a1001.MessageMap .*b.later, mutated in place`
	return nil
}

func (b *Boot) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	b.live["x"] = "y"
	return nil, nil
}
