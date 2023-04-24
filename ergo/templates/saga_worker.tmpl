package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/gen"
	"github.com/ergo-services/ergo/etf"
)

func create{{ .Name }}Worker() gen.SagaWorkerBehavior {
	return &{{ .Name }}Worker{}
}

type {{ .Name }}Worker struct {
	gen.SagaWorker
}

//
// Mandatory callbacks
//

// HandleJobStart invoked on a worker start
func (w *{{ .Name }}Worker) HandleJobStart(process *gen.SagaWorkerProcess, job gen.SagaJob) error {
	fmt.Printf(" Worker started on Saga with value %q\n", job.Value)
	// process job and send result using process.SendResult(result)
	return nil
}

// HandleJobCancel invoked if transaction was canceled before the termination.
func (w *{{ .Name }}Worker) HandleJobCancel(process *gen.SagaWorkerProcess, reason string) {
	return
}

//
// Optional callbacks
//

// HandleJobCommit invoked if this job was a part of the transaction
// with enabled TwoPhaseCommit option. All workers involved in this TX
// handling are receiving this call. Callback invoked before the termination.
func (w *{{ .Name }}Worker) HandleJobCommit(process *gen.SagaWorkerProcess, final interface{}) {
	// if two phase commit is enabled on this transaction
	fmt.Printf(" Worker received final result with value %q\n", final)
}

// HandleWorkerInfo this callback is invoked on Process.Send(...).
func (w *{{ .Name }}Worker) HandleWorkerInfo(process *gen.SagaWorkerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleWorkerCast this callback is invoked on ServerProcess.Cast(...).
// for the implementation
func (w *{{ .Name }}Worker) HandleWorkerCast(process *gen.SagaWorkerProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleWorkerCall this callback is invoked on ServerProcess.Call(...).
func (w *{{ .Name }}Worker) HandleWorkerCall(process *gen.SagaWorkerProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleWorkerDirect this callback is invoked on Process.Direct(...).
func (w *{{ .Name }}Worker) HandleWorkerDirect(process *gen.SagaWorkerProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	return nil, gen.DirectStatusOK
}

// HandleWorkerTerminate this callback invoked on a process termination
func (w *{{ .Name }}Worker) HandleWorkerTerminate(process *gen.SagaWorkerProcess, reason string) {
	fmt.Printf("Saga Worker terminated with reason: %q\n", reason)
}
