package a2011

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"shared"
)

// Client is reached through an interface, so what writes into the object behind it
// is invisible here: an interface field is never exempt.
type Client interface {
	Do(request string) error
}

type MixedArgs struct {
	Name  string
	Index map[string]string
	Store *shared.Unguarded
}

type ClientArgs struct {
	ID      string
	Clients map[string]Client
}

type Head struct {
	act.Actor
	store    *shared.Unguarded
	clients  map[string]Client
	live     map[string]Client
	profiles []Profile
	ids      []int64
}

func (h *Head) Init(args ...any) error {
	// the map is filled once, before any child of this actor is spawned
	h.clients = map[string]Client{}
	h.clients["a"] = nil
	return nil
}

func (h *Head) HandleMessage(from gen.PID, message any) error {
	// the witness names the field that is shared, not the first reference of the type
	h.Spawn(factoryWorker, gen.ProcessOptions{}, MixedArgs{Index: map[string]string{}, Store: h.store}) // want `\[tier2\] \[A2011\] a2011.MixedArgs is passed to Spawn and shares unsynchronized memory with the parent: Store \*shared.Unguarded.*reaches h.store, only ever replaced whole`

	// a shared map of interface values, written only in Init
	h.Spawn(factoryWorker, gen.ProcessOptions{}, ClientArgs{Clients: h.clients}) // want `\[tier2\] \[A2011\] a2011.ClientArgs is passed to Spawn and shares unsynchronized memory with the parent: Clients map\[string\]a2011.Client.*reaches h.clients, only ever replaced whole`

	// the same shape written while children run
	h.Spawn(factoryWorker, gen.ProcessOptions{}, ClientArgs{Clients: h.live}) // want `\[tier1\] \[A2011\] a2011.ClientArgs .*reaches h.live, mutated in place`

	// a fresh slice of copies shares only what the copies reference
	h.Spawn(factoryWorker, gen.ProcessOptions{}, h.profileCopies()) // want `\[tier2\] \[A2011\] \[\]a2011.Profile .*reaches h.profiles\[\], only ever replaced whole`

	// and a fresh slice of plain values shares nothing at all
	h.Spawn(factoryWorker, gen.ProcessOptions{}, h.idCopies())
	return nil
}

func (h *Head) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	h.live["x"] = nil
	return nil, nil
}

func (h *Head) profileCopies() []Profile {
	out := make([]Profile, 0, len(h.profiles))
	for _, p := range h.profiles {
		out = append(out, p)
	}
	return out
}

func (h *Head) idCopies() []int64 {
	out := make([]int64, 0, len(h.ids))
	out = append(out, h.ids...)
	return out
}
