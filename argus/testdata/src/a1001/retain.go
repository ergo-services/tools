package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Announce struct {
	Name  string
	Attrs map[string]any
}

type MessageSpawn Announce

type Entry struct {
	Attrs map[string]any
}

type MessageEntry struct {
	E *Entry
}

// Keeper stores what it sends. Inserting into a map and deleting from it moves the
// map, not the values already in it, so a receiver of a stored value races with this
// actor only where the actor writes through the stored elements.
type Keeper struct {
	act.Actor
	running map[string]Announce
	entries map[string]*Entry
	peer    gen.PID
}

func (k *Keeper) drop(name string) { delete(k.running, name) }

func (k *Keeper) touch(name string) { k.entries[name].Attrs["seen"] = true }

func (k *Keeper) HandleMessage(from gen.PID, message any) error {
	a := Announce{Name: "x", Attrs: map[string]any{}}
	k.running[a.Name] = a
	k.Send(k.peer, MessageSpawn(a)) // want `\[tier2\] \[A1001\] a1001.MessageSpawn .*k.running\[\] = a, only ever replaced whole`

	e := &Entry{Attrs: map[string]any{}}
	k.entries["x"] = e
	k.Send(k.peer, MessageEntry{E: e}) // want `\[tier1\] \[A1001\] a1001.MessageEntry .*k.entries\[\] = e, mutated in place`
	return nil
}

// A store on a path the send never takes does not make the send share anything.
func (k *Keeper) spawnOrStore(a Announce, full bool) {
	if full {
		k.Send(k.peer, MessageSpawn(a)) // want `\[tier2\] \[A1001\] a1001.MessageSpawn .*does not resolve here \(parameter a\)`
		return
	}
	k.running[a.Name] = a
}

func (k *Keeper) storeOrSpawn(full bool) {
	a := Announce{Name: "y", Attrs: map[string]any{}}
	if full {
		k.Send(k.peer, MessageSpawn(a))
	} else {
		k.running[a.Name] = a
	}
}

func (k *Keeper) earlyReturn(full bool) {
	a := Announce{Name: "z", Attrs: map[string]any{}}
	if full {
		k.Send(k.peer, MessageSpawn(a))
		return
	}
	k.running[a.Name] = a
}

// A loop carries the store of one iteration around to the send of the next.
func (k *Keeper) looped(names []string) {
	e := &Entry{Attrs: map[string]any{}}
	for i, name := range names {
		if i > 0 {
			k.Send(k.peer, MessageEntry{E: e}) // want `\[tier1\] \[A1001\] a1001.MessageEntry .*k.entries\[\] = e, mutated in place`
			return
		}
		k.entries[name] = e
	}
}

// A store after the send on the same path still leaves two holders.
func (k *Keeper) sendThenStore() {
	e := &Entry{Attrs: map[string]any{}}
	k.Send(k.peer, MessageEntry{E: e}) // want `\[tier1\] \[A1001\] a1001.MessageEntry .*k.entries\[\] = e, mutated in place`
	k.entries["late"] = e
}
