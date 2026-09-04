package a2005

import (
	"ergo.services/ergo/act"
	"ergo.services/ergo/gen"
)

type Worker struct {
	act.Actor
}

func factoryWorker() gen.ProcessBehavior { return &Worker{} }

// A valid literal spec is silent.
type SupOK struct {
	act.Supervisor
}

func (s *SupOK) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Name: "w1", Factory: factoryWorker},
			{Name: "w2", Factory: factoryWorker},
		},
	}
	return spec, nil
}

// A spec that never mentions Children has an empty list, which is a resolved state
// and the likeliest real instance of the defect.
type SupNoChildren struct {
	act.Supervisor
}

func (s *SupNoChildren) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec // want `\[tier2\] \[A2005\] the Children list is empty`
	spec.Type = act.SupervisorTypeSimpleOneForOne
	return spec, nil
}

// An explicitly empty list is the same defect.
type SupEmptyChildren struct {
	act.Supervisor
}

func (s *SupEmptyChildren) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{ // want `A2005.*the Children list is empty`
		Type:     act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{},
	}
	return spec, nil
}

// Child level rejections.
type SupBadChildren struct {
	act.Supervisor
}

func (s *SupBadChildren) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{Factory: factoryWorker},             // want `A2005.*child spec 0 has no Name`
			{Name: "", Factory: factoryWorker},   // want `A2005.*child spec 1 has an empty Name`
			{Name: "w3"},                         // want `A2005.*child spec 2 has no Factory`
			{Name: "w4", Factory: nil},           // want `A2005.*child spec 3 has a nil Factory`
			{Name: "w3", Factory: factoryWorker}, // want `A2005.*child name "w3" is already used`
		},
	}
	return spec, nil
}

// Cross field restart rejections.
type SupRestart struct {
	act.Supervisor
}

func (s *SupRestart) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Children: []act.SupervisorChildSpec{
			{
				Name:    "period-without-intensity",
				Factory: factoryWorker,
				Restart: act.SupervisorChildRestart{
					Period: 60, // want `A2005.*Restart.Period requires Restart.Intensity > 0`
				},
			},
			{
				Name:    "disable-without-intensity",
				Factory: factoryWorker,
				Restart: act.SupervisorChildRestart{
					OnExceed: act.OnExceedDisable, // want `A2005.*Restart.OnExceed=Disable requires Restart.Intensity > 0`
				},
			},
			{
				Name:    "valid-pair",
				Factory: factoryWorker,
				Restart: act.SupervisorChildRestart{
					Intensity: 3,
					Period:    60,
					OnExceed:  act.OnExceedDisable,
				},
			},
		},
	}
	return spec, nil
}

// Per child intensity and PreserveMailbox are unsupported for All and Rest For One.
type SupAllForOne struct {
	act.Supervisor
}

func (s *SupAllForOne) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeAllForOne,
		Children: []act.SupervisorChildSpec{
			{
				Name:    "w1",
				Factory: factoryWorker,
				Restart: act.SupervisorChildRestart{
					Intensity: 3, // want `A2005.*a per child Restart.Intensity is not supported for AllForOne`
				},
			},
			{
				Name:    "w2",
				Factory: factoryWorker,
				Options: gen.ProcessOptions{
					PreserveMailbox: true, // want `A2005.*Options.PreserveMailbox is not supported for AllForOne`
				},
			},
		},
	}
	return spec, nil
}

// An unknown enum value is rejected as "unknown".
type SupUnknownType struct {
	act.Supervisor
}

func (s *SupUnknownType) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorType(9), // want `A2005.*9 is not a known supervisor type`
		Children: []act.SupervisorChildSpec{
			{Name: "w1", Factory: factoryWorker},
		},
	}
	return spec, nil
}

// Dead configuration, reported as information rather than an error.
type SupDeadConfig struct {
	act.Supervisor
}

func (s *SupDeadConfig) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeSimpleOneForOne,
		Restart: act.SupervisorRestart{
			KeepOrder: true, // want `\[tier3\] \[A2005\] Restart.KeepOrder is ignored for SimpleOneForOne`
		},
		Children: []act.SupervisorChildSpec{
			{
				Name:        "w1",
				Factory:     factoryWorker,
				Significant: true, // want `\[tier3\] \[A2005\] Significant is ignored for SimpleOneForOne`
			},
		},
	}
	return spec, nil
}

// Significant under Permanent is also dead, and the strategy may be inherited from
// the supervisor level.
type SupPermanent struct {
	act.Supervisor
}

func (s *SupPermanent) Init(args ...any) (act.SupervisorSpec, error) {
	spec := act.SupervisorSpec{
		Type: act.SupervisorTypeOneForOne,
		Restart: act.SupervisorRestart{
			Strategy: act.SupervisorStrategyPermanent,
		},
		Children: []act.SupervisorChildSpec{
			{
				Name:        "w1",
				Factory:     factoryWorker,
				Significant: true, // want `\[tier3\] \[A2005\] Significant is ignored under the Permanent restart strategy`
			},
		},
	}
	return spec, nil
}

// The assignment built form: a third of real supervisors contain no literal at all.
type SupAssembled struct {
	act.Supervisor
}

func (s *SupAssembled) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeRestForOne
	spec.Children = []act.SupervisorChildSpec{
		{
			Name:    "w1",
			Factory: factoryWorker,
			Options: gen.ProcessOptions{
				PreserveMailbox: true, // want `A2005.*Options.PreserveMailbox is not supported for RestForOne`
			},
		},
	}
	return spec, nil
}

// A child list built by append is not enumerable, so the rule says nothing about
// the children.
type SupDynamic struct {
	act.Supervisor
}

func (s *SupDynamic) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	spec.Type = act.SupervisorTypeAllForOne
	for i := 0; i < 3; i++ {
		spec.Children = append(spec.Children, act.SupervisorChildSpec{
			Name:    "w",
			Factory: factoryWorker,
		})
	}
	return spec, nil
}

// A field assigned in a branch is order dependent, so the spec is unresolved and
// the rule stays silent rather than guessing which arm ran.
type SupConditional struct {
	act.Supervisor
}

func (s *SupConditional) Init(args ...any) (act.SupervisorSpec, error) {
	var spec act.SupervisorSpec
	if len(args) > 0 {
		spec.Type = act.SupervisorTypeAllForOne
	}
	return spec, nil
}

// A spec returned directly, which is how five specs in six are written. Resolving
// only the variable form left the rule blind to all of them.
type SupReturned struct {
	act.Supervisor
}

func (s *SupReturned) Init(args ...any) (act.SupervisorSpec, error) {
	return act.SupervisorSpec{ // want `A2005.*the Children list is empty`
		Type: act.SupervisorTypeOneForOne,
	}, nil
}

// A literal returned alongside an error is the failure path and carries no spec.
type SupFailing struct {
	act.Supervisor
}

func (s *SupFailing) Init(args ...any) (act.SupervisorSpec, error) {
	if len(args) == 0 {
		return act.SupervisorSpec{}, gen.ErrIncorrect
	}
	return act.SupervisorSpec{
		Children: []act.SupervisorChildSpec{
			{Name: "w", Factory: factoryWorker},
		},
	}, nil
}
