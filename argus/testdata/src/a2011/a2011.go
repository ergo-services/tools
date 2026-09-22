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

type Profile struct {
	Name  string
	Tools []string
}

type ProfileArgs struct {
	Profile Profile
}

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

type Sup struct {
	act.Actor
	store   *shared.Unguarded
	guarded *shared.Guarded
	conns   *sync.Map
	index   map[string]string
	profile Profile
}

func (s *Sup) record(key string, value string) { s.index[key] = value }

func (s *Sup) Init(args ...any) error {
	// a value argument is silent
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ValueArgs{ID: 1})

	// built here and handed over: the child becomes the owner
	s.Spawn(factoryWorker, gen.ProcessOptions{}, SharedArgs{Store: &shared.Unguarded{}})

	// an unsynchronized pointee this actor keeps
	store := s.store
	s.Spawn(factoryWorker, gen.ProcessOptions{}, SharedArgs{Store: store}) // want `\[tier2\] \[A2011\] a2011.SharedArgs is passed to Spawn and shares unsynchronized memory`

	// guarded and sync.Map are deliberate sharing
	guarded := s.guarded
	s.Spawn(factoryWorker, gen.ProcessOptions{}, GuardedArgs{Store: guarded})
	conns := s.conns
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ConnArgs{Conns: conns})

	// a func argument gets its own wording, and it is not a question of ownership
	s.Spawn(factoryWorker, gen.ProcessOptions{}, FuncArgs{}) // want `A2011.*a2011.FuncArgs is passed to Spawn and carries a func`

	// the registered form carries the arguments one slot further along
	s.SpawnRegister("worker", factoryWorker, gen.ProcessOptions{}, SharedArgs{Store: store}) // want `A2011.*a2011.SharedArgs is passed to SpawnRegister`

	// a map this actor writes into is a live race with the child
	index := s.index
	s.Spawn(factoryWorker, gen.ProcessOptions{}, index) // want `\[tier1\] \[A2011\].*mutated in place`

	// the copy the parent is asked for: every reference field of the struct is
	// rebuilt before the spawn, so nothing of the parent's is handed over
	profile := s.profile
	profile.Tools = append([]string(nil), s.profile.Tools...)
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ProfileArgs{Profile: profile})

	// the same struct without the copy
	s.Spawn(factoryWorker, gen.ProcessOptions{}, ProfileArgs{Profile: s.profile}) // want `\[tier2\] \[A2011\] a2011.ProfileArgs`

	// a recorded exception is silent
	//argus:allow A2011 the store is read only after start
	s.Spawn(factoryWorker, gen.ProcessOptions{}, SharedArgs{Store: store})

	// a spawn with no user arguments has nothing to check
	s.Spawn(factoryWorker, gen.ProcessOptions{})
	return nil
}
