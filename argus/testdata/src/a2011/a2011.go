package a2011

import (
	"sync"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

// value only, which is what a spawn argument should be
type ValueArgs struct {
	ID   int64
	Name string
}

// a pointee with plain maps and no lock anywhere
type SharedArgs struct {
	Store *shared.Unguarded
}

// a pointee that synchronizes its own state, so sharing it is deliberate
type GuardedArgs struct {
	Store *shared.Guarded
}

// a sync.Map is safe by construction
type ConnArgs struct {
	Conns *sync.Map
}

// a func cannot be serialized, so it also pins the child to this node
type FuncArgs struct {
	Build func() int
}

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

type Sup struct {
	act.Actor
}

func (s *Sup) Init(args ...any) error {
	// a value argument is silent
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ValueArgs{ID: 1})

	// an unsynchronized pointee is handed to a second goroutine
	s.Spawn(factoryWorker, gen.ProcessOptions{}, SharedArgs{}) // want `\[tier2\] \[A2011\] a2011.SharedArgs is passed to Spawn and shares unsynchronized memory`

	// guarded and sync.Map are deliberate sharing
	s.Spawn(factoryWorker, gen.ProcessOptions{}, GuardedArgs{})
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ConnArgs{})

	// a func argument gets its own wording
	s.Spawn(factoryWorker, gen.ProcessOptions{}, FuncArgs{}) // want `A2011.*a2011.FuncArgs is passed to Spawn and carries a func`

	// the registered form carries the arguments one slot further along
	s.SpawnRegister("worker", factoryWorker, gen.ProcessOptions{}, SharedArgs{}) // want `A2011.*a2011.SharedArgs is passed to SpawnRegister`

	// a bare map is the same defect without a wrapper type
	s.Spawn(factoryWorker, gen.ProcessOptions{}, map[string]string{}) // want `A2011.*map\[string\]string is passed to Spawn`

	// a recorded exception is silent
	//argus:allow A2011 the store is read only after start
	s.Spawn(factoryWorker, gen.ProcessOptions{}, SharedArgs{})

	// a spawn with no user arguments has nothing to check
	s.Spawn(factoryWorker, gen.ProcessOptions{})
	return nil
}
