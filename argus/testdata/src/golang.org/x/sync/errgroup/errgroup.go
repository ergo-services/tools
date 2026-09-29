// Package errgroup is a stand-in for golang.org/x/sync/errgroup. It mirrors the
// declarations the forks surface names: Go and TryGo take the function they run on a
// goroutine of their own as the first argument.
package errgroup

type Group struct{}

func (g *Group) Go(f func() error) {}

func (g *Group) TryGo(f func() error) bool { return true }

func (g *Group) Wait() error { return nil }
