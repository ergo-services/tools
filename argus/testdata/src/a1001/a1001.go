package a1001

import (
	"maps"
	"sync"
	"time"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

type MessageValues struct {
	ID    int64
	Name  string
	When  time.Time
	Where gen.PID
}

type MessageSlice struct {
	ID    int64
	Items []string
}

type MessageMap struct {
	Meta map[string]string
}

type MessageNested struct {
	ID    int64
	Inner Inner
}

type Inner struct {
	Tags []string
}

type Order struct {
	ID int64
}

type MessageOrders struct {
	Orders map[int64]*Order
}

type MessagePointer struct {
	Cache *shared.Unguarded
}

type MessageGuarded struct {
	Cache *shared.Guarded
}

type MessageSyncMap struct {
	Conns *sync.Map
}

type MessageLeaky struct {
	Store *shared.Leaky
}

type MessageBytes struct {
	Payload []byte
}

type MessageChan struct {
	Done chan struct{}
}

type Worker struct {
	act.Actor
	pid    gen.PID
	live   map[string]string
	orders map[int64]*Order
	queued map[int64]*Order
	names  []string
	tags   []string
	cache  *shared.Unguarded
	guard  *shared.Guarded
	conns  *sync.Map
	leaky  *shared.Leaky
	done   chan struct{}
}

// The two fields this actor writes into. What separates a tier 1 finding from a
// tier 2 one is exactly this: whether the memory the receiver holds moves.
func (w *Worker) record(key string, value string) { w.live[key] = value }
func (w *Worker) place(order *Order)              { w.orders[order.ID] = order }
func (w *Worker) renumber(id int64)               { w.orders[id].ID = id }
func (w *Worker) queue(order *Order)              { w.queued[order.ID] = order }
func (w *Worker) tag(name string)                 { w.tags = append(w.tags, name) }

// This one replaces the whole slice, which is not a write into anything the
// receiver of an earlier message is holding.
func (w *Worker) refresh(names []string) { w.names = names }

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	// value only payloads are silent
	w.Send(w.pid, MessageValues{ID: 1, Name: "x"})

	// built here and abandoned: the receiver becomes the owner, which is what
	// sending a reference instead of a copy is for
	w.Send(w.pid, MessageSlice{ID: 1, Items: []string{"a", "b"}})

	// a map this actor keeps writing into, reached through a local
	live := w.live
	w.Send(w.pid, MessageMap{Meta: live}) // want `\[tier1\] \[A1001\] a1001.MessageMap is sent as a message and shares memory.*mutated in place`

	// a field only ever replaced whole: stable today, and nothing in the type says so
	names := w.names
	w.Send(w.pid, MessageSlice{ID: 2, Items: names}) // want `\[tier2\] \[A1001\].*only ever replaced whole`

	// memory that arrived in a message and goes straight back out
	if forwarded, ok := message.(MessageNested); ok {
		w.Send(w.pid, forwarded) // want `\[tier2\] \[A1001\].*arrived in message`
	}

	// a clone copies the map and not what its values point at, and this actor
	// writes through one of those pointers
	w.Send(w.pid, MessageOrders{Orders: maps.Clone(w.orders)}) // want `\[tier1\] \[A1001\] a1001.MessageOrders.*written through`

	// the same clone of a map the actor only ever adds to: what the receiver holds
	// is a map of its own, and nothing reaches the orders behind it
	w.Send(w.pid, MessageOrders{Orders: maps.Clone(w.queued)}) // want `\[tier2\] \[A1001\] a1001.MessageOrders`

	// a clone of a map of values is a real copy, so this one is silent
	w.Send(w.pid, MessageMap{Meta: maps.Clone(w.live)})

	// a pointer to a type with plain maps and no lock
	cache := w.cache
	w.Send(w.pid, MessagePointer{Cache: cache}) // want `A1001.*MessagePointer`

	// a channel can never be shared safely
	done := w.done
	w.Send(w.pid, MessageChan{Done: done}) // want `A1001.*MessageChan`

	// a mutex bearing pointee is deliberately shared, so it stays silent
	guard := w.guard
	w.Send(w.pid, MessageGuarded{Cache: guard})

	// sync.Map is safe by construction
	conns := w.conns
	w.Send(w.pid, MessageSyncMap{Conns: conns})

	// a mutex does not cover an exported reference field reached directly
	leaky := w.leaky
	w.Send(w.pid, MessageLeaky{Store: leaky}) // want `A1001.*MessageLeaky`

	// []byte is allowlisted as a payload by convention
	w.Send(w.pid, MessageBytes{Payload: []byte("x")})

	// a recorded exception is silent
	//argus:allow A1001 the receiver owns the slice from here on
	w.Send(w.pid, MessageSlice{ID: 3, Items: names})

	// a blanket ignore is silent too
	w.Send(w.pid, MessageMap{Meta: live}) //argus:ignore migrating this path
	return nil
}

func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// a send outside the payload position must not be mistaken for one
	w.SendResponse(from, ref, MessageValues{ID: 2})

	// one caller hands over state, the other builds fresh: the verdict at the
	// helper is the worse of the two
	w.publish(w.tags)
	w.publish([]string{"fresh"})
	return MessageValues{ID: 3}, nil
}

// An unexported helper is resolvable: every call to it is in this package, so the
// provenance of its parameter is the worst provenance any caller passes.
func (w *Worker) publish(names []string) {
	w.Send(w.pid, MessageSlice{Items: names}) // want `\[tier1\] \[A1001\].*at its callers.*mutated in place`
}

// An exported one is not: this package cannot see who calls it.
func (w *Worker) Forward(items []string) {
	w.Send(w.pid, MessageSlice{Items: items}) // want `\[tier2\] \[A1001\].*does not resolve here`
}

// gen.Node.SendEvent carries the routing options where a process carries the
// message, so the payload is one argument further along.
func sendViaNode(node gen.Node, name gen.Atom, token gen.Ref, tags []gen.TracingAttribute, w *Worker) {
	node.SendEvent(name, token, gen.MessageOptions{TracingAttributes: tags},
		MessageMap{Meta: w.live}) // want `A1001.*a1001.MessageMap`
}

// On a connection the position a process uses holds a PID, which carries no
// reference at all, so reading it loses the send silently.
func sendViaConnection(conn gen.Connection, from gen.PID, to gen.PID, w *Worker) {
	conn.SendPID(from, to, gen.MessageOptions{},
		MessageMap{Meta: w.live}) // want `A1001.*a1001.MessageMap`
}
