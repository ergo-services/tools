package a1007

import (
	"sync"

	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type serverInfo struct {
	ID         string
	MaxPayload int64
	URLs       []string
}

// conn reads its fields under a lock and returns them. A string or an int64 is a
// copy by the time the caller holds it, whichever field it was read from; a slice
// is the field's own backing array.
type conn struct {
	mu   sync.RWMutex
	info serverInfo
}

func (c *conn) ServerID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info.ID
}

func (c *conn) MaxPayload() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info.MaxPayload
}

func (c *conn) URLs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info.URLs
}

type MessageConnect struct {
	ServerID   string
	MaxPayload int64
}

type MessageURLs struct {
	URLs []string
}

type Connector struct {
	act.Actor
	c   *conn
	pid gen.PID
}

func (a *Connector) HandleMessage(from gen.PID, message any) error {
	a.Send(a.pid, MessageConnect{ServerID: a.c.ServerID(), MaxPayload: a.c.MaxPayload()})
	a.Send(a.pid, MessageURLs{URLs: a.c.URLs()}) // want `\[tier1\] \[A1007\] URLs returns memory derived from its receiver's info field`
	return nil
}
