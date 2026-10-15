package a1001

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Spec struct {
	Symbol string
	Tags   []string
}

type MessageSpecs struct {
	Specs []Spec
}

type MessageList struct {
	V []string
}

type MessageGaps struct {
	Gaps   []int64
	Ranges []Spec
}

type ListRequest struct{}

type ListResponse struct {
	V []string
}

func buildSpecs(n int) ([]Spec, error) {
	return make([]Spec, 0, n), nil
}

// One call forwarding all of its results: each result is the callee's result in
// the same position.
func lookupSpecs(n int) ([]Spec, error) {
	return buildSpecs(n)
}

// A closed interface: its unexported method can only be implemented in this
// package, so the implementers here are all there are.
type symbolSource interface {
	symbols() []string
}

type freshSource struct{}

func (freshSource) symbols() []string { return []string{"a"} }

type keptSource struct{ list []string }

func (k *keptSource) symbols() []string { return k.list }

type freshOnly interface {
	next() []string
}

type generator struct{}

func (generator) next() []string { return []string{"b"} }

// An interface with exported methods only can be implemented anywhere, by a type
// this package never sees.
type Fetcher interface {
	Fetch() []string
}

type Results struct {
	act.Actor
	specs  []Spec
	source symbolSource
	maker  freshOnly
	fetch  Fetcher
	ranges func() []Spec
	peer   gen.PID
}

func (r *Results) cachedSpecs() ([]Spec, error) { return r.specs, nil }

func (r *Results) forwardSpecs() ([]Spec, error) { return r.cachedSpecs() }

func (r *Results) HandleMessage(from gen.PID, message any) error {
	specs, _ := lookupSpecs(3)
	r.Send(r.peer, MessageSpecs{Specs: specs})

	kept, _ := r.forwardSpecs()
	r.Send(r.peer, MessageSpecs{Specs: kept}) // want `\[tier2\] \[A1001\] a1001.MessageSpecs .*r.specs, only ever replaced whole`

	fresh := r.maker.next()
	r.Send(r.peer, MessageList{V: fresh})

	symbols := r.source.symbols()
	r.Send(r.peer, MessageList{V: symbols}) // want `\[tier2\] \[A1001\] a1001.MessageList .*this payload reaches r.source`

	fetched := r.fetch.Fetch()
	r.Send(r.peer, MessageList{V: fetched}) // want `\[tier2\] \[A1001\] a1001.MessageList .*does not resolve here \(Fetch\(\)\)`

	// the response to a request is memory the responder handed over
	response, err := r.Call(r.peer, ListRequest{})
	if err != nil {
		return nil
	}
	list := response.(ListResponse)
	r.Send(r.peer, MessageList{V: list.V}) // want `\[tier2\] \[A1001\] a1001.MessageList .*came back from Call\(\) and is forwarded on`

	// the witness names the field the payload sets, not the first reference the type has
	r.Send(r.peer, MessageGaps{Ranges: r.ranges()}) // want `MessageGaps -> Ranges \[\]a1001.Spec .*does not resolve here \(ranges\(\)\)`
	return nil
}
