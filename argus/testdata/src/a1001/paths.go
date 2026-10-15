package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Profile struct {
	Version string
	Attrs   map[string]any
}

type MessageProfile Profile

type MessageSources struct {
	Sources []string
}

type feed struct {
	sources []string
	revived bool
}

// Paths writes into its state at different depths. A write is a race with a
// receiver only when it lands in memory the payload references: under the part the
// payload holds, and through a pointer, a map or a slice.
type Paths struct {
	act.Actor
	value   Profile
	written Profile
	streams map[string]*feed
	grown   map[string]*feed
	peer    gen.PID
}

// A value field of a value struct: the copy a receiver holds never sees it.
func (p *Paths) bump() { p.value.Version = "v2" }

// The map behind the field, reached through two selectors: a real write into memory
// every copy of written shares.
func (p *Paths) tag() { p.written.Attrs["seen"] = true }

// A flag in the element a payload never holds.
func (p *Paths) revive(key string) { p.streams[key].revived = true }

// The slice a payload does hold.
func (p *Paths) grow(key string) { p.grown[key].sources[0] = "x" }

func (p *Paths) HandleMessage(from gen.PID, message any) error {
	value := p.value
	p.Send(p.peer, MessageProfile(value)) // want `\[tier2\] \[A1001\] a1001.MessageProfile .*p.value, only ever replaced whole`

	written := p.written
	p.Send(p.peer, MessageProfile(written)) // want `\[tier1\] \[A1001\] a1001.MessageProfile .*p.written, mutated in place`

	stream := p.streams["k"]
	p.Send(p.peer, MessageSources{Sources: stream.sources}) // want `\[tier2\] \[A1001\] a1001.MessageSources .*p.streams\[\].sources, only ever replaced whole`

	grown := p.grown["k"]
	p.Send(p.peer, MessageSources{Sources: grown.sources}) // want `\[tier1\] \[A1001\] a1001.MessageSources .*p.grown\[\].sources, written through`
	return nil
}
