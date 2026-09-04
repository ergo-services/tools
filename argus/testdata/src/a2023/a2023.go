package a2023

import (
	"errors"

	"ergo.services/ergo/app"
	"ergo.services/ergo/gen"
)

type pool struct{}

func (p *pool) Close() {}

func openPool() (*pool, error) { return &pool{}, nil }

// The shape the rule exists for: Init opens a pool, a later step fails, and the
// callback that would have closed it is never called.
type LeakApp struct {
	app.Application
	pgPool *pool
	cache  *pool
}

func (a *LeakApp) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	p, err := openPool()
	if err != nil {
		// Nothing has been acquired yet, so there is nothing to release.
		return err
	}
	a.pgPool = p

	if _, err := openPool(); err != nil {
		return err // want `\[tier2\] \[A2023\] this returns an error after Init acquired pgPool`
	}
	return nil
}

func (a *LeakApp) Terminate(reason error) {
	if a.pgPool != nil {
		a.pgPool.Close()
	}
}

// The same shape with the release on the failure path.
type CleanApp struct {
	app.Application
	pgPool *pool
}

func (a *CleanApp) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	p, err := openPool()
	if err != nil {
		return err
	}
	a.pgPool = p

	if _, err := openPool(); err != nil {
		a.release()
		return err
	}
	if _, err := openPool(); err != nil {
		a.pgPool.Close()
		return errors.New("second step")
	}
	return nil
}

func (a *CleanApp) release() {
	if a.pgPool != nil {
		a.pgPool.Close()
	}
}

func (a *CleanApp) Terminate(reason error) {
	a.release()
}

// An application with nothing to release is not this rule's business.
type PlainApp struct {
	app.Application
	name string
}

func (a *PlainApp) Init(ref gen.Ref, mode gen.ApplicationMode) error {
	a.name = "plain"
	if a.name == "" {
		return errors.New("no name")
	}
	return nil
}

func (a *PlainApp) Terminate(reason error) {}
