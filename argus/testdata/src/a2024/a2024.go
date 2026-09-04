package a2024

import (
	"bytes"
	"database/sql"
	"net/http"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
	db     *sql.DB
	client *http.Client
	buf    *bytes.Buffer
}

func (w *Worker) Init(args ...any) error {
	w.db.Ping() // want `\[tier2\] \[A2024\] Init performs a database round trip`
	return nil
}

func (w *Worker) HandleMessage(from gen.PID, message any) error {
	w.client.Get("http://peer/health") // want `A2024.*HandleMessage performs an HTTP round trip`
	return nil
}

// The verdict travels through the project's own helpers.
func (w *Worker) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return w.load(), nil // want `A2024.*HandleCall performs a database round trip, through load`
}

func (w *Worker) load() int64 {
	w.db.QueryRow("select 1")
	return 0
}

// An in-memory write is not a round trip, which is the whole reason the surface is
// a list rather than a receiver-type match.
func (w *Worker) HandleEvent(message gen.MessageEvent) error {
	w.buf.WriteString("still here")
	return nil
}

// Terminate belongs to A2014, which has its own argument about it.
func (w *Worker) Terminate(reason error) {
	w.db.Exec("update jobs set state = 'failed'")
}

// A meta run loop is where a blocking client belongs.
type Source struct {
	gen.MetaProcess
	client *http.Client
}

func (s *Source) Init(process gen.MetaProcess) error { return nil }

func (s *Source) Start() error {
	for {
		s.client.Get("http://peer/stream")
	}
}

func (s *Source) HandleMessage(from gen.PID, message any) error { return nil }

func (s *Source) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (s *Source) Terminate(reason error) {}
