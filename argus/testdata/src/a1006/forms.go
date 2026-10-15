package a1006

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Ann struct {
	Version string
	Attrs   map[string]any
}

type Req Ann

type Runes []rune

type Mixed struct {
	Raw   []byte
	Items []int
}

// Holder sends its own fields in the forms this rule did not see before, and to
// the one receiver that never races with it.
type Holder struct {
	act.Actor
	ann      Ann
	tagged   Ann
	booted   map[string]int
	mixed    Mixed
	title    string
	reporter gen.PID
}

func (h *Holder) Init(args ...any) error {
	h.booted = map[string]int{}
	h.booted["a"] = 1
	return nil
}

// Only a value sub-field: a copy of ann never sees it.
func (h *Holder) bump() { h.ann.Version = "v2" }

// The map behind tagged, reached through two selectors.
func (h *Holder) tag() { h.tagged.Attrs["seen"] = true }

// A replacement is invisible to a copy and visible through the field's address.
func (h *Holder) reset() { h.mixed = Mixed{} }

func (h *Holder) HandleMessage(from gen.PID, message any) error {
	// a conversion hands over the same memory as the field itself
	h.Send(h.reporter, Req(h.ann))    // want `\[tier2\] \[A1006\] HandleMessage puts h.ann into the payload.*only ever replaces h.ann whole`
	h.Send(h.reporter, Req(h.tagged)) // want `\[tier1\] \[A1006\] HandleMessage puts h.tagged into the payload.*writes into h.tagged`

	// written only in Init, which is over before this callback can run
	h.Send(h.reporter, h.booted) // want `\[tier2\] \[A1006\] HandleMessage puts h.booted into the payload`

	// the allowlisted first reference does not hide the slice after it
	h.Send(h.reporter, h.mixed) // want `\[tier2\] \[A1006\] HandleMessage puts h.mixed into the payload.*reads it through Items \[\]int`

	// a conversion out of a string copies, so nothing of the field is handed over
	h.Send(h.reporter, Runes(h.title))

	// the address of a field sees its replacement as well
	h.Send(h.reporter, &h.mixed) // want `\[tier1\] \[A1006\] HandleMessage puts h.mixed into the payload.*writes into h.mixed`

	// its own mailbox is handled by this same goroutine
	h.Send(h.PID(), h.tagged)
	return nil
}

type base struct {
	cache map[string]int
}

// A promoted field is the same memory whichever way it is spelled.
type Embedder struct {
	act.Actor
	base
	reporter gen.PID
}

func (e *Embedder) put() { e.base.cache["k"] = 1 }

func (e *Embedder) HandleMessage(from gen.PID, message any) error {
	e.Send(e.reporter, e.cache) // want `\[tier1\] \[A1006\] HandleMessage puts e.cache into the payload.*writes into e.cache`
	return nil
}
