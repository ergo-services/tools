// Package shared exercises the cross package path: these types are declared here
// and sent from another package, so the verdict must travel through export data.
package shared

import "sync"

// Guarded synchronizes its own state, so sharing it is deliberate.
type Guarded struct {
	mu   sync.RWMutex
	data map[string]string
}

func (g *Guarded) Get(k string) string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.data[k]
}

// Unguarded has plain maps and a mutating method with no lock anywhere.
type Unguarded struct {
	Data map[string]string
}

func (u *Unguarded) Put(k, v string) { u.Data[k] = v }

// Leaky exposes an exported reference field despite holding a mutex, so the lock
// does not cover a caller reaching the field directly.
type Leaky struct {
	mu    sync.Mutex
	Items []int
}

// Buffer is the shape that produced the one real defect in a corpus sweep. It is
// properly locked, and it still hands out memory it keeps mutating: callers must
// not retain the result of Peek across Advance.
type Buffer struct {
	mu   sync.Mutex
	data []byte
	pos  int
}

// Peek returns a window into the buffer's own storage, so the alias escapes. The
// escape fact is published here and consumed by the sending package, which is what
// exercises the cross package path end to end.
func (b *Buffer) Peek(n int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data[b.pos : b.pos+n]
}

// Head returns the address of an element, which is the same escape by pointer.
func (b *Buffer) Head() *byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return &b.data[b.pos]
}

// Copy is the honoured form: the caller owns the result outright.
func (b *Buffer) Copy(n int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data[b.pos:b.pos+n]...)
}

// Len returns a value, so nothing escapes.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.data) - b.pos
}

func (b *Buffer) Advance(n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pos += n
}
