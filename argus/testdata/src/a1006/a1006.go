package a1006

import (
	"sync"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Order struct {
	ID    int64
	Price string
}

type MessageSnapshot struct {
	Orders map[int64]*Order
	Taken  int64
}

type MessageCount struct {
	Total int
}

type Guarded struct {
	mu    sync.Mutex
	items map[string]int
}

type Manager struct {
	act.Actor
	orders   map[int64]*Order
	pending  []int64
	guarded  *Guarded
	total    int
	name     string
	reporter gen.PID
}

// The direct form: the field itself is the payload.
func (m *Manager) HandleMessage(from gen.PID, message any) error {
	m.Send(m.reporter, m.orders) // want `\[tier1\] \[A1006\] HandleMessage puts m.orders into the payload, and m.orders is this actor's own state`

	// A field of a composite literal payload.
	m.Send(m.reporter, MessageSnapshot{Orders: m.orders, Taken: 1}) // want `A1006.*puts m.orders into this message`

	// A slice field is the same hazard.
	m.Send(m.reporter, m.pending) // want `A1006.*puts m.pending into the payload`

	// A value field carries no reference, so there is nothing to share.
	m.Send(m.reporter, m.total)
	m.Send(m.reporter, MessageCount{Total: m.total})

	// A pointee that synchronizes its own state was shared on purpose, which is the
	// same gate A1001 applies.
	m.Send(m.reporter, m.guarded)

	// A string is immutable.
	m.Send(m.reporter, m.name)

	// A freshly built map is not the actor's state: nothing else holds it.
	snapshot := map[int64]*Order{}
	for id, order := range m.orders {
		snapshot[id] = order
	}
	m.Send(m.reporter, MessageSnapshot{Orders: snapshot})
	return nil
}

// A response is a send too.
func (m *Manager) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	m.SendResponse(from, ref, m.orders) // want `A1006.*HandleCall puts m.orders into the payload`
	return nil, nil
}

// A slice expression on a receiver field is memory escaping the owner, which is
// A1007's surface, so this rule does not also claim it.
func (m *Manager) HandleEvent(event gen.MessageEvent) error {
	m.Send(m.reporter, m.pending[:1])
	return nil
}

// A spawn argument is A2011's surface exclusively.
func (m *Manager) Init(args ...any) error {
	m.orders = map[int64]*Order{}
	m.Spawn(factoryWorker, gen.ProcessOptions{}, m.orders)
	return nil
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

type Worker struct {
	act.Actor
	shared map[int64]*Order
	peer   gen.PID
}

// A recorded exception is silent.
func (w *Worker) HandleMessage(from gen.PID, message any) error {
	//argus:allow A1006 the receiver only reads this map and the writer is stopped by then
	w.Send(w.peer, w.shared)
	return nil
}

// Outside a callback there is no actor whose state this is, and no receiver identifier
// to compare against either.
func report(peer gen.PID, orders map[int64]*Order) {}
