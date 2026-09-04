// Package gen is a stand-in for the framework's gen package. Only the surface the
// rules resolve is present, and every shape mirrors the real declaration: argus
// gates on resolved objects and package paths, so a stub that drifts would make a
// test pass against a surface that does not exist.
package gen

import (
	"errors"
	"fmt"
	"time"
)

type Atom string

type PID struct {
	Node Atom
	ID   uint64
}

type Ref struct{ ID uint64 }

type Alias struct{ ID uint64 }

type Event struct {
	Name Atom
	Node Atom
}

type Env string

type CancelFunc func() bool

type MessagePriority int

const (
	MessagePriorityNormal MessagePriority = iota
	MessagePriorityHigh
	MessagePriorityMax
)

var (
	ErrNoConnection         = errors.New("no connection")
	ErrTimeout              = errors.New("timeout")
	ErrProcessUnknown       = errors.New("process unknown")
	ErrUnsupported          = errors.New("unsupported")
	ErrUnknown              = errors.New("unknown")
	ErrIncorrect            = errors.New("incorrect")
	ErrNotAllowed           = errors.New("not allowed")
	TerminateReasonNormal   = errors.New("normal")
	TerminateReasonShutdown = errors.New("shutdown")
)

type Log interface {
	Error(f string, a ...any)
	Warning(f string, a ...any)
	Info(f string, a ...any)
	Debug(f string, a ...any)
	Panic(f string, a ...any)
}

// EventOptions mirrors the real options: Notify drives the producer notifications
// and Buffer is what makes a discarded event buffer data loss.
type EventOptions struct {
	Notify bool
	Buffer int
	Open   bool
}

type MessageEventStart struct{ Name Atom }

type MessageEventStop struct{ Name Atom }

type MessageEvent struct {
	Event   Event
	Message any
}

type CompressionType string

type CompressionLevel int

const (
	CompressionTypeGZIP CompressionType = "gzip"
	CompressionTypeLZW  CompressionType = "lzw"
	CompressionTypeZLIB CompressionType = "zlib"

	CompressionDefault   CompressionLevel = 0
	CompressionBestSpeed CompressionLevel = 1
	CompressionBestSize  CompressionLevel = 2
)

var DefaultCompressionThreshold int = 1024

type Compression struct {
	Enable    bool
	Type      CompressionType
	Level     CompressionLevel
	Threshold int
}

type ProcessOptions struct {
	PreserveMailbox bool
	InitTimeout     int
	Compression     Compression
	Env             map[Env]any
}

type ProcessFactory func() ProcessBehavior

type ProcessBehavior interface {
	ProcessInit(args ...any) error
}

type MetaBehavior interface {
	Init(process MetaProcess) error
	Start() error
	HandleMessage(from PID, message any) error
	HandleCall(from PID, ref Ref, request any) (any, error)
	Terminate(reason error)
}

type ApplicationBehavior interface {
	Load(node Node, args ...any) (ApplicationSpec, error)
	Start(mode ApplicationMode)
	Terminate(reason error)
}

type ApplicationMode int

type ApplicationSpec struct {
	Name    Atom
	Group   []ApplicationMemberSpec
	Mode    ApplicationMode
	Network ApplicationNetwork
}

type ApplicationNetwork struct {
	RegisterTypes  []any
	RegisterErrors []error
	RegisterAtoms  []Atom
}

type ApplicationMemberSpec struct {
	Name    Atom
	Factory ProcessFactory
	Options ProcessOptions
}

type Network interface {
	RegisterType(v any) error
	RegisterTypes(types []any) error
	RegisterError(err error) error
	RegisterErrors(errs []error) error
	RegisterAtom(a Atom) error
	RegisterAtoms(atoms []Atom) error
}

// Node carries the routing methods A1012 reads plus enough of the rest to show that
// taking a node handle is not by itself a send.
type Node interface {
	Network() Network
	Log() Log
	Name() Atom
	Uptime() int64
	Send(to any, message any) error
	SendWithPriority(to any, message any, priority MessagePriority) error
	SendEvent(name Atom, token Ref, message any) error
	SendExit(pid PID, reason error) error
	Call(to any, request any) (any, error)
	CallWithTimeout(to any, request any, timeout int) (any, error)
	CallPID(to PID, request any, timeout int) (any, error)
}

type ProcessID struct {
	Name Atom
	Node Atom
}

// MetaProcess is the handle a meta holds. The gated methods are here because the
// state table says they fail during a meta Init.
type MetaProcess interface {
	ID() Alias
	Parent() PID
	Send(to any, message any) error
	SendWithPriority(to any, message any, priority MessagePriority) error
	SendResponse(to PID, ref Ref, message any) error
	SendResponseError(to PID, ref Ref, err error) error
	SetSendPriority(priority MessagePriority) error
	SetCompression(enable bool) error
	Spawn(behavior MetaBehavior, options MetaOptions) (Alias, error)
	Log() Log
}

type MetaOptions struct {
	MailboxSize int64
}

// Duration is re-exported so a fixture can name a period without importing time
// when it does not otherwise need it.
type Duration = time.Duration

// Error is the framework's own error value. EDF reaches its Wrapped slice, which
// is what makes gen.Errorf preserve identity across a node hop.
type Error struct {
	Msg     string
	Wrapped []error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

func (e *Error) Unwrap() []error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

func Errorf(format string, args ...any) error {
	w := fmt.Errorf(format, args...)
	e := &Error{Msg: w.Error()}
	switch u := w.(type) {
	case interface{ Unwrap() error }:
		if m := u.Unwrap(); m != nil {
			e.Wrapped = []error{m}
		}
	case interface{ Unwrap() []error }:
		e.Wrapped = u.Unwrap()
	}
	return e
}

// Application is the runtime handle an application behavior holds.
type Application interface {
	Name() Atom
	Node() Node
	Log() Log
}
