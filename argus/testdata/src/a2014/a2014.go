package a2014

import (
	"bufio"
	"bytes"
	"database/sql"
	"net/http"
	"os"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type RequestRecord struct{ Reason string }

type Worker struct {
	act.Actor
	db     *sql.DB
	client *http.Client
	audit  gen.PID
	file   *os.File
	buf    *bufio.Writer
}

// The canonical form: a final "record this as failed" write in Terminate.
func (w *Worker) Terminate(reason error) {
	w.db.Exec("update jobs set state = 'failed'") // want `\[tier2\] \[A2014\] Terminate performs a database round trip;`
}

type Reporter struct {
	act.Actor
	client *http.Client
	audit  gen.PID
}

// A request to another process is a round trip by construction.
func (r *Reporter) Terminate(reason error) {
	r.Call(r.audit, RequestRecord{Reason: reason.Error()}) // want `A2014.*Terminate performs a request awaiting a reply;`
}

// An HTTP call through the shared client.
func (r *Reporter) HandleMessage(from gen.PID, message any) error {
	return nil
}

type Poster struct {
	act.Actor
	client *http.Client
}

func (p *Poster) Terminate(reason error) {
	p.client.Get("http://audit/record") // want `A2014.*an HTTP round trip;`
}

// The verdict reaches a helper in this package, and the helper is named.
type Delegating struct {
	act.Actor
	db *sql.DB
}

func (d *Delegating) Terminate(reason error) {
	d.record(reason) // want `A2014.*a database round trip, through record;`
}

func (d *Delegating) record(reason error) {
	d.db.Exec("insert into audit values ($1)", reason.Error())
}

// A durability barrier and a cleanup have nowhere else to go, and are not on the
// surface. This is the shape a first attempt at the rule reported wrongly.
type Closing struct {
	act.Actor
	file *os.File
}

func (c *Closing) Terminate(reason error) {
	c.file.Sync()
	c.file.Close()
	os.Remove(c.file.Name())
}

// A Flush matches on receiver type alone, and this one writes to memory. Keeping it
// off the surface is the reason the surface is round trips rather than I/O.
type Buffering struct {
	act.Actor
	buf *bufio.Writer
}

func (b *Buffering) Terminate(reason error) {
	b.buf.Flush()
}

func newMemoryWriter() *bufio.Writer {
	return bufio.NewWriter(&bytes.Buffer{})
}

// The same call in a message handler is not this rule's business: there the process is
// alive and the mailbox is the thing at risk, which is A1002's question.
type Live struct {
	act.Actor
	db *sql.DB
}

func (l *Live) HandleMessage(from gen.PID, message any) error {
	l.db.Exec("update jobs set state = 'done'")
	return nil
}

func (l *Live) Terminate(reason error) {}

// A send does not wait for anything, so it is the recommended fix rather than a
// finding.
type Fire struct {
	act.Actor
	audit gen.PID
}

func (f *Fire) Terminate(reason error) {
	f.Send(f.audit, RequestRecord{Reason: reason.Error()})
}

// A recorded exception is silent.
type Recorded struct {
	act.Actor
	db *sql.DB
}

func (r *Recorded) Terminate(reason error) {
	//argus:allow A2014 the node is never stopped without draining this table first
	r.db.Exec("update jobs set state = 'failed'")
}
