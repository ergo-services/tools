package ergomodel

import (
	"time"

	"golang.org/x/tools/go/analysis"
)

type ShapeFact struct {
	Aliasing bool
	Opaque   bool
	WireOK   bool
	Message  bool
	Local    bool
	Allowed  bool
	Clone    bool
	Witness  string
}

func (*ShapeFact) AFact() {}

type SenderFact struct{ Params []int }

func (*SenderFact) AFact() {}

type BlocksFact struct {
	Why     string
	Bounded bool
	Timeout time.Duration
}

func (*BlocksFact) AFact() {}

type SpawnsFact struct{}

func (*SpawnsFact) AFact() {}

type UnrecoveredSpawnFact struct{}

func (*UnrecoveredSpawnFact) AFact() {}

type RecoversFact struct{}

func (*RecoversFact) AFact() {}

type Escape struct {
	Result int
	Field  string
}

type EscapesFact struct{ Escapes []Escape }

func (*EscapesFact) AFact() {}

type RepliesFact struct{}

func (*RepliesFact) AFact() {}

type FactoryFact struct{ Behavior string }

func (*FactoryFact) AFact() {}

type InitBudgetFact struct {
	Blocks       bool
	Why          string
	InnerTimeout int
	Chain        string
}

func (*InitBudgetFact) AFact() {}

type RoundTripFact struct{ Why string }

func (*RoundTripFact) AFact() {}

type ExternalRoundTripFact struct{ Why string }

func (*ExternalRoundTripFact) AFact() {}

type NodeSenderFact struct {
	Params []int
	Method string
}

func (*NodeSenderFact) AFact() {}

type InitSentinelFact struct{ Sentinel string }

func (*InitSentinelFact) AFact() {}

type FmtWrappedFact struct{}

func (*FmtWrappedFact) AFact() {}

var factTypes = []analysis.Fact{
	(*ShapeFact)(nil),
	(*SenderFact)(nil),
	(*BlocksFact)(nil),
	(*SpawnsFact)(nil),
	(*UnrecoveredSpawnFact)(nil),
	(*RecoversFact)(nil),
	(*EscapesFact)(nil),
	(*RepliesFact)(nil),
	(*FactoryFact)(nil),
	(*InitBudgetFact)(nil),
	(*InitSentinelFact)(nil),
	(*RoundTripFact)(nil),
	(*NodeSenderFact)(nil),
	(*FmtWrappedFact)(nil),
	(*ExternalRoundTripFact)(nil),
}
