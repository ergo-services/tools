// Package act is a stand-in for the framework's act package. The method set on
// Actor is what the send, spawn and state-gated surfaces resolve against, and the
// supervisor spec types mirror the real ones because every A2005 check is cross
// field and reads real field paths.
package act

import (
	"time"

	"ergo.services/ergo/gen"
)

type Actor struct{}

func (a *Actor) ProcessInit(args ...any) error { return nil }

// sends
func (a *Actor) Send(to any, message any) error          { return nil }
func (a *Actor) SendPID(to gen.PID, message any) error   { return nil }
func (a *Actor) SendProcessID(to any, message any) error { return nil }
func (a *Actor) SendAlias(to gen.Alias, message any) error {
	return nil
}
func (a *Actor) SendWithPriority(to any, message any, priority gen.MessagePriority) error {
	return nil
}
func (a *Actor) SendImportant(to any, message any) error { return nil }
func (a *Actor) SendEvent(name gen.Atom, token gen.Ref, message any) error {
	return nil
}
func (a *Actor) SendResponse(to gen.PID, ref gen.Ref, message any) error {
	return nil
}
func (a *Actor) SendResponseError(to gen.PID, ref gen.Ref, err error) error {
	return nil
}
func (a *Actor) SendResponseImportant(to gen.PID, ref gen.Ref, message any) error {
	return nil
}
func (a *Actor) SendResponseErrorImportant(to gen.PID, ref gen.Ref, err error) error {
	return nil
}
func (a *Actor) SendExit(to gen.PID, reason error) error { return nil }

// timers
func (a *Actor) SendAfter(to any, message any, after time.Duration) (gen.CancelFunc, error) {
	return nil, nil
}
func (a *Actor) SendEvery(to any, message any, period time.Duration) (gen.CancelFunc, error) {
	return nil, nil
}
func (a *Actor) SendWithPriorityEvery(to any, message any, priority gen.MessagePriority, period time.Duration) (gen.CancelFunc, error) {
	return nil, nil
}
func (a *Actor) SendExitAfter(to gen.PID, reason error, after time.Duration) (gen.CancelFunc, error) {
	return nil, nil
}

// requests
func (a *Actor) Call(to any, request any) (any, error) { return nil, nil }
func (a *Actor) CallWithTimeout(to any, request any, timeout int) (any, error) {
	return nil, nil
}

// spawn
func (a *Actor) Spawn(factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return gen.PID{}, nil
}
func (a *Actor) SpawnRegister(register gen.Atom, factory gen.ProcessFactory, options gen.ProcessOptions, args ...any) (gen.PID, error) {
	return gen.PID{}, nil
}
func (a *Actor) SpawnMeta(behavior gen.MetaBehavior, options gen.MetaOptions) (gen.Alias, error) {
	return gen.Alias{}, nil
}

// subscriptions
func (a *Actor) LinkPID(target gen.PID) error    { return nil }
func (a *Actor) MonitorPID(target gen.PID) error { return nil }
func (a *Actor) LinkEvent(target gen.Event) ([]gen.MessageEvent, error) {
	return nil, nil
}
func (a *Actor) MonitorEvent(target gen.Event) ([]gen.MessageEvent, error) {
	return nil, nil
}
func (a *Actor) DemonitorEvent(target gen.Event) error { return nil }
func (a *Actor) RegisterEvent(name gen.Atom, options gen.EventOptions) (gen.Ref, error) {
	return gen.Ref{}, nil
}
func (a *Actor) UnregisterEvent(name gen.Atom) error { return nil }
func (a *Actor) RegisterName(name gen.Atom) error    { return nil }
func (a *Actor) CreateAlias() (gen.Alias, error)     { return gen.Alias{}, nil }

// state
func (a *Actor) SetEnv(name gen.Env, value any) {}
func (a *Actor) Aliases() []gen.Alias           { return nil }
func (a *Actor) CallWithPriority(to any, request any, priority gen.MessagePriority) (any, error) {
	return nil, nil
}
func (a *Actor) CallImportant(to any, request any) (any, error) { return nil, nil }

