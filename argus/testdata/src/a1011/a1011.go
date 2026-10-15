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

	// a sleep or a channel wait has no inner budget to set against the init budget:
	// that wait is A1002's, and a request next to it is still this rule's
	s.Spawn(factories.FactorySleepy, gen.ProcessOptions{})
	s.Spawn(factories.FactoryWaiting, gen.ProcessOptions{})
	s.Spawn(factories.FactoryMixed, gen.ProcessOptions{}) // want `A1011.*FactoryMixed waits in Init \(request, via factories.Mixed.Init\) with an inner budget of the default 5s`

	// the timeout of a request in a helper of another package travels with the fact
	s.Spawn(factories.FactoryDelegated, gen.ProcessOptions{InitTimeout: 10}) // want `A1011.*FactoryDelegated waits in Init.*inner budget of 30s against an init budget of 10s`
	s.Spawn(factories.FactoryDelegatedBrief, gen.ProcessOptions{})

	// a request that Init only stores for later is not made in Init
	s.Spawn(factories.FactoryRetrying, gen.ProcessOptions{})

	// the inner budget is the longest one: a short request does not hide a default
	// one, here or through a helper of another package; a timeout of 0 is the default,
	// and one that does not resolve is not a budget
	s.Spawn(factories.FactoryHidden, gen.ProcessOptions{})        // want `A1011.*FactoryHidden waits in Init.*inner budget of the default 5s against an init budget of the default 5s`
	s.Spawn(factories.FactoryDelegatedBoth, gen.ProcessOptions{}) // want `A1011.*FactoryDelegatedBoth waits in Init.*inner budget of the default 5s`
	s.Spawn(factories.FactoryZero, gen.ProcessOptions{})          // want `A1011.*FactoryZero waits in Init.*inner budget of the default 5s`
	s.Spawn(factories.FactoryConfigured, gen.ProcessOptions{})

	// a literal inside a factory returning some other behavior does not hide the one
	// the factory returns
	s.Spawn(factories.FactoryWrapped, gen.ProcessOptions{}) // want `A1011.*FactoryWrapped waits in Init \(request, via factories.Direct.Init\)`

	// the registered form carries the factory one slot further along
	s.SpawnRegister("direct", factories.FactoryDirect, gen.ProcessOptions{}) // want `A1011.*FactoryDirect waits in Init`

	// a recorded exception is silent
	//argus:allow A1011 the seed service is on the same node and answers in microseconds
	s.Spawn(factories.FactoryDirect, gen.ProcessOptions{})

	// options built in a variable resolve too, with the field writes that run on the
	// way to the spawn
	options := gen.ProcessOptions{PreserveMailbox: true}
	options.InitTimeout = 10
	s.Spawn(factories.FactoryDirect, options)

	var assigned gen.ProcessOptions
	assigned = gen.ProcessOptions{InitTimeout: roomyInit}
	s.SpawnRegister("assigned", factories.FactoryDirect, assigned)

	base := gen.ProcessOptions{InitTimeout: 10}
	copied := base
	copied.PreserveMailbox = true
	s.Spawn(factories.FactoryDirect, copied)

	plain := gen.ProcessOptions{PreserveMailbox: true}
	s.Spawn(factories.FactoryDirect, plain) // want `A1011.*FactoryDirect waits in Init.*init budget of the default 5s`

	s.Spawn(factories.FactoryDirect, roomy)
	s.Spawn(factories.FactoryDirect, tight) // want `A1011.*FactoryDirect waits in Init.*init budget of 5s`

	// a write after the spawn does not reach it
	late := gen.ProcessOptions{}
	s.Spawn(factories.FactoryDirect, late) // want `A1011.*FactoryDirect waits in Init`
	late.InitTimeout = 10

	// a budget the source does not decide is unknown, and unknown is not the default
	branchy := gen.ProcessOptions{}
	if len(args) > 0 {
		branchy.InitTimeout = 10
	}
	s.Spawn(factories.FactoryDirect, branchy)
	s.Spawn(factories.FactoryDirect, gen.ProcessOptions{InitTimeout: len(args) * 10})
	s.Spawn(factories.FactoryDirect, s.options())
	s.Spawn(factories.FactoryDirect, Exported)

	looped := gen.ProcessOptions{}
	for i := range args {
		s.Spawn(factories.FactoryDirect, looped)
		looped.InitTimeout = i
	}
	return nil
}

const roomyInit = 10

var roomy = gen.ProcessOptions{InitTimeout: roomyInit}

var tight = gen.ProcessOptions{InitTimeout: 5}

// Exported can be changed from any package, so its value here is unknown.
var Exported = gen.ProcessOptions{}

func (s *Sup) options() gen.ProcessOptions { return roomy }

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
			{
				Name:    "direct-with-room-in-a-variable",
				Factory: factories.FactoryDirect,
				Options: childOptions,
			},
			{ // want `A1011.*FactoryDirect waits in Init`
				Name:    "direct-in-a-variable",
				Factory: factories.FactoryDirect,
				Options: defaults,
			},
			{
				Name:    "direct-with-unknown-room",
				Factory: factories.FactoryDirect,
				Options: c.childOptions(),
			},
		},
	}
	return spec, nil
}

var childOptions = gen.ProcessOptions{InitTimeout: 20}

var defaults = gen.ProcessOptions{}

func (c *ChildSup) childOptions() gen.ProcessOptions { return childOptions }

// An application group member is the capped case: past fifteen seconds the advice is
// to restructure, because raising the budget aborts the whole application start.
type App struct{}

func (a *App) Load(node gen.Node, args ...any) (gen.ApplicationSpec, error) {
	member := gen.ProcessOptions{InitTimeout: 10}
	return gen.ApplicationSpec{
		Name: "worker_app",
		Group: []gen.ApplicationMemberSpec{
			{Name: "quiet", Factory: factories.FactoryQuiet},
			{Name: "patient", Factory: factories.FactoryPatient}, // want `A1011.*FactoryPatient waits in Init.*has to be restructured`
			{Name: "direct", Factory: factories.FactoryDirect, Options: member},
			{Name: "unknown", Factory: factories.FactoryDirect, Options: a.member()},
		},
	}, nil
}

func (a *App) member() gen.ProcessOptions { return gen.ProcessOptions{} }

func (a *App) Start(mode gen.ApplicationMode) {}

func (a *App) Terminate(reason error) {}
