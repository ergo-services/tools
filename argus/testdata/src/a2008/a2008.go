package a2008

import (
	"net/http"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
	"ergo.services/ergo/meta"
)

type MessageIdle struct{}

// The canonical defect: the request is answered and never completed, so ServeHTTP
// waits out RequestTimeout and the client gets a 504 instead of this response.
type Handler struct {
	act.Actor
}

func (h *Handler) HandleMessage(from gen.PID, message any) error {
	switch r := message.(type) {
	case meta.MessageWebRequest: // want `\[tier2\] \[A2008\] HandleMessage handles meta.MessageWebRequest and never calls r.Done\(\)`
		r.Response.Write([]byte("ok"))
	case MessageIdle:
	}
	return nil
}

// The correct form.
type Correct struct {
	act.Actor
}

func (c *Correct) HandleMessage(from gen.PID, message any) error {
	switch r := message.(type) {
	case meta.MessageWebRequest:
		defer r.Done()
		r.Response.Write([]byte("ok"))
	}
	return nil
}

// Calling Done directly, not deferred, still completes the request.
type Direct struct {
	act.Actor
}

func (d *Direct) HandleMessage(from gen.PID, message any) error {
	if r, ok := message.(meta.MessageWebRequest); ok {
		r.Response.Write([]byte("ok"))
		r.Done()
	}
	return nil
}

// Handing the whole request to a helper moves the obligation with it.
type Delegating struct {
	act.Actor
}

func (d *Delegating) HandleMessage(from gen.PID, message any) error {
	if r, ok := message.(meta.MessageWebRequest); ok {
		d.serve(r)
	}
	return nil
}

func (d *Delegating) serve(r meta.MessageWebRequest) {
	defer r.Done()
	r.Response.Write([]byte("ok"))
}

// The assertion form with nothing bound: Done cannot be called even in principle.
type Discarding struct {
	act.Actor
}

func (d *Discarding) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case meta.MessageWebRequest: // want `A2008.*matches meta.MessageWebRequest without binding it`
		d.Log().Warning("web request arrived")
	}
	return nil
}

// A case listing several types binds the interface rather than the request.
type Multi struct {
	act.Actor
}

func (m *Multi) HandleMessage(from gen.PID, message any) error {
	switch message.(type) {
	case MessageIdle, meta.MessageWebRequest: // want `A2008.*without binding it`
		m.Log().Warning("something arrived")
	}
	return nil
}

// A WebWorker's run loop intercepts the request before HandleMessage, so this case is
// dead and the verb methods answer every real request.
type Worker struct {
	act.WebWorker
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	switch r := message.(type) {
	case meta.MessageWebRequest: // want `A2008.*Worker embeds act.WebWorker, so this meta.MessageWebRequest case is unreachable: the run loop matches the request before HandleMessage is called`
		defer r.Done()
		r.Response.Write([]byte("never runs"))
	}
	return nil
}

func (w *Worker) HandleGet(from gen.PID, writer http.ResponseWriter, request *http.Request) error {
	writer.Write([]byte("ok"))
	return nil
}

// The event branch delivers gen.MessageEvent, so a raw request never arrives there.
type EventWorker struct {
	act.WebWorker
}

func (e *EventWorker) HandleEvent(event gen.MessageEvent) error {
	if r, ok := event.Message.(meta.MessageWebRequest); ok { // want `A2008.*the event branch delivers gen.MessageEvent`
		defer r.Done()
	}
	return nil
}

// HandleCall is not in either population: a request can reach it only because user
// code forwarded it with Call, and then it genuinely holds it.
type Forwarded struct {
	act.WebWorker
}

func (f *Forwarded) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if r, ok := request.(meta.MessageWebRequest); ok {
		r.Response.Write([]byte("ok"))
	}
	return nil, nil
}

// A plain actor's HandleCall is out for the same reason.
type PlainCall struct {
	act.Actor
}

func (p *PlainCall) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	if r, ok := request.(meta.MessageWebRequest); ok {
		r.Response.Write([]byte("ok"))
	}
	return nil, nil
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	switch req := message.(type) {
	//argus:allow A2008 a sibling actor holds the same request and completes it
	case meta.MessageWebRequest:
		req.Response.Write([]byte("ok"))
	}
	return nil
}
