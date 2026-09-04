package a2013

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type MessagePoint struct{ V int }

// A producer in this same package is the one user case where the buffer size is
// visible at the subscription site.
type Producer struct {
	act.Actor
}

func (p *Producer) Init(args ...any) error {
	p.RegisterEvent("points", gen.EventOptions{Buffer: 10})
	p.RegisterEvent("unbuffered", gen.EventOptions{Notify: true})
	return nil
}

// A consumer that drops the catch-up buffer loses everything published before it
// subscribed.
type Consumer struct {
	act.Actor
}

func (c *Consumer) Init(args ...any) error {
	c.MonitorEvent(gen.Event{Name: "points", Node: "node@localhost"}) // want `\[tier2\] \[A2013\] the event buffer returned by MonitorEvent is discarded, but "points" is a producer in this package buffering 10 messages`

	// The blanked form is the same discard.
	_, _ = c.LinkEvent(gen.Event{Name: "points"}) // want `A2013.*the event buffer returned by LinkEvent is discarded`

	// The framework's node event bus is buffered at 1000 on every node.
	c.MonitorEvent(gen.Event{Name: "core"}) // want `A2013.*"core" is the node event bus, gen.CoreEvent buffering 1000 messages`

	// Keeping the result is the fix, and the compiler proves it is used.
	buf, err := c.LinkEvent(gen.Event{Name: "points"})
	if err == nil {
		for range buf {
		}
	}

	// An unbuffered producer returns nil, so there is nothing to lose.
	c.MonitorEvent(gen.Event{Name: "unbuffered"})

	// An inspect event is buffered at one and re-published every period, so a discard
	// delays a snapshot rather than losing one.
	c.MonitorEvent(gen.Event{Name: "inspect_process_list"})

	// A name nobody in this package registers: the producer is unreachable from here.
	c.MonitorEvent(gen.Event{Name: "somebody_elses_event"})

	// A name that is not a literal at all.
	c.MonitorEvent(c.watched())

	// A local with exactly one assignment resolves.
	ev := gen.Event{Name: "points"}
	c.MonitorEvent(ev) // want `A2013.*the event buffer returned by MonitorEvent is discarded`

	// A recorded exception is silent.
	//argus:allow A2013 this subscriber only cares about what happens from now on
	c.MonitorEvent(gen.Event{Name: "points"})
	return nil
}

func (c *Consumer) HandleEvent(event gen.MessageEvent) error { return nil }

func (c *Consumer) watched() gen.Event { return gen.Event{Name: "points"} }

// A subscription taken for the down signal alone has no consumer to feed, so dropping
// the buffer loses nothing this behavior would have read.
type Watcher struct {
	act.Actor
}

func (w *Watcher) Init(args ...any) error {
	w.MonitorEvent(gen.Event{Name: "points"})
	return nil
}