// The three address-typed forms take the timeout as their third argument, exactly as
// the framework declares them.
func (a *Actor) CallProcessID(to gen.ProcessID, request any, timeout int) (any, error) {
	return nil, nil
}
func (a *Actor) CallAlias(to gen.Alias, request any, timeout int) (any, error) {
	return nil, nil
}
func (a *Actor) CallPID(to gen.PID, request any, timeout int) (any, error) {
	return nil, nil
}
func (a *Actor) Log() gen.Log   { return nil }
func (a *Actor) PID() gen.PID   { return gen.PID{} }
func (a *Actor) Name() gen.Atom { return "" }
func (a *Actor) Node() gen.Node { return nil }

func (a *Actor) SendExitMeta(meta gen.Alias, reason error) error { return nil }
func (a *Actor) Parent() gen.PID                                 { return gen.PID{} }
func (a *Actor) Leader() gen.PID                                 { return gen.PID{} }
func (a *Actor) Link(target any) error                           { return nil }
func (a *Actor) Monitor(target any) error                        { return nil }
func (a *Actor) LinkProcessID(target gen.ProcessID) error        { return nil }
func (a *Actor) MonitorProcessID(target gen.ProcessID) error     { return nil }
func (a *Actor) SetCompressionThreshold(threshold int) error     { return nil }
func (a *Actor) SetCompressionType(ctype gen.CompressionType) error {
	return nil
}
func (a *Actor) SetCompressionLevel(level gen.CompressionLevel) error {
	return nil
}
func (a *Actor) SetSendPriority(priority gen.MessagePriority) error { return nil }

type Pool struct{ Actor }

type Router struct{ Actor }

// Route and RouterOptions mirror the real declarations: A2024 resolves the literal
// and reads the same field paths the runtime validates.
type Route struct {
	Name    gen.Atom
	Factory gen.ProcessFactory
	Args    []any
}

type RouterOptions struct {
	Routes      []Route
	MailboxSize int64
}

type PoolOptions struct {
	WorkerMailboxSize int64
	PoolSize          int64
	WorkerFactory     gen.ProcessFactory
	WorkerArgs        []any
}

type WebWorker struct{ Actor }

type Supervisor struct{ Actor }

// Supervisor spec surface. The field names and the enum values match the real ones
// exactly, because the rule reads resolved paths and numeric constants.

type SupervisorType int

const (
	SupervisorTypeOneForOne       SupervisorType = 0
	SupervisorTypeAllForOne       SupervisorType = 1
	SupervisorTypeRestForOne      SupervisorType = 2
	SupervisorTypeSimpleOneForOne SupervisorType = 3
)

type SupervisorStrategy int

const (
	SupervisorStrategyInherit   SupervisorStrategy = 0
	SupervisorStrategyTransient SupervisorStrategy = 1
	SupervisorStrategyTemporary SupervisorStrategy = 2
	SupervisorStrategyPermanent SupervisorStrategy = 3
)

type OnExceed uint8

const (
	OnExceedTerminateSupervisor OnExceed = 0
	OnExceedDisable             OnExceed = 1
)

type SupervisorRestart struct {
	Strategy  SupervisorStrategy
	Intensity uint16
	Period    uint16
	KeepOrder bool
}

type SupervisorChildRestart struct {
	Strategy  SupervisorStrategy
	Intensity uint16
	Period    uint16
	OnExceed  OnExceed
}

type SupervisorChildSpec struct {
	Name        gen.Atom
	Significant bool
	Factory     gen.ProcessFactory
	Options     gen.ProcessOptions
	Args        []any
	Restart     SupervisorChildRestart
}

type SupervisorSpec struct {
	Children            []SupervisorChildSpec
	Type                SupervisorType
	Restart             SupervisorRestart
	EnableHandleChild   bool
	DisableAutoShutdown bool
}
