package a1005

import (
	"bufio"
	"net"
	"sync"
	"sync/atomic"

	"ergo.services/ergo/gen"
)

type MessageData struct{ Payload []byte }

// The motivating defect: both goroutines write through one buffered writer, and
// neither side assigns the field, so a model looking for assignments finds nothing.
type Streaming struct {
	gen.MetaProcess
	conn   net.Conn
	writer *bufio.Writer
	sent   uint64
}

func (s *Streaming) Init(process gen.MetaProcess) error {
	s.MetaProcess = process
	s.writer = bufio.NewWriter(s.conn)
	return nil
}

func (s *Streaming) Start() error {
	for {
		s.writer.Write([]byte(": heartbeat\n\n")) // want `\[tier1\] \[A1005\] Start mutates writer, and HandleMessage mutates it too at line \d+`
		s.writer.Flush()
	}
}

func (s *Streaming) HandleMessage(from gen.PID, message any) error {
	if data, ok := message.(MessageData); ok {
		s.writer.Write(data.Payload)
		s.writer.Flush()
	}
	return nil
}

func (s *Streaming) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

// Closing the connection is how the mailbox side unblocks a Start stuck in Read. It is
// what every meta in the framework does, and reporting it would report all of them.
func (s *Streaming) Terminate(reason error) {
	s.conn.Close()
}

// The framework's own shape: the mailbox goroutine is the only writer, and Start hands
// work to it through a channel.
type SingleWriter struct {
	gen.MetaProcess
	conn   net.Conn
	writer *bufio.Writer
	ch     chan []byte
}

func (w *SingleWriter) Init(process gen.MetaProcess) error {
	w.MetaProcess = process
	w.writer = bufio.NewWriter(w.conn)
	w.ch = make(chan []byte, 10)
	return nil
}

func (w *SingleWriter) Start() error {
	for {
		w.ch <- []byte(": heartbeat\n\n")
	}
}

func (w *SingleWriter) HandleMessage(from gen.PID, message any) error {
	select {
	case data := <-w.ch:
		w.writer.Write(data)
	default:
	}
	return nil
}

func (w *SingleWriter) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (w *SingleWriter) Terminate(reason error) {
	w.conn.Close()
}

// An assignment on each side is the plain form.
type Reassigning struct {
	gen.MetaProcess
	state string
	count int
}

func (r *Reassigning) Init(process gen.MetaProcess) error {
	r.MetaProcess = process
	r.state = "init"
	return nil
}

func (r *Reassigning) Start() error {
	r.state = "running" // want `A1005.*Start mutates state`
	r.count++           // want `A1005.*Start mutates count`
	return nil
}

func (r *Reassigning) HandleMessage(from gen.PID, message any) error {
	r.state = "handling"
	r.count++
	return nil
}

func (r *Reassigning) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (r *Reassigning) Terminate(reason error) {}

// A counter written with atomic operations on its address is how the framework does
// this, and it is not a race.
type Counting struct {
	gen.MetaProcess
	bytesIn uint64
}

func (c *Counting) Init(process gen.MetaProcess) error {
	c.MetaProcess = process
	return nil
}

func (c *Counting) Start() error {
	atomic.AddUint64(&c.bytesIn, 1)
	return nil
}

func (c *Counting) HandleMessage(from gen.PID, message any) error {
	atomic.AddUint64(&c.bytesIn, 2)
	return nil
}

func (c *Counting) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	_ = atomic.LoadUint64(&c.bytesIn)
	return nil, nil
}

func (c *Counting) Terminate(reason error) {}

// Every access on both sides taken under a mutex of the same receiver.
type Locked struct {
	gen.MetaProcess
	mu    sync.Mutex
	items map[string]int
}

func (l *Locked) Init(process gen.MetaProcess) error {
	l.MetaProcess = process
	l.items = map[string]int{}
	return nil
}

func (l *Locked) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items["start"] = 1
	return nil
}

func (l *Locked) HandleMessage(from gen.PID, message any) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items["handle"] = 2
	return nil
}

func (l *Locked) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (l *Locked) Terminate(reason error) {}

// A field written in Init and only read afterwards is ordered: Init runs on the
// spawning actor's goroutine, before the start goroutine exists.
type Configured struct {
	gen.MetaProcess
	target gen.Atom
	buf    []byte
}

func (c *Configured) Init(process gen.MetaProcess) error {
	c.MetaProcess = process
	c.target = "worker"
	c.buf = make([]byte, 1024)
	return nil
}

func (c *Configured) Start() error {
	return c.Send(c.target, MessageData{})
}

func (c *Configured) HandleMessage(from gen.PID, message any) error {
	return c.Send(c.target, message)
}

func (c *Configured) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *Configured) Terminate(reason error) {}

// A field touched only in Terminate is on one side, so it is never reported against
// itself even though the runtime may reach Terminate from either goroutine.
type ClosingOnly struct {
	gen.MetaProcess
	scratch map[string]int
}

func (c *ClosingOnly) Init(process gen.MetaProcess) error {
	c.MetaProcess = process
	c.scratch = map[string]int{}
	return nil
}

func (c *ClosingOnly) Start() error { return nil }

func (c *ClosingOnly) HandleMessage(from gen.PID, message any) error { return nil }

func (c *ClosingOnly) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (c *ClosingOnly) Terminate(reason error) {
	c.scratch["terminated"] = 1
}

// A plain actor has one goroutine, so none of this applies to it.
type NotAMeta struct {
	items map[string]int
}

func (n *NotAMeta) Start() error {
	n.items["start"] = 1
	return nil
}

func (n *NotAMeta) HandleMessage(from gen.PID, message any) error {
	n.items["handle"] = 2
	return nil
}

// A recorded exception is silent.
type Recorded struct {
	gen.MetaProcess
	writer *bufio.Writer
}

func (r *Recorded) Init(process gen.MetaProcess) error {
	r.MetaProcess = process
	return nil
}

func (r *Recorded) Start() error {
	//argus:allow A1005 Start returns before the mailbox goroutine is given any work
	r.writer.Write([]byte("hello"))
	return nil
}

func (r *Recorded) HandleMessage(from gen.PID, message any) error {
	r.writer.Write([]byte("world"))
	return nil
}

func (r *Recorded) HandleCall(from gen.PID, ref gen.Ref, request any) (any, error) {
	return nil, nil
}

func (r *Recorded) Terminate(reason error) {}
