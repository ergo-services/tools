package ergomodel

import (
	"go/ast"
	"strings"
)

type Surface string

const (
	SurfaceProcess Surface = "process"
	SurfaceMeta    Surface = "meta"
)

type ResultShape int

const (
	ResultError ResultShape = iota
	ResultNone
	ResultCancelOneShot
	ResultCancelPeriodic
	ResultEventBuffer
)

type Gate struct {
	Method  string
	Surface Surface

	ForbidTerminated bool

	ForbidMetaPreStart bool
	Result             ResultShape
}

func (g Gate) Observable() bool { return g.Result != ResultNone }

var processGates = func() map[string]Gate {
	out := map[string]Gate{}

	for _, name := range []string{
		"Send", "SendPID", "SendProcessID", "SendAlias", "SendWithPriority",
		"SendExit", "SendExitMeta",
	} {
		out[name] = Gate{Method: name, Surface: SurfaceProcess}
	}

	forbidden := []string{
		"Call", "CallWithTimeout", "CallWithPriority", "CallImportant",
		"CallPID", "CallProcessID", "CallAlias",
		"SendImportant", "SendEvent",
		"SendResponse", "SendResponseError", "SendResponseImportant",
		"Spawn", "SpawnRegister", "SpawnMeta", "RemoteSpawn", "RemoteSpawnRegister",
		"LinkPID", "LinkProcessID", "LinkAlias", "LinkNode",
		"UnlinkPID", "UnlinkProcessID", "UnlinkAlias", "UnlinkEvent", "UnlinkNode",
		"MonitorPID", "MonitorProcessID", "MonitorAlias", "MonitorNode",
		"DemonitorPID", "DemonitorProcessID", "DemonitorAlias", "DemonitorEvent",
		"DemonitorNode",
		"RegisterName", "UnregisterName", "RegisterEvent", "UnregisterEvent",
		"CreateAlias", "DeleteAlias",
		"SetCompression", "SetCompressionLevel", "SetCompressionType",
		"SetCompressionThreshold", "SetSendPriority", "SetImportantDelivery",
		"SetKeepNetworkOrder", "SetProcessKind", "SetTracingSampler",
		"Inspect", "InspectMeta", "MetaInfo",
	}
	for _, name := range forbidden {
		out[name] = Gate{Method: name, Surface: SurfaceProcess, ForbidTerminated: true}
	}

	for _, name := range []string{"SendAfter", "SendWithPriorityAfter", "SendExitAfter", "SendExitMetaAfter"} {
		out[name] = Gate{Method: name, Surface: SurfaceProcess,
			ForbidTerminated: true, Result: ResultCancelOneShot}
	}
	for _, name := range []string{"SendEvery", "SendWithPriorityEvery"} {
		out[name] = Gate{Method: name, Surface: SurfaceProcess,
			ForbidTerminated: true, Result: ResultCancelPeriodic}
	}

	for _, name := range []string{"LinkEvent", "MonitorEvent"} {
		out[name] = Gate{Method: name, Surface: SurfaceProcess,
			ForbidTerminated: true, Result: ResultEventBuffer}
	}

	out["SetEnv"] = Gate{Method: "SetEnv", Surface: SurfaceProcess,
		ForbidTerminated: true, Result: ResultNone}

	return out
}()

var metaGates = map[string]Gate{
	"SendResponse":      {Method: "SendResponse", Surface: SurfaceMeta, ForbidMetaPreStart: true},
	"SendResponseError": {Method: "SendResponseError", Surface: SurfaceMeta, ForbidMetaPreStart: true},
	"SetSendPriority":   {Method: "SetSendPriority", Surface: SurfaceMeta, ForbidMetaPreStart: true},
	"SetCompression":    {Method: "SetCompression", Surface: SurfaceMeta, ForbidMetaPreStart: true},
}

func LookupGate(surface Surface, method string) (Gate, bool) {
	if surface == SurfaceMeta {
		if g, ok := metaGates[method]; ok {
			return g, true
		}

	}
	g, ok := processGates[method]
	return g, ok
}

func SurfaceOf(cb *Callback) Surface {
	if cb != nil && cb.Meta {
		return SurfaceMeta
	}
	return SurfaceProcess
}

func (m *Model) GatedCall(call *ast.CallExpr, surface Surface) (Gate, bool) {
	fn, ok := m.calleeFunc(call)
	if ok == false {
		return Gate{}, false
	}
	if fn.Pkg() == nil || strings.HasPrefix(fn.Pkg().Path(), ergoPrefix) == false {
		return Gate{}, false
	}
	return LookupGate(surface, fn.Name())
}

func GatedMethods() map[string]Gate { return processGates }

func MetaGatedMethods() map[string]Gate { return metaGates }

var KnownRuleIDs = map[string]bool{
	"A1001":  true,
	"A1002":  true,
	"A1003":  true,
	"A1004":  true,
	"A1007":  true,
	"A1008":  true,
	"A1010":  true,
	"A1011":  true,
	"A1005":  true,
	"A1006":  true,
	"A1012":  true,
	"A2001":  true,
	"A2001a": true,
	"A2002":  true,
	"A2003":  true,
	"A2004":  true,
	"A2005":  true,
	"A2006":  true,
	"A2007":  true,
	"A2008":  true,
	"A2009":  true,
	"A2011":  true,
	"A2012":  true,
	"A2013":  true,
	"A2014":  true,
	"A2014a": true,
	"A2015":  true,
	"A2016":  true,
	"A2017":  true,
	"A2018":  true,
	"A2020":  true,
	"A2021":  true,
	"A2022":  true,
	"A2023":  true,
	"A2024":  true,
	"A2025":  true,
	"A2026":  true,
	"A2027":  true,
	"A2028":  true,
	"A3001":  true,
	"A3003":  true,
	"A3005":  true,
	"A3006":  true,
	"A3007":  true,
}
