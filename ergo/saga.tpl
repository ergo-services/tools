package {{ .Package }}

import (
	"fmt"

	"github.com/ergo-services/ergo/etf"
	"github.com/ergo-services/ergo/gen"
)

func create{{ .Name }}() gen.SagaBehavior {
	return &{{ .Name }}{}
}

type {{ .Name }} struct {
	gen.Saga
}

//
// Mandatory callbacks
//

// InitSaga
func (s *{{ .Name }}) InitSaga(process *gen.SagaProcess, args ...etf.Term) (gen.SagaOptions, error) {
	opts := gen.SagaOptions{
		Worker: create{{ .Name }}Worker(),
	}

	return opts, nil
}

// HandleTxNew invokes on a new TX receiving by this saga.
func (s *{{ .Name }}) HandleTxNew(process *gen.SagaProcess, id gen.SagaTransactionID, value interface{}) gen.SagaStatus {
	fmt.Printf("Starting new Tx %v with value: %q\n", id, value)
	process.StartJob(id, gen.SagaJobOptions{}, value)
	return gen.SagaStatusOK
}

// HandleTxResult invoked on a receiving result from the next saga
func (s *{{ .Name }}) HandleTxResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, result interface{}) gen.SagaStatus {
	fmt.Printf("Received result for TX %v from %v with value %q\n", id, from, result)
	return gen.SagaStatusOK
}

// HandleTxCancel invoked on a request of transaction cancelation.
func (s *{{ .Name }}) HandleTxCancel(process *gen.SagaProcess, id gen.SagaTransactionID, reason string) gen.SagaStatus {
	fmt.Printf("Tx %v is canceled with reason: %s\n", id, reason)
	return gen.SagaStatusOK
}


//
// Optional callbacks
//

// HandleTxDone invoked when the transaction is done on a saga where it was created.
// It returns the final result and SagaStatus. The commit message will deliver the final
// result to all participants of this transaction (if it has enabled the TwoPhaseCommit option).
// Otherwise the final result will be ignored.
func (s *{{ .Name }}) HandleTxDone(process *gen.SagaProcess, id gen.SagaTransactionID, result interface{}) (interface{}, gen.SagaStatus) {
	fmt.Printf("Tx %v is done with result: %v\n", id, result)
	return result, gen.SagaStatusOK
}

// HandleTxInterim invoked if received interim result from the next hop
func (s *{{ .Name }}) HandleTxInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaNextID, interim interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}


// HandleTxCommit invoked if TwoPhaseCommit option is enabled for the given TX.
// All sagas involved in this TX receive a commit message with final value and invoke this callback.
// The final result has a value returned by HandleTxDone on a Saga created this TX.
func (s *{{ .Name }}) HandleTxCommit(process *gen.SagaProcess, id gen.SagaTransactionID, final interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

//
// Optional Callback to handle result/interim from the worker(s)
//

// HandleJobResult
func (s *{{ .Name }}) HandleJobResult(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, result interface{}) gen.SagaStatus {
	fmt.Printf("Received result for Job %v from %v with value %q\n", id, from, result)
	return gen.SagaStatusOK
}
// HandleJobInterim
func (s *{{ .Name }}) HandleJobInterim(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, interim interface{}) gen.SagaStatus {
	return gen.SagaStatusOK
}

// HandleJobFailed
func (s *{{ .Name }}) HandleJobFailed(process *gen.SagaProcess, id gen.SagaTransactionID, from gen.SagaJobID, reason string) gen.SagaStatus {
	return gen.SagaStatusOK
}

//
// Optional gen.Server's callbacks
//

// HandleStageInfo this callback is invoked on Process.Send(...).
// Implement this method in order to trap TX cancelation and forward it to the other saga process
func (s *{{ .Name }}) HandleSagaInfo(process *gen.SagaProcess, message etf.Term) gen.ServerStatus {
	switch m := message.(type) {
	case gen.MessageSagaCancel:
		fmt.Printf("Trapped cancelation %v. Reason %q\n", m.TransactionID, m.Reason)
		//  next := gen.SagaNext{
		//    Saga:  gen.ProcessID{Name: "otherSaga", Node: "otherNode@localhost"},
		//    Value: <value>,
		//  }
		//  next_id, _ := process.Next(m.TransactionID, next)
	}
	return gen.ServerStatusOK
}

// HandleStageCast this callback is invoked on ServerProcess.Cast(...).
func (s *{{ .Name }}) HandleSagaCast(process *gen.SagaProcess, message etf.Term) gen.ServerStatus {
	return gen.ServerStatusOK
}

// HandleSagaCall this callback is invoked on ServerProcess.Call(...).
func (s *{{ .Name }}) HandleSagaCall(process *gen.SagaProcess, from gen.ServerFrom, message etf.Term) (etf.Term, gen.ServerStatus) {
	return nil, gen.ServerStatusOK
}

// HandleSagaDirect this callback is invoked on Process.Direct(...).
func (s *{{ .Name }}) HandleSagaDirect(process *gen.SagaProcess, ref etf.Ref, message interface{}) (interface{}, gen.DirectStatus) {
	return nil, gen.DirectStatusOK
}
