package a1011

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"

	"factories"
)

type Sup struct {
	act.Supervisor
}

// The spawning side. Every verdict here arrived on the factory object from the
// factories package, which is the only way this can work: the operand is a
// gen.ProcessFactory and its static type says nothing about what Init does.
func (s *Sup) Init(args ...any) error {
	// nothing waits in Init
	s.Spawn(factories.FactoryQuiet, gen.ProcessOptions{})

	// a request with the default timeout, inside the default budget: equal, so the
	// spawner gives up exactly when the request would finish
	s.Spawn(factories.FactoryDirect, gen.ProcessOptions{}) // want `\[tier1\] \[A1011\] FactoryDirect waits in Init \(request, via factories.Direct.Init\) with an inner budget of the default 5s against an init budget of the default 5s`

	// the same defect two frames down, which is the shape a syntactic search misses
	s.Spawn(factories.FactoryIndirect, gen.ProcessOptions{}) // want `A1011.*FactoryIndirect waits in Init`

	// raising the budget above the inner wait is the fix
	s.Spawn(factories.FactoryDirect, gen.ProcessOptions{InitTimeout: 10})

	// a long inner timeout is worse, not better
	s.Spawn(factories.FactoryPatient, gen.ProcessOptions{InitTimeout: 10}) // want `A1011.*FactoryPatient waits in Init`

	// a short request inside the default budget is correct
	s.Spawn(factories.FactoryBrief, gen.ProcessOptions{})

	// a sleep is a wait too, and it has no inner timeout of its own
	s.Spawn(factories.FactorySleepy, gen.ProcessOptions{}) // want `A1011.*FactorySleepy waits in Init`

	// the registered form carries the factory one slot further along
	s.SpawnRegister("direct", factories.FactoryDirect, gen.ProcessOptions{}) // want `A1011.*FactoryDirect waits in Init`

	// a recorded exception is silent
	//argus:allow A1011 the seed service is on the same node and answers in microseconds
	s.Spawn(factories.FactoryDirect, gen.ProcessOptions{})
	return nil
}

// A supervisor child spec carries its own options, and the budget is uncapped there.
type ChildSup struct {
	act.Supervisor
}

func (c *ChildSup) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{
				Name:    "quiet",
				Factory: factories.FactoryQuiet,
			},
			{ // want `A1011.*FactoryDirect waits in Init`
				Name:    "direct",
				Factory: factories.FactoryDirect,
			},
			{
				Name:    "direct-with-room",
				Factory: factories.FactoryDirect,
				Options: gen.ProcessOptions{InitTimeout: 20},
			},
		},
	}
	return spec, nil
}

// An application group member is the capped case: past fifteen seconds the advice is
// to restructure, because raising the budget aborts the whole application start.
type App struct{}

func (a *App) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	return gen.ApplicationSpec{
		Name: "worker_app",
		Group: []gen.ApplicationMemberSpec{
			{Name: "quiet", Factory: factories.FactoryQuiet},
			{Name: "patient", Factory: factories.FactoryPatient}, // want `A1011.*FactoryPatient waits in Init.*has to be restructured`
		},
	}, nil
}

func (a *App) Start(mode gen.ApplicationMode) {}

func (a *App) Terminate(reason error) {}
