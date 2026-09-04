package a2025

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

// The router rejections, each one an error out of ProcessInit.
type BadRouter struct {
	act.Router
}

func (r *BadRouter) Init(args ...any) (act.RouterOptions, error) {
	return act.RouterOptions{
		Routes: []act.Route{
			{Name: "ok", Factory: factoryWorker},
			{Name: "", Factory: factoryWorker},   // want `\[tier2\] \[A2025\] route 1 has an empty Name`
			{Name: "nofactory"},                  // want `A2025.*route 2 has no Factory`
			{Name: "nilfactory", Factory: nil},   // want `A2025.*route 3 has a nil Factory`
			{Name: "ok", Factory: factoryWorker}, // want `A2025.*route name "ok" is already used`
		},
	}, nil
}

// A router with no routes at all is legitimate: routes can be added later.
type EmptyRouter struct {
	act.Router
}

func (r *EmptyRouter) Init(args ...any) (act.RouterOptions, error) {
	return act.RouterOptions{}, nil
}

// A pool spawns its workers from the factory before Init returns.
type BadPool struct {
	act.Pool
}

func (p *BadPool) Init(args ...any) (act.PoolOptions, error) {
	return act.PoolOptions{ // want `A2025.*PoolOptions has no WorkerFactory`
		PoolSize: 4,
	}, nil
}

type NilPool struct {
	act.Pool
}

func (p *NilPool) Init(args ...any) (act.PoolOptions, error) {
	opts := act.PoolOptions{PoolSize: 4}
	opts.WorkerFactory = nil // want `A2025.*PoolOptions has no WorkerFactory`
	return opts, nil
}

// A zero PoolSize normalizes to three, so only the factory matters here.
type GoodPool struct {
	act.Pool
}

func (p *GoodPool) Init(args ...any) (act.PoolOptions, error) {
	return act.PoolOptions{WorkerFactory: factoryWorker}, nil
}

// A literal returned on the failure path carries no options.
type FailingPool struct {
	act.Pool
}

func (p *FailingPool) Init(args ...any) (act.PoolOptions, error) {
	if len(args) == 0 {
		return act.PoolOptions{}, gen.ErrIncorrect
	}
	return act.PoolOptions{WorkerFactory: factoryWorker}, nil
}
