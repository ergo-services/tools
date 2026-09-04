package a1008

import (
	"fmt"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Router struct {
	act.Actor
}

type StatusRequest struct{}
type StatusResponse struct{ Open bool }

// The correct forms, all silent.
func (r *Router) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case StatusRequest:
		return StatusResponse{Open: true}, nil
	}
	// the error travels as the reply
	return gen.ErrUnsupported, nil
}

type Manager struct {
	act.Actor
}

// The defect: the error lands in the termination reason slot.
func (m *Manager) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	switch request.(type) {
	case StatusRequest:
		return StatusResponse{}, nil
	}
	return nil, gen.ErrUnsupported // want `\[tier1\] \[A1008\] HandleCall returns an error in the termination reason slot`
}

type Builder struct {
	act.Actor
}

func (b *Builder) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if request == nil {
		return nil, fmt.Errorf("nil request") // want `A1008.*termination reason slot`
	}
	// reply-then-stop is the idiomatic use of the reason slot
	if request == "stop" {
		return StatusResponse{}, gen.TerminateReasonNormal
	}
	// a deferred reply is legitimate: A2002 owns the never-replies case
	return nil, nil
}

type Deep struct {
	act.Actor
}

func (d *Deep) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	// a nested literal has its own results and must not be matched
	f := func() (any, error) { return nil, fmt.Errorf("inner") }
	v, err := f()
	if err != nil {
		return v, nil
	}
	return nil, nil
}

// Not a callback: an ordinary helper with the same shape stays silent.
func helper() (any, error) {
	return nil, fmt.Errorf("plain error return")
}
