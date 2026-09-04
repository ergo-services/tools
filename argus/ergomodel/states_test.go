package ergomodel_test

import (
	"testing"

	"ergo.tools/argus/ergomodel"
)

func TestStateTableSendFamilySplit(t *testing.T) {
	permitted := []string{
		"Send", "SendPID", "SendProcessID", "SendAlias", "SendWithPriority",
		"SendExit", "SendExitMeta",
	}
	for _, name := range permitted {
		gate, ok := ergomodel.LookupGate(ergomodel.SurfaceProcess, name)
		if ok == false {
			t.Errorf("%s is missing from the process table", name)
			continue
		}
		if gate.ForbidTerminated {
			t.Errorf("%s must be permitted on the terminate path (isStateIRT)", name)
		}
	}

	gate, ok := ergomodel.LookupGate(ergomodel.SurfaceProcess, "SendImportant")
	if ok == false {
		t.Fatal("SendImportant is missing from the process table")
	}
	if gate.ForbidTerminated == false {
		t.Error("SendImportant is isStateIR, so it must be forbidden on the terminate path")
	}
}

func TestStateTableMetaSurface(t *testing.T) {
	forbidden := []string{"SendResponse", "SendResponseError", "SetSendPriority", "SetCompression"}
	for _, name := range forbidden {
		gate, ok := ergomodel.LookupGate(ergomodel.SurfaceMeta, name)
		if ok == false {
			t.Errorf("%s is missing from the meta table", name)
			continue
		}
		if gate.ForbidMetaPreStart == false {
			t.Errorf("%s requires MetaStateRunning, so it must be forbidden before Start", name)
		}
	}

	if gate, ok := ergomodel.LookupGate(ergomodel.SurfaceMeta, "Send"); ok == false || gate.ForbidMetaPreStart {
		t.Error("Send is not gated on Running, so it must work during a meta Init")
	}
	if gate, ok := ergomodel.LookupGate(ergomodel.SurfaceMeta, "Spawn"); ok == false || gate.ForbidMetaPreStart {
		t.Error("Spawn is not gated on Running, so it must work during a meta Init")
	}
}

func TestStateTableResultShapes(t *testing.T) {
	cases := map[string]ergomodel.ResultShape{
		"SendAfter":             ergomodel.ResultCancelOneShot,
		"SendWithPriorityAfter": ergomodel.ResultCancelOneShot,
		"SendExitAfter":         ergomodel.ResultCancelOneShot,
		"SendEvery":             ergomodel.ResultCancelPeriodic,
		"SendWithPriorityEvery": ergomodel.ResultCancelPeriodic,
		"LinkEvent":             ergomodel.ResultEventBuffer,
		"MonitorEvent":          ergomodel.ResultEventBuffer,
		"SetEnv":                ergomodel.ResultNone,
		"RegisterName":          ergomodel.ResultError,
	}
	for name, want := range cases {
		gate, ok := ergomodel.LookupGate(ergomodel.SurfaceProcess, name)
		if ok == false {
			t.Errorf("%s is missing from the process table", name)
			continue
		}
		if gate.Result != want {
			t.Errorf("%s result shape = %v, want %v", name, gate.Result, want)
		}
	}

	gate, _ := ergomodel.LookupGate(ergomodel.SurfaceProcess, "SetEnv")
	if gate.Observable() {
		t.Error("SetEnv returns nothing, so its failure is not observable")
	}
}

func TestStateTableIsConsistent(t *testing.T) {
	for name, gate := range ergomodel.GatedMethods() {
		if gate.Method != name {
			t.Errorf("process table key %q holds Method %q", name, gate.Method)
		}
		if gate.Surface != ergomodel.SurfaceProcess {
			t.Errorf("%s is in the process table with surface %q", name, gate.Surface)
		}
		if gate.ForbidMetaPreStart {
			t.Errorf("%s is a process row and must not carry the meta gate", name)
		}
	}
	for name, gate := range ergomodel.MetaGatedMethods() {
		if gate.Method != name {
			t.Errorf("meta table key %q holds Method %q", name, gate.Method)
		}
		if gate.Surface != ergomodel.SurfaceMeta {
			t.Errorf("%s is in the meta table with surface %q", name, gate.Surface)
		}
	}
}
