package a2026

import (
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	peer gen.PID
	name gen.Atom
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// The runtime refuses an exit signal to this process, its parent and its leader.
	w.SendExit(w.PID(), gen.ErrUnknown)    // want `\[tier2\] \[A2026\] SendExit to w.PID\(\) always answers gen.ErrNotAllowed`
	w.SendExit(w.Parent(), gen.ErrUnknown) // want `A2026.*SendExit to w.Parent\(\) always answers gen.ErrNotAllowed`
	w.SendExit(w.Leader(), gen.ErrUnknown) // want `A2026.*SendExit to w.Leader\(\) always answers gen.ErrNotAllowed`
	w.SendExit(w.peer, gen.ErrUnknown)

	// A nil reason is not a termination reason.
	w.SendExit(w.peer, nil)                   // want `A2026.*SendExit with a nil reason`
	w.SendExitAfter(w.peer, nil, time.Second) // want `A2026.*SendExitAfter with a nil reason`
	w.SendExitMeta(gen.Alias{}, nil)          // want `A2026.*SendExitMeta with a nil reason`
	w.SendExitAfter(w.peer, gen.ErrUnknown, time.Second)

	// A process cannot link to or monitor itself.
	w.Link(w.PID())       // want `A2026.*Link to w.PID\(\) always answers gen.ErrNotAllowed`
	w.MonitorPID(w.PID()) // want `A2026.*MonitorPID to w.PID\(\) always answers gen.ErrNotAllowed`
	w.LinkPID(w.peer)

	// The address parameter is any, so a plain string compiles and then fails.
	w.Send("worker", MessageTick{}) // want `A2026.*Send takes the address as any, and a string is not one`
	w.Call("worker", MessageTick{}) // want `A2026.*Call takes the address as any, and a string is not one`
	w.Send(w.name, MessageTick{})
	w.Send(w.peer, MessageTick{})
	w.Send(gen.ProcessID{Name: "worker"}, MessageTick{})
	w.Send(gen.Alias{}, MessageTick{})
	return nil
}

func (w *Worker) Init(args ...any) error {
	// Below the framework floor the setter refuses and leaves the old value.
	w.SetCompressionThreshold(256) // want `A2026.*a threshold of 256 is below the framework floor of 1024`
	w.SetCompressionThreshold(4096)

	w.SetSendPriority(9) // want `A2026.*9 is outside the range SetSendPriority accepts`
	w.SetSendPriority(gen.MessagePriorityHigh)
	w.SetCompressionLevel(7) // want `A2026.*7 is outside the range SetCompressionLevel accepts`
	w.SetCompressionLevel(gen.CompressionBestSize)
	return nil
}

type MessageTick struct{ N int64 }

// Outside the framework surface a method of the same name is ordinary code.
type helper struct{}

func (h *helper) Send(to any, message any) error { return nil }

func use(h *helper) { h.Send("anything", 1) }
