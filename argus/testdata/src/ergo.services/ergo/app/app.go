// Package app is a stand-in for the framework's app package. PreLoad is what
// identifies an application behavior, and the lifecycle callbacks carry the
// signatures the rules read.
package app

import "ergo.services/ergo/gen"

type Application struct {
	gen.Application
}

func (a *Application) PreLoad(app gen.Application, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{}, nil
}

func (a *Application) Load(args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{}, nil
}

func (a *Application) Init(ref gen.Ref, mode gen.ApplicationMode) error { return nil }

func (a *Application) Start(ref gen.Ref, mode gen.ApplicationMode) {}

func (a *Application) Stop(ref gen.Ref, reason error) {}

func (a *Application) Terminate(reason error) {}
